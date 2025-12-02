package chat

import (
	"context"
	"testing"
)

type fakeBackend struct {
	resp string
	err  error
}

func (f *fakeBackend) Ask(ctx context.Context, prompt *Prompt) (string, error) {
	return f.resp, f.err
}

func TestModelGenerateUsesOverride(t *testing.T) {
	reset := setBackendOverride(&fakeBackend{resp: "ok"})
	defer reset()

	m := &Model{}
	out, err := m.Generate(context.Background(), &Prompt{Question: "hi"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected response: %q", out)
	}
}

func TestModelGenerateValidatesQuestion(t *testing.T) {
	m := &Model{}
	_, err := m.Generate(context.Background(), &Prompt{Question: "   "})
	if err == nil {
		t.Fatalf("expected error for empty question")
	}
	if err.Error() != "question is empty" {
		t.Fatalf("unexpected error: %v", err)
	}
}
