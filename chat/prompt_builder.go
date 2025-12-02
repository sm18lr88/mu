package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/data"
)

// BuildHistory converts any loosely-typed history payload (JSON, map, slice)
// into a History slice, keeping only the last five exchanges.
func BuildHistory(raw interface{}) History {
	history := History{}

	appendMsg := func(prompt, answer string) {
		if prompt == "" && answer == "" {
			return
		}
		history = append(history, Message{
			Prompt: prompt,
			Answer: answer,
		})
	}

	switch v := raw.(type) {
	case nil:
		// nothing to do
	case History:
		history = append(history, v...)
	case []Message:
		history = append(history, v...)
	case Message:
		appendMsg(v.Prompt, v.Answer)
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			// Try parsing as JSON array first
			var arr []interface{}
			if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
				return BuildHistory(arr)
			}

			// Fallback: try direct []Message
			var msgs []Message
			if err := json.Unmarshal([]byte(trimmed), &msgs); err == nil {
				return BuildHistory(msgs)
			}
		}
	case []byte:
		return BuildHistory(string(v))
	case []interface{}:
		for _, item := range v {
			switch m := item.(type) {
			case map[string]interface{}:
				appendMsg(fmt.Sprintf("%v", m["prompt"]), fmt.Sprintf("%v", m["answer"]))
			case map[string]string:
				appendMsg(m["prompt"], m["answer"])
			case Message:
				appendMsg(m.Prompt, m.Answer)
			default:
				// ignore unknown shapes
			}
		}
	}

	// Keep only the most recent five exchanges
	if len(history) > 5 {
		history = history[len(history)-5:]
	}

	return history
}

// BuildPrompt assembles a Prompt with RAG context and trimmed history.
// It returns the prompt along with the search query and the matched entries.
func BuildPrompt(question, topic string, history History) (*Prompt, string, []*data.IndexEntry) {
	q := strings.TrimSpace(question)
	t := strings.TrimSpace(topic)

	searchQuery := q
	if t != "" {
		searchQuery = t + " " + q
	}

	ragEntries := data.Search(searchQuery, 3)
	ragContext := formatRagContext(ragEntries)

	return &Prompt{
		Rag:      ragContext,
		Context:  history,
		Question: q,
	}, searchQuery, ragEntries
}

func formatRagContext(entries []*data.IndexEntry) []string {
	var ragContext []string
	for _, entry := range entries {
		contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
		if len(contextStr) > 500 {
			contextStr = contextStr[:500]
		}
		if url, ok := entry.Metadata["url"].(string); ok && len(url) > 0 {
			contextStr += fmt.Sprintf(" (Source: %s)", url)
		}
		ragContext = append(ragContext, contextStr)
	}
	return ragContext
}

// RenderPromptText returns the final text prompt that will be sent to the backend.
// Useful for debugging CLI requests.
func RenderPromptText(p *Prompt) (string, error) {
	systemPromptText, err := buildSystemPrompt(p)
	if err != nil {
		return "", err
	}
	return buildPromptText(systemPromptText, p), nil
}
