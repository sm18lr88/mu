package chat

import (
	"strings"
	"testing"

	"mu/data"
)

func TestBuildHistoryFromJSONString(t *testing.T) {
	raw := `[{"prompt":"p1","answer":"a1"},{"prompt":"p2","answer":"a2"},{"prompt":"p3","answer":"a3"},{"prompt":"p4","answer":"a4"},{"prompt":"p5","answer":"a5"},{"prompt":"p6","answer":"a6"}]`

	h := BuildHistory(raw)

	if len(h) != 5 {
		t.Fatalf("expected 5 items, got %d", len(h))
	}

	if h[0].Prompt != "p2" || h[4].Prompt != "p6" {
		t.Fatalf("expected last five prompts p2..p6, got %+v", h)
	}
}

func TestBuildPromptWithTopicAndRAG(t *testing.T) {
	data.ClearIndex()
	data.Index("id1", "news", "Tech win", "AI hits milestone", map[string]interface{}{"url": "http://example.com"})

	prompt, searchQuery, ragEntries := BuildPrompt("What's new?", "Tech", nil)

	if searchQuery != "Tech What's new?" {
		t.Fatalf("unexpected search query: %s", searchQuery)
	}
	if len(ragEntries) != 1 {
		t.Fatalf("expected 1 rag entry, got %d", len(ragEntries))
	}
	if got := strings.Join(prompt.Rag, " "); !strings.Contains(got, "Tech win") {
		t.Fatalf("expected rag context to include title, got %q", got)
	}
}

func TestRenderPromptTextIncludesQuestionAndSystem(t *testing.T) {
	p := &Prompt{
		Rag: []string{"context line"},
		Context: History{
			{Prompt: "hi", Answer: "hello"},
		},
		Question: "Q?",
	}

	txt, err := RenderPromptText(p)
	if err != nil {
		t.Fatalf("RenderPromptText error: %v", err)
	}

	if !strings.Contains(txt, "Question: Q?") {
		t.Fatalf("missing question in prompt text:\n%s", txt)
	}
	if !strings.Contains(txt, "User: hi") || !strings.Contains(txt, "Assistant: hello") {
		t.Fatalf("missing conversation in prompt text:\n%s", txt)
	}
	if !strings.Contains(txt, "Answer (markdown") {
		t.Fatalf("missing answer directive in prompt text:\n%s", txt)
	}
}
