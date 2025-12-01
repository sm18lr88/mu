package news

import "testing"

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Simple", "<p>Hello</p>", "Hello"},
		{"Nested", "<div><p>Test</p></div>", "Test"},
		{"Multiple Tags", "<p>One</p><p>Two</p>", "One Two"},
		{"Attributes", `<a href="foo">Link</a>`, "Link"},
		{"Empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := htmlToText(tt.input); got != tt.want {
				t.Errorf("htmlToText() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDomain(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.google.com/search", "google.com"},
		{"http://news.ycombinator.com", "news.ycombinator.com"},
		{"https://coindesk.com", "coindesk.com"},
		{"https://user.github.io/repo", "user.github.io"},
	}

	for _, tt := range tests {
		if got := getDomain(tt.url); got != tt.want {
			t.Errorf("getDomain(%s) = %v, want %v", tt.url, got, tt.want)
		}
	}
}
