package app

import (
	"strings"
	"testing"
	"time"
)

func TestTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{"Just now", time.Time{}, "just now"},
		{"Seconds ago", now.Add(-10 * time.Second), "10 secs ago"},
		{"Minute ago", now.Add(-1 * time.Minute), "1 minute ago"},
		{"Minutes ago", now.Add(-10 * time.Minute), "10 minutes ago"},
		{"Hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"Hours ago", now.Add(-5 * time.Hour), "5 hours ago"},
		{"Day ago", now.Add(-25 * time.Hour), "1 day ago"},
		{"Days ago", now.Add(-48 * time.Hour), "2 days ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TimeAgo(tt.input); got != tt.expected {
				t.Errorf("TimeAgo() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRenderHTML(t *testing.T) {
	title := "Test Title"
	desc := "Test Desc"
	content := "<p>Hello</p>"

	output := RenderHTML(title, desc, content)

	if !strings.Contains(output, "<title>Test Title | Mu</title>") {
		t.Error("RenderHTML missing title")
	}
	if !strings.Contains(output, "Test Desc") {
		t.Error("RenderHTML missing description")
	}
	if !strings.Contains(output, "Hello") {
		t.Error("RenderHTML missing content")
	}
}
