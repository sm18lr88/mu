package home

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/blog"
	"mu/config"
	"mu/news"
	"mu/video"
)

//go:embed cards.json
var f embed.FS

var Template = `<div id="home">
  <div class="home-left">%s</div>
  <div class="home-right">%s</div>
</div>`

type Card struct {
	ID          string
	Title       string
	Column      string // "left" or "right"
	Position    int
	Link        string
	Content     func() string
	CachedHTML  string    // Cached rendered content
	ContentHash string    // Hash of content for change detection
	UpdatedAt   time.Time // Last update timestamp
}

var (
	lastRefresh time.Time
	cacheMutex  sync.RWMutex
	cacheTTL    time.Duration = 0 // Always refresh card content for latest feeds/settings
)

type CardConfig struct {
	Left []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
	} `json:"left"`
	Right []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Type     string `json:"type"`
		Position int    `json:"position"`
		Link     string `json:"link"`
	} `json:"right"`
}

var Cards []Card

func Load() {
	data, _ := f.ReadFile("cards.json")
	var cardsCfg CardConfig
	if err := json.Unmarshal(data, &cardsCfg); err != nil {
		fmt.Println("Error loading cards.json:", err)
		return
	}

	// Map of card types to their content functions
	cardFunctions := map[string]func() string{
		"news":     news.Headlines,
		"markets":  news.Markets,
		"reminder": news.Reminder,
		"posts":    blog.Preview,
		"video":    video.Latest,
	}

	// Build Cards array from config
	Cards = []Card{}

	for _, c := range cardsCfg.Left {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Column:   "left",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	for _, c := range cardsCfg.Right {
		if fn, ok := cardFunctions[c.Type]; ok {
			Cards = append(Cards, Card{
				ID:       c.ID,
				Title:    c.Title,
				Column:   "right",
				Position: c.Position,
				Link:     c.Link,
				Content:  fn,
			})
		}
	}

	// Sort by column and position
	sort.Slice(Cards, func(i, j int) bool {
		if Cards[i].Column != Cards[j].Column {
			return Cards[i].Column < Cards[j].Column
		}
		return Cards[i].Position < Cards[j].Position
	})

	// Do initial refresh
	RefreshCards()

	// Refresh cards immediately when settings change (e.g., news sources)
	config.RegisterUpdateHook(func(config.Settings) {
		cacheMutex.Lock()
		lastRefresh = time.Time{}
		cacheMutex.Unlock()
		RefreshCards()
	})
}

// RefreshCards updates card content and timestamps if content changed
func RefreshCards() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	now := time.Now()
	cacheValid := cacheTTL > 0 && now.Sub(lastRefresh) < cacheTTL

	for i := range Cards {
		card := &Cards[i]

		// Always refresh reminder; others respect cache TTL
		if !cacheValid || card.ID == "reminder" {
			content := card.Content()
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			if hash != card.ContentHash {
				card.CachedHTML = content
				card.ContentHash = hash
				card.UpdatedAt = now
			}
		}
	}

	lastRefresh = now
}

// RefreshHandler clears the last_visit cookie to show all cards again
func RefreshHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	cookie := &http.Cookie{
		Name:     "last_visit",
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // Delete cookie
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)

	// Redirect back to home
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Refresh cards if cache expired (2 minute TTL)
	RefreshCards()

	var leftHTML []string
	var rightHTML []string

	for _, card := range Cards {
		content := card.CachedHTML
		if strings.TrimSpace(content) == "" {
			continue
		}

		// Add "More" link if card has a link URL
		if card.Link != "" {
			content += app.Link("More", card.Link)
		}
		html := app.Card(card.ID, card.Title, content)
		if card.Column == "left" {
			leftHTML = append(leftHTML, html)
		} else {
			rightHTML = append(rightHTML, html)
		}
	}

	// create homepage
	var homepage string
	if len(leftHTML) == 0 && len(rightHTML) == 0 {
		// No content - show message
		homepage = `<div id="home"><div class="home-left">` +
			app.Card("no-content", "Welcome", "<p>Welcome to Mu! Your personalized content will appear here.</p>") +
			`</div><div class="home-right"></div></div>`
	} else {
		homepage = fmt.Sprintf(Template,
			strings.Join(leftHTML, "\n"),
			strings.Join(rightHTML, "\n"))
	}

	// render html using user's language preference
	html := app.RenderHTMLForRequest("Home", "The Mu homescreen", homepage, r)

	w.Write([]byte(html))
}
