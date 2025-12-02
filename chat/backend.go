package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"mu/codex"
	"mu/config"
)

// Backend defines the minimal API for chat backends.
type Backend interface {
	Ask(ctx context.Context, prompt *Prompt) (string, error)
}

// disabledBackend is returned when no usable backend is available.
type disabledBackend struct {
	reason string
}

func (d *disabledBackend) Ask(ctx context.Context, prompt *Prompt) (string, error) {
	return "", errors.New(d.reason)
}

// backendOverride lets tests and the CLI inject a stub backend.
// Not exposed outside this package in production code paths.
var backendOverride Backend

// codexBackend uses the local Codex CLI.
type codexBackend struct{}

func (c *codexBackend) Ask(ctx context.Context, prompt *Prompt) (string, error) {
	systemPromptText, err := buildSystemPrompt(prompt)
	if err != nil {
		return "", err
	}

	textPrompt := buildPromptText(systemPromptText, prompt)

	opt := codex.DefaultOptions()
	opt.WorkDir = "."
	model, thinking := currentModelThinking()
	opt.Model = model             // empty uses Codex defaults
	opt.ReasoningLevel = thinking // empty uses Codex defaults

	return codex.Ask(ctx, textPrompt, opt)
}

// fanarBackend preserves the existing Fanar flow as a fallback.
type fanarBackend struct{}

func (f *fanarBackend) Ask(ctx context.Context, prompt *Prompt) (string, error) {
	apiKey := config.Get().FanarAPIKey
	if len(apiKey) == 0 {
		return "", fmt.Errorf("FANAR_API_KEY not set")
	}

	systemPromptText, err := buildSystemPrompt(prompt)
	if err != nil {
		return "", err
	}

	messages := buildFanarMessages(systemPromptText, prompt)

	fanarReq := map[string]interface{}{
		"model":    "Fanar",
		"messages": messages,
	}

	body, err := json.Marshal(fanarReq)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.fanar.qa/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fanar backend error: status %d: %s", resp.StatusCode, string(respBody))
	}

	var fanarResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &fanarResp); err != nil {
		return "", err
	}

	var content string
	if len(fanarResp.Choices) > 0 {
		content = fanarResp.Choices[0].Message.Content
	}
	if fanarResp.Error != nil {
		return "", fmt.Errorf("%v", fanarResp.Error)
	}
	if content == "" {
		return "", fmt.Errorf("fanar returned empty content")
	}

	return content, nil
}

// selectBackend chooses the backend based on env / availability.
func selectBackend() Backend {
	if backendOverride != nil {
		return backendOverride
	}

	fanarKey := config.Get().FanarAPIKey
	noBackendReason := "chat backend unavailable: install Codex CLI (npm i -g @openai/codex && codex login) or set FANAR_API_KEY"

	switch os.Getenv("MU_CHAT_BACKEND") {
	case "codex":
		if codex.HasCodex() {
			return &codexBackend{}
		}
		return &disabledBackend{reason: noBackendReason}
	case "fanar":
		if fanarKey != "" {
			return &fanarBackend{}
		}
		return &disabledBackend{reason: noBackendReason}
	}

	if codex.HasCodex() {
		return &codexBackend{}
	}

	if fanarKey != "" {
		return &fanarBackend{}
	}

	return &disabledBackend{reason: noBackendReason}
}

// setBackendOverride is used by tests to swap in a fake backend.
// It returns a reset function to restore the previous backend.
func setBackendOverride(b Backend) (reset func()) {
	prev := backendOverride
	backendOverride = b
	return func() {
		backendOverride = prev
	}
}

func isBackendDisabled(b Backend) bool {
	_, ok := b.(*disabledBackend)
	return ok
}

func disabledReason(b Backend) string {
	if d, ok := b.(*disabledBackend); ok {
		return d.reason
	}
	return ""
}

// chatContext returns a context with a conservative timeout for LLM calls.
func chatContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := 90 * time.Second
	return context.WithTimeout(parent, timeout)
}
