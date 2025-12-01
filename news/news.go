package news

import (
	"crypto/md5"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"mu/app"
	"mu/config"
	"mu/data"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
	"github.com/mrz1836/go-sanitize"
	"github.com/piquette/finance-go/future"
	nethtml "golang.org/x/net/html"
)

//go:embed feeds.json
var f embed.FS

var mutex sync.RWMutex

var feeds = map[string]map[string]string{}

var status = map[string]*Feed{}

// cached topics html
var topicsHtml string

// cached headlines and content html
var headlinesAndContentHtml string

// cached headlines
var headlinesHtml string

// markets
var marketsHtml string

// cached prices
var cachedPrices map[string]float64

// reminder
var reminderHtml string
var reminderSource string
var reminderFetched time.Time
var reminderMutex sync.Mutex

// track the currently loaded news source selection to detect drift vs settings
var sourcesSignature string
var lastRefreshRequest time.Time

// the cached feed
var feed []*Post

type Feed struct {
	Name     string
	URL      string
	Error    error
	Attempts int
	Backoff  time.Time
}

type Post struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Published   string    `json:"published"`
	Category    string    `json:"category"`
	PostedAt    time.Time `json:"posted_at"`
	Image       string    `json:"image"`
	Content     string    `json:"content"`
}

type Metadata struct {
	Created     int64
	Title       string
	Description string
	Type        string
	Image       string
	Url         string
	Site        string
	Content     string
}

// htmlToText converts HTML to plain text with proper spacing and preserved links.
func htmlToText(html string) string {
	if html == "" {
		return ""
	}

	doc, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		re := regexp.MustCompile(`<[^>]*>`)
		text := re.ReplaceAllString(html, " ")
		re2 := regexp.MustCompile(`\s+`)
		return strings.TrimSpace(re2.ReplaceAllString(text, " "))
	}

	var sb strings.Builder
	var extract func(*nethtml.Node)

	extract = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			sb.WriteString(n.Data)
		}
		if n.Type == nethtml.ElementNode {
			if n.Data == "a" {
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				if href != "" {
					sb.WriteString(`<a href="`)
					sb.WriteString(href)
					sb.WriteString(`" target="_blank" rel="noopener noreferrer">`)
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c)
				}
				if href != "" {
					sb.WriteString("</a>")
				}
				sb.WriteString(" ")
			} else {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extract(c)
				}
				switch n.Data {
				case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
					sb.WriteString(" ")
				}
			}
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extract(c)
			}
		}
	}

	extract(doc)

	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(sb.String(), " "))
}

func getDomain(v string) string {
	var host string

	u, err := url.Parse(v)
	if err == nil {
		host = u.Hostname()
	} else {
		parts := strings.Split(v, "/")
		if len(parts) < 3 {
			return v
		}
		host = strings.TrimSpace(parts[2])
	}

	if strings.Contains(host, "github.io") {
		return host
	}

	host = strings.TrimPrefix(host, "www.")

	return host
}

var Results = `
<div id="topics">%s</div>
<h1 style="margin-top: 0">Results</h1>
<div id="results">
%s
</div>`

func getSummary(post *Post) string {
	discussLink := ""
	if post.ID != "" {
		discussLink = fmt.Sprintf(` | <a href="/chat?id=news_%s" style="color: inherit;">Discuss</a>`, post.ID)
	}
	return fmt.Sprintf(`Source: <i>%s</i> | %s%s`, getDomain(post.URL), app.TimeAgo(post.PostedAt), discussLink)
}

func getPrices() map[string]float64 {
	app.Log("news", "Getting prices")
	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		app.Log("news", "Error getting prices: %v", err)
		return nil
	}
	b, _ := io.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	var res map[string]interface{}
	json.Unmarshal(b, &res)
	if res == nil {
		return nil
	}

	rates := res["data"].(map[string]interface{})["rates"].(map[string]interface{})

	prices := map[string]float64{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t.(string), 64)
		if err != nil {
			continue
		}
		prices[k] = 1 / val
	}

	// let's get other prices
	for key, ftr := range futures {
		// Use closure to safely handle potential panics from individual futures
		func() {
			defer func() {
				if r := recover(); r != nil {
					app.Log("news", "Recovered from panic getting future %s (%s): %v", key, ftr, r)
				}
			}()

			f, err := future.Get(ftr)
			if err != nil {
				app.Log("news", "Failed to get future %s (%s): %v", key, ftr, err)
				return
			}
			if f == nil {
				app.Log("news", "Future returned nil for %s (%s)", key, ftr)
				return
			}
			// Access the price, which may panic if Quote struct is malformed
			price := f.Quote.RegularMarketPrice
			if price > 0 {
				prices[key] = price
			}
		}()
	}

	app.Log("news", "Finished getting all prices")
	return prices
}

var tickers = []string{"GBP", "XLM", "ETH", "BTC", "PAXG"}

var futures = map[string]string{
	"OIL":      "CL=F",
	"GOLD":     "GC=F",
	"COFFEE":   "KC=F",
	"OATS":     "ZO=F",
	"WHEAT":    "KE=F",
	"SILVER":   "SI=F",
	"COPPER":   "HG=F",
	"CORN":     "ZC=F",
	"SOYBEANS": "ZS=F",
}

var futuresKeys = []string{"OIL", "OATS", "COFFEE", "WHEAT", "GOLD"}

// ContentParser functions clean up feed descriptions
type ContentParser struct {
	Name      string
	FeedNames []string // Apply to these feeds only (empty = all feeds)
	Parse     func(string) string
}

var contentParsers = []ContentParser{
	{
		Name:      "Strip HackerNews Comments-Only Descriptions",
		FeedNames: []string{"Hacker News", "Dev"},
		Parse: func(desc string) string {
			desc = strings.TrimSpace(desc)
			desc = strings.TrimPrefix(desc, "<![CDATA[")
			desc = strings.TrimSuffix(desc, "]]>")
			desc = strings.TrimSpace(desc)

			if strings.HasPrefix(desc, `<a href="https://news.ycombinator.com/item?id=`) &&
				strings.HasSuffix(desc, `">Comments</a>`) {
				return ""
			}

			if desc == "Comments" {
				return ""
			}

			return desc
		},
	},
	{
		Name: "Strip TechCrunch Copyright",
		Parse: func(desc string) string {
			return strings.Replace(desc, "© 2025 TechCrunch. All rights reserved. For personal use only.", "", -1)
		},
	},
	{
		Name: "Remove Images",
		Parse: func(desc string) string {
			return regexp.MustCompile(`<img .*>`).ReplaceAllString(desc, "")
		},
	},
	{
		Name: "Extract First Paragraph",
		Parse: func(desc string) string {
			parts := strings.Split(desc, "</p>")
			if len(parts) > 0 {
				return strings.Replace(parts[0], "<p>", "", 1)
			}
			return desc
		},
	},
	{
		Name: "Sanitize HTML",
		Parse: func(desc string) string {
			return sanitize.HTML(desc)
		},
	},
}

// applyContentParsers applies all relevant parsers to a description
func applyContentParsers(desc string, feedName string) string {
	for _, parser := range contentParsers {
		if len(parser.FeedNames) > 0 {
			matched := false
			for _, name := range parser.FeedNames {
				if name == feedName {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		desc = parser.Parse(desc)
	}
	return desc
}

func saveHtml(head, content []byte) {
	if len(content) == 0 {
		return
	}
	body := fmt.Sprintf(`<div id="topics">%s</div><div>%s</div>`, string(head), string(content))
	topicsHtml = string(head)
	headlinesAndContentHtml = string(content)
	page := app.RenderHTML("News", "Read the news", body)
	data.SaveFile("news.html", page)
	data.SaveFile("topics.html", topicsHtml)
	data.SaveFile("headlines_content.html", headlinesAndContentHtml)
}

func loadFeed() {
	// Try loading from local disk first (user edits), fall back to embedded
	var bytes []byte
	var err error
	var selectionSignature string

	for _, loc := range []string{"news/feeds.json", "feeds.json"} {
		if b, e := os.ReadFile(loc); e == nil {
			bytes = b
			break
		}
	}
	if bytes == nil {
		bytes, err = f.ReadFile("feeds.json")
		if err != nil {
			app.Log("news", "Error reading embedded feeds.json: %v", err)
		}
	}
	if bytes == nil {
		bytes = []byte("{}")
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Reset feeds map to avoid stale entries
	feeds = make(map[string]map[string]string)

	if err := json.Unmarshal(bytes, &feeds); err != nil {
		app.Log("news", "Error parsing feeds.json: %v", err)
	}

	// Filter sources based on settings
	selected := config.Get().NewsSources
	selectionSignature = signatureFromSelection(selected)
	if len(selected) > 0 {
		allowed := make(map[string]bool, len(selected))
		for _, s := range selected {
			allowed[s] = true
		}

		for category, sources := range feeds {
			for name := range sources {
				id := category + "|" + name
				if !allowed[id] {
					delete(sources, name)
				}
			}
			if len(sources) == 0 {
				delete(feeds, category)
			}
		}
	}

	// Record the signature of the currently loaded selection
	sourcesSignature = selectionSignature
}

// AvailableSources returns the current default feed map
func AvailableSources() map[string]map[string]string {
	var bytes []byte
	var err error

	for _, loc := range []string{"news/feeds.json", "feeds.json"} {
		if b, e := os.ReadFile(loc); e == nil {
			bytes = b
			break
		}
	}
	if bytes == nil {
		bytes, err = f.ReadFile("feeds.json")
		if err != nil {
			return map[string]map[string]string{}
		}
	}

	var m map[string]map[string]string
	if err := json.Unmarshal(bytes, &m); err != nil {
		return map[string]map[string]string{}
	}
	return m
}

func backoff(attempts int) time.Duration {
	if attempts > 13 {
		return time.Hour
	}
	return time.Duration(math.Pow(float64(attempts), math.E)) * time.Millisecond * 100
}

func signatureFromSelection(sel []string) string {
	if len(sel) == 0 {
		return "all"
	}
	cp := append([]string(nil), sel...)
	sort.Strings(cp)
	return strings.Join(cp, ",")
}

// ensureFeedsFresh compares the current selection with loaded feeds and requests a refresh if needed.
func ensureFeedsFresh() {
	desiredSig := signatureFromSelection(config.Get().NewsSources)

	mutex.RLock()
	currentSig := sourcesSignature
	lastReq := lastRefreshRequest
	mutex.RUnlock()

	if desiredSig != currentSig && time.Since(lastReq) > time.Minute {
		// schedule a refresh without blocking callers
		go Refresh()
		mutex.Lock()
		lastRefreshRequest = time.Now()
		mutex.Unlock()
	}
}

func getMetadataPath(uri string) string {
	itemID := fmt.Sprintf("%x", md5.Sum([]byte(uri)))[:16]
	return filepath.Join("news", "metadata", itemID+".json")
}

func loadCachedMetadata(uri string) (*Metadata, bool) {
	path := getMetadataPath(uri)
	var md Metadata
	if err := data.LoadJSON(path, &md); err != nil {
		return nil, false
	}
	return &md, true
}

func saveCachedMetadata(uri string, md *Metadata) {
	path := getMetadataPath(uri)
	if err := data.SaveJSON(path, md); err != nil {
		app.Log("news", "Error saving metadata: %v", err)
	}
}

func getMetadata(uri string) (*Metadata, error) {
	// Check cache first
	if cached, exists := loadCachedMetadata(uri); exists {
		return cached, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	d, err := goquery.NewDocument(u.String())
	if err != nil {
		return nil, err
	}

	g := &Metadata{
		Created: time.Now().UnixNano(),
		Url:     uri,
	}

	check := func(p []string) bool {
		if p[0] == "twitter" {
			return true
		}
		if p[0] == "og" {
			return true
		}

		return false
	}

	for _, node := range d.Find("meta").Nodes {
		if len(node.Attr) < 2 {
			continue
		}

		p := strings.Split(node.Attr[0].Val, ":")
		if !check(p) {
			p = strings.Split(node.Attr[1].Val, ":")
			if !check(p) {
				continue
			}
			node.Attr = node.Attr[1:]
			if len(node.Attr) < 2 {
				continue
			}
		}

		switch p[1] {
		case "site_name":
			g.Site = node.Attr[1].Val
		case "site":
			if len(g.Site) == 0 {
				g.Site = node.Attr[1].Val
			}
		case "title":
			g.Title = node.Attr[1].Val
		case "description":
			g.Description = node.Attr[1].Val
		case "card", "type":
			g.Type = node.Attr[1].Val
		case "url":
			g.Url = node.Attr[1].Val
		case "image":
			if len(p) > 2 && p[2] == "src" {
				g.Image = node.Attr[1].Val
			} else if len(p) > 2 {
				continue
			} else if len(g.Image) == 0 {
				g.Image = node.Attr[1].Val
			}

			if len(g.Image) > 0 && g.Image[0] == '/' {
				g.Image = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, g.Image)
			}
		}
	}

	// attempt to get the content
	var fn func(*nethtml.Node)

	fn = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode && len(node.Data) > 0 {
			first := node.Data[0]
			last := node.Data[len(node.Data)-1]

			data := sanitize.HTML(node.Data)

			if unicode.IsUpper(rune(first)) && last == '.' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else if first == '"' && last == '"' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else {
				g.Content += fmt.Sprintf(` %s`, data)
			}
		}

		if node.FirstChild != nil {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				fn(c)
			}
		}
	}

	if strings.Contains(u.String(), "cnbc.com") {
		for _, node := range d.Find(".ArticleBody-articleBody").Nodes {
			fn(node)
		}
	}

	// Attempt to pull main article content using common containers if still empty
	if len(strings.TrimSpace(g.Content)) == 0 {
		contentSelectors := []string{
			"article",
			"main article",
			".ArticleBody-articleBody",
			".article-body",
			".article__content",
			"main",
		}

		for _, sel := range contentSelectors {
			section := d.Find(sel)
			if section.Length() == 0 {
				continue
			}
			if html, err := section.First().Html(); err == nil {
				if text := htmlToText(html); text != "" {
					g.Content = text
					break
				}
			}
		}
	}

	if strings.TrimSpace(g.Content) == "" {
		var paras []string
		d.Find("p").EachWithBreak(func(i int, s *goquery.Selection) bool {
			txt := strings.TrimSpace(s.Text())
			if txt != "" {
				paras = append(paras, sanitize.HTML(txt))
			}
			return len(paras) < 5
		})
		g.Content = strings.Join(paras, " ")
	}

	if g.Url == "" {
		g.Url = uri
	}

	saveCachedMetadata(uri, g)

	return g, nil
}

func fetchQuranReminder() (string, error) {
	app.Log("news", "Getting Reminder at %s", time.Now().String())
	uri := "https://reminder.dev/api/daily/latest"

	resp, err := http.Get(uri)
	if err != nil {
		return "", err
	}

	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var val map[string]interface{}

	err = json.Unmarshal(b, &val)
	if err != nil {
		return "", err
	}

	link := fmt.Sprintf("https://reminder.dev%s", val["links"].(map[string]interface{})["verse"].(string))

	html := fmt.Sprintf(`<div class="verse">%s</div><a href="%s">More</a>`, val["verse"], link)
	return html, nil
}

func fetchBibleReminder() (string, error) {
	app.Log("news", "Getting Bible Reminder at %s", time.Now().String())
	req, err := http.NewRequest("GET", "https://beta.ourmanna.com/api/v1/get?format=json&order=daily", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var payload struct {
		Verse struct {
			Details struct {
				Text      string `json:"text"`
				Reference string `json:"reference"`
			} `json:"details"`
		} `json:"verse"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	text := strings.TrimSpace(payload.Verse.Details.Text)
	ref := strings.TrimSpace(payload.Verse.Details.Reference)
	if text == "" && ref == "" {
		return "", fmt.Errorf("empty verse from OurManna")
	}

	verse := text
	if ref != "" {
		verse += " — " + ref
	}
	return fmt.Sprintf(`<div class="verse">%s</div>`, verse), nil
}

func fetchZenReminder() (string, error) {
	app.Log("news", "Getting Zen Quote at %s", time.Now().String())
	resp, err := http.Get("https://zenquotes.io/api/random")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var payload []struct {
		Q string `json:"q"`
		A string `json:"a"`
		H string `json:"h"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("zenquotes returned empty response")
	}
	q := strings.TrimSpace(payload[0].Q)
	a := strings.TrimSpace(payload[0].A)

	quote := q
	if a != "" {
		quote += " — " + a
	}
	if quote == "" {
		return "", fmt.Errorf("zenquotes returned empty quote")
	}

	return fmt.Sprintf(`<div class="verse">%s</div>`, quote), nil
}

func getReminder() {
	// Refresh source in case settings changed
	reminderSource = config.Get().ReminderSource
	source := reminderSource
	var html string
	var err error

	switch source {
	case "bible":
		html, err = fetchBibleReminder()
	case "zen":
		html, err = fetchZenReminder()
	default:
		html, err = fetchQuranReminder()
	}

	if err != nil {
		app.Log("news", "Error getting reminder: %v", err)
		time.Sleep(time.Minute)
		go getReminder()
		return
	}

	mutex.Lock()
	data.SaveFile("reminder.html", html)
	reminderHtml = html
	reminderFetched = time.Now()
	mutex.Unlock()

	time.Sleep(time.Hour)

	go getReminder()
}

func parseFeed() {
	defer func() {
		if r := recover(); r != nil {
			app.Log("news", "Recovered from panic in feed parser: %v", r)
			// You can perform cleanup, logging, or other error handling here.
			// For example, you might send an error to a channel to notify main.
			debug.PrintStack()

			app.Log("news", "Relaunching feed parser in 1 minute")
			time.Sleep(time.Minute)

			go parseFeed()
		}
	}()

	app.Log("news", "Parsing feed at %s", time.Now().String())
	p := gofeed.NewParser()
	p.UserAgent = "Mu/0.1"

	content := []byte{}
	stats := map[string]Feed{}
	feedSnapshot := make(map[string]map[string]string)

	var sorted []string

	mutex.RLock()
	for category, sources := range feeds {
		sorted = append(sorted, category)
		inner := make(map[string]string, len(sources))
		for name, url := range sources {
			inner[name] = url
			feedID := category + "|" + name
			if stat, ok := status[feedID]; ok && stat != nil {
				stats[feedID] = *stat
			}
		}
		feedSnapshot[category] = inner
	}
	mutex.RUnlock()

	sort.Strings(sorted)
	// build topics head for rendering and caching
	head := []byte(app.Head("news", sorted))

	// all the news
	var news []*Post
	var headlines []*Post

	for _, category := range sorted {
		catSources := feedSnapshot[category]
		for sourceName, feedUrl := range catSources {
			feedID := category + "|" + sourceName

			// check last attempt
			stat, ok := stats[feedID]
			if !ok {
				stat = Feed{
					Name: feedID,
					URL:  feedUrl,
				}

				mutex.Lock()
				status[feedID] = &stat
				mutex.Unlock()
			}

			if stat.Attempts > 0 && time.Until(stat.Backoff) > 0 {
				continue
			}

			// parse the feed
			f, err := p.ParseURL(feedUrl)
			if err != nil {
				stat.Attempts++
				stat.Error = err
				stat.Backoff = time.Now().Add(backoff(stat.Attempts))
				app.Log("news", "Error parsing %s (%s): %v", sourceName, feedUrl, err)

				mutex.Lock()
				status[feedID] = &stat
				mutex.Unlock()
				continue
			}

			mutex.Lock()
			stat.Attempts = 0
			stat.Backoff = time.Time{}
			stat.Error = nil
			status[feedID] = &stat
			mutex.Unlock()

			content = append(content, []byte(`<div class=section>`)...)
			content = append(content, []byte(`<hr id="`+category+`" class="anchor">`)...)
			content = append(content, []byte(`<h1>`+category+` - `+sourceName+`</h1>`)...)

			for i, item := range f.Items {
				// only 10 items
				if i >= 10 {
					break
				}

				item.Description = applyContentParsers(item.Description, sourceName)

				link := item.Link

				app.Log("news", "Checking link %s", link)

				if strings.HasPrefix(link, "https://themwl.org/ar/") {
					link = strings.Replace(link, "themwl.org/ar/", "themwl.org/en/", 1)
					app.Log("news", "Replacing mwl ar link %s -> %s", item.Link, link)
				}

				// get meta
				md, err := getMetadata(link)
				if err != nil {
					app.Log("news", "Error parsing %s: %v", link, err)
					continue
				}

				if strings.Contains(link, "themwl.org") {
					item.Title = md.Title
				}

				// extracted content using goquery
				if len(md.Content) > 0 && len(item.Content) == 0 {
					item.Content = md.Content
				}

				// Handle nil PublishedParsed
				var postedAt time.Time
				if item.PublishedParsed != nil {
					postedAt = *item.PublishedParsed
				} else {
					postedAt = time.Now()
				}

				// Clean up description HTML
				desc := item.Description

				// Convert plain text newlines to em dashes
				if !strings.Contains(desc, "<") {
					desc = strings.ReplaceAll(desc, "\n", " — ")
				}

				cleanDescription := htmlToText(desc)

				maxLen := 250
				if len(cleanDescription) > maxLen {
					truncated := cleanDescription[:maxLen]
					if idx := strings.Index(truncated, ". "); idx > 0 {
						cleanDescription = truncated[:idx+1]
					} else if idx := strings.Index(truncated, ".\n"); idx > 0 {
						cleanDescription = truncated[:idx+1]
					} else {
						cleanDescription = truncated[:maxLen-3] + "..."
					}
				} else {
					if idx := strings.Index(cleanDescription, ". "); idx > 0 {
						cleanDescription = cleanDescription[:idx+1]
					} else if idx := strings.Index(cleanDescription, ".\n"); idx > 0 {
						cleanDescription = cleanDescription[:idx+1]
					}
				}

				// Generate stable ID from URL hash - more reliable than GUID which can change
				itemID := fmt.Sprintf("%x", md5.Sum([]byte(link)))[:16]

				// create post
				post := &Post{
					ID:          itemID,
					Title:       item.Title,
					Description: cleanDescription,
					URL:         link,
					Published:   item.Published,
					PostedAt:    postedAt,
					Category:    category,
					Image:       md.Image,
					Content:     item.Content,
				}

				news = append(news, post)

				// Index the article for search/RAG
				data.Index(
					itemID,
					"news",
					item.Title,
					cleanDescription+" "+item.Content,
					map[string]interface{}{
						"url":       link,
						"category":  category,
						"source":    sourceName,
						"published": item.Published,
						"image":     md.Image,
					},
				)

				var val string

				if len(md.Image) > 0 {
					val = fmt.Sprintf(`
	<div id="%s" class="news">
	  <div style="display: inline-block; width: 100%%;">
	    <a href="%s" rel="noopener noreferrer" target="_blank" style="text-decoration: none;">
	      <img class="cover" src="%s">
	    </a>
	    <div class="blurb">
	      <a href="%s" rel="noopener noreferrer" target="_blank" style="text-decoration: none;">
	        <span class="title">%s</span>
	      </a>
	      <div class="description collapsed" onclick="toggleDescription(this)">%s</div>
	    </div>
	  </div>
	  <div style="font-size: 0.8em; margin-top: 5px; color: #777;">%s</div>
				`, item.GUID, link, md.Image, link, item.Title, cleanDescription, getSummary(post))
				} else {
					val = fmt.Sprintf(`
	<div id="%s" class="news">
	  <div style="display: inline-block; width: 100%%;">
	    <a href="%s" rel="noopener noreferrer" target="_blank" style="text-decoration: none;">
	      <img class="cover">
	    </a>
	    <div class="blurb">
	      <a href="%s" rel="noopener noreferrer" target="_blank" style="text-decoration: none;">
	        <span class="title">%s</span>
	      </a>
	      <div class="description collapsed" onclick="toggleDescription(this)">%s</div>
	    </div>
	  </div>
	  <div style="font-size: 0.8em; margin-top: 5px; color: #777;">%s</div>
				`, item.GUID, link, link, item.Title, cleanDescription, getSummary(post))
				}

				// close div
				val += `</div>`

				content = append(content, []byte(val)...)

				if i > 0 {
					continue
				}

				// add to headlines / 1 per category
				headlines = append(headlines, post)
			}

			content = append(content, []byte(`</div>`)...)
		}
	}

	headline := []byte(`<div class=section>`)

	// get crypto prices
	newPrices := getPrices()

	if newPrices != nil {
		// Cache the prices for the markets page
		mutex.Lock()
		cachedPrices = newPrices
		mutex.Unlock()

		info := []byte(`<div class="markets-wrapper"><div id="tickers">`)

		for _, ticker := range tickers {
			price := newPrices[ticker]
			line := fmt.Sprintf(`<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div><div id="futures">`)...)

		for _, ticker := range futuresKeys {
			price := newPrices[ticker]
			line := fmt.Sprintf(`<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div></div>`)...)
		marketsHtml = string(info)

		// Index all prices for search/RAG
		for ticker, price := range newPrices {
			data.Index(
				"market_"+ticker,
				"market",
				ticker,
				fmt.Sprintf("$%.2f", price),
				map[string]interface{}{
					"ticker": ticker,
					"price":  price,
				},
			)
		}
	}

	// create the headlines
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].PostedAt.After(headlines[j].PostedAt)
	})

	for _, h := range headlines {
		val := fmt.Sprintf(`
			<div class="headline">
			<a href="/news#%s" class="category">%s</a>
			  <a href="%s" rel="noopener noreferrer" target="_blank">
			   <span class="title">%s</span>
			  </a>
			 <div class="description collapsed" onclick="toggleDescription(this)" style="margin-top: 5px;">%s</div>
			 <div style="font-size: 0.8em; margin-top: 5px; color: #777;">%s</div>
			`, h.Category, h.Category, h.URL, h.Title, h.Description, getSummary(h))

		// close val
		val += `</div>`
		headline = append(headline, []byte(val)...)
	}

	headline = append(headline, []byte(`</div>`)...)

	// set the headline
	content = append(headline, content...)

	mutex.Lock()

	// set the feed
	feed = news
	// set the headlines
	headlinesHtml = string(headline)
	// save it
	saveHtml(head, content)
	// save the headlines
	data.SaveFile("headlines.html", headlinesHtml)
	// save markets
	data.SaveFile("markets.html", marketsHtml)

	// save the prices as JSON for persistence
	data.SaveJSON("prices.json", cachedPrices)

	mutex.Unlock()

	// wait an hour
	time.Sleep(time.Hour)

	// go again
	go parseFeed()
}

func Load() {
	reminderSource = config.Get().ReminderSource
	// load headlines
	b, _ := data.LoadFile("headlines.html")
	headlinesHtml = string(b)

	// load markets
	b, _ = data.LoadFile("markets.html")
	marketsHtml = string(b)

	// load cached prices
	b, _ = data.LoadFile("prices.json")
	if len(b) > 0 {
		var prices map[string]float64
		if err := json.Unmarshal(b, &prices); err == nil {
			mutex.Lock()
			cachedPrices = prices
			mutex.Unlock()
		}
	}

	b, _ = data.LoadFile("reminder.html")

	reminderHtml = string(b)

	// load news
	// kept for potential future use (page cache), but not stored in memory

	// load topics
	b, _ = data.LoadFile("topics.html")
	topicsHtml = string(b)

	// load headlines and content
	b, _ = data.LoadFile("headlines_content.html")
	headlinesAndContentHtml = string(b)

	// load the feeds
	loadFeed()

	go parseFeed()

	go getReminder()

	// Refresh feeds automatically when settings change
	config.RegisterUpdateHook(func(config.Settings) {
		app.Log("news", "Settings changed, refreshing news...")
		go Refresh()
	})
}

// Refresh reloads feed configurations and triggers a fetch
func Refresh() {
	app.Log("news", "Refreshing news feeds...")
	loadFeed()

	// Reset status and cached content to force immediate fetch and rebuild
	mutex.Lock()
	status = make(map[string]*Feed)
	topicsHtml = ""
	headlinesAndContentHtml = ""
	headlinesHtml = ""
	feed = nil
	marketsHtml = ""
	lastRefreshRequest = time.Time{}
	mutex.Unlock()

	go parseFeed()
}

func Headlines() string {
	ensureFeedsFresh()

	mutex.RLock()
	defer mutex.RUnlock()

	return headlinesHtml
}

func Markets() string {
	ensureFeedsFresh()

	mutex.RLock()
	defer mutex.RUnlock()

	return marketsHtml
}

func Reminder() string {
	ensureFeedsFresh()

	cfg := config.Get()

	// Decide if we should fetch now (source changed, empty, or stale > 1h)
	shouldFetch := false

	mutex.RLock()
	shouldFetch = reminderHtml == "" || reminderSource != cfg.ReminderSource || time.Since(reminderFetched) > time.Hour
	mutex.RUnlock()

	if shouldFetch {
		refreshReminder(cfg.ReminderSource)
	}

	mutex.RLock()
	html := reminderHtml
	mutex.RUnlock()
	return html
}

func refreshReminder(source string) {
	reminderMutex.Lock()
	defer reminderMutex.Unlock()

	html, err := func() (string, error) {
		switch source {
		case "bible":
			return fetchBibleReminder()
		case "zen":
			return fetchZenReminder()
		default:
			return fetchQuranReminder()
		}
	}()

	if err != nil || html == "" {
		app.Log("news", "Reminder refresh error: %v", err)
		return
	}

	mutex.Lock()
	reminderHtml = html
	reminderSource = source
	reminderFetched = time.Now()
	mutex.Unlock()
}

func Handler(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	defer mutex.RUnlock()

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		resp := map[string]interface{}{
			"feed": feed,
		}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Build page: topics, then headlines and content
	content := fmt.Sprintf(`
		<div id="topics">%s</div>
		<h2>Headlines</h2>
		<div>%s</div>
	`, topicsHtml, headlinesAndContentHtml)

	page := app.RenderHTMLForRequest("News", "Latest news headlines", content, r)
	w.Write([]byte(page))
}

// GetAllPrices returns all cached prices
func GetAllPrices() map[string]float64 {
	mutex.RLock()
	defer mutex.RUnlock()

	// Return a copy to avoid concurrent map access
	prices := make(map[string]float64)
	for k, v := range cachedPrices {
		prices[k] = v
	}
	return prices
}

// GetHomepageTickers returns the list of tickers displayed on homepage
func GetHomepageTickers() []string {
	return append([]string{}, tickers...)
}

// GetHomepageFutures returns the list of futures displayed on homepage
func GetHomepageFutures() []string {
	return append([]string{}, futuresKeys...)
}
