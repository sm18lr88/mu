package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"mu/codex"
	"mu/config"
)

// Backend defines the minimal API for chat backends.
type Backend interface {
	Ask(ctx context.Context, prompt *Prompt) (string, error)
}

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
	fanarKey := config.Get().FanarAPIKey

	switch os.Getenv("MU_CHAT_BACKEND") {
	case "codex":
		return &codexBackend{}
	case "fanar":
		return &fanarBackend{}
	}

	if _, err := exec.LookPath("codex"); err == nil {
		return &codexBackend{}
	}

	if fanarKey != "" {
		return &fanarBackend{}
	}

	return &codexBackend{}
}

// chatContext returns a context with a conservative timeout for LLM calls.
func chatContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := 90 * time.Second
	return context.WithTimeout(parent, timeout)
}
