package blog

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	tmpDir, _ := os.MkdirTemp("", "mu_test_blog")
	originalHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)

	code := m.Run()

	_ = os.Setenv("HOME", originalHome)
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestLinkify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			"Standard Link",
			"Check out https://google.com",
			`<a href="https://google.com" target="_blank" rel="noopener noreferrer">https://google.com</a>`,
		},
		{
			"Youtube Embed",
			"Watch https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			`<iframe src="/video?id=dQw4w9WgXcQ"`,
		},
		{
			"Newline conversion",
			"Hello\nWorld",
			"Hello<br>World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Linkify(tt.input)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("Linkify() = %v, expected to contain %v", got, tt.contains)
			}
		})
	}
}

func TestPostCRUD(t *testing.T) {
	posts = []*Post{}

	if err := CreatePost("Hello World", "This is a test post.", "tester", "123"); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	if len(posts) != 1 {
		t.Fatalf("Expected 1 post, got %d", len(posts))
	}

	id := posts[0].ID

	post := GetPost(id)
	if post == nil {
		t.Fatal("GetPost failed")
	}
	if post.Title != "Hello World" {
		t.Errorf("Expected title 'Hello World', got %s", post.Title)
	}

	if err := UpdatePost(id, "Updated Title", "Updated content"); err != nil {
		t.Fatalf("UpdatePost failed: %v", err)
	}

	post = GetPost(id)
	if post.Title != "Updated Title" {
		t.Error("Post update not reflected")
	}

	preview := Preview()
	if !strings.Contains(preview, "Updated Title") {
		t.Error("Preview does not contain updated title")
	}

	if err := DeletePost(id); err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}

	if len(posts) != 0 {
		t.Error("Post not deleted")
	}
}
