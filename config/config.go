package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings holds user-provided API keys and other mutable configuration.
type Settings struct {
	YouTubeAPIKey  string   `json:"youtube_api_key"`
	FanarAPIKey    string   `json:"fanar_api_key"`
	ReminderSource string   `json:"reminder_source"`
	NewsSources    []string `json:"news_sources"`

	// Chat/summaries Codex controls
	ChatModel       string `json:"chat_model"`
	ChatThinking    string `json:"chat_thinking"`
	SummaryModel    string `json:"summary_model"`
	SummaryThinking string `json:"summary_thinking"`
}

var (
	mu       sync.RWMutex
	settings Settings
	hooks    []func(Settings)
)

func settingsPath() string {
	dir := os.ExpandEnv("$HOME/.mu")
	return filepath.Join(dir, "data", "settings.json")
}

// Load reads settings from disk. Missing files are ignored.
func Load() {
	mu.Lock()
	defer mu.Unlock()

	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return
	}

	_ = json.Unmarshal(b, &settings)
}

// Save writes current settings to disk.
func Save() error {
	mu.RLock()
	defer mu.RUnlock()

	b, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	path := settingsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	return os.WriteFile(path, b, 0o600)
}

// Get returns a copy of current settings, applying environment fallbacks.
func Get() Settings {
	mu.RLock()
	defer mu.RUnlock()

	s := settings

	if s.YouTubeAPIKey == "" {
		s.YouTubeAPIKey = os.Getenv("YOUTUBE_API_KEY")
	}
	if s.FanarAPIKey == "" {
		s.FanarAPIKey = os.Getenv("FANAR_API_KEY")
	}

	if s.ReminderSource == "" {
		s.ReminderSource = "quran"
	}

	// Empty NewsSources means "use defaults from feeds.json"

	return s
}

// RegisterUpdateHook registers a callback invoked after settings are successfully updated.
func RegisterUpdateHook(fn func(Settings)) {
	mu.Lock()
	defer mu.Unlock()
	hooks = append(hooks, fn)
}

// Update replaces settings and persists them.
func Update(new Settings) error {
	mu.Lock()
	settings = new
	// copy hooks to avoid holding lock while invoking
	copiedHooks := append([]func(Settings){}, hooks...)
	mu.Unlock()

	if err := Save(); err != nil {
		return err
	}

	for _, fn := range copiedHooks {
		// invoke hooks asynchronously to avoid blocking callers
		go fn(new)
	}
	return nil
}

// Update replaces settings and persists them (deprecated: kept for compatibility).
func UpdateLegacy(new Settings) error { // nolint:unused
	mu.Lock()
	settings = new
	mu.Unlock()
	return Save()
}
