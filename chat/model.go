package chat

import (
	"context"
	"fmt"
	"strings"
)

type Model struct{}

func (m *Model) Generate(ctx context.Context, prompt *Prompt) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if prompt == nil {
		return "", fmt.Errorf("prompt is nil")
	}
	if strings.TrimSpace(prompt.Question) == "" {
		return "", fmt.Errorf("question is empty")
	}

	backend := selectBackend()

	return backend.Ask(ctx, prompt)
}

func buildSystemPrompt(prompt *Prompt) (string, error) {
	if prompt.System != "" {
		return prompt.System, nil
	}

	sb := &strings.Builder{}
	if err := systemPrompt.Execute(sb, prompt.Rag); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func buildFanarMessages(systemPromptText string, prompt *Prompt) []map[string]string {
	messages := []map[string]string{
		{
			"role":    "system",
			"content": systemPromptText,
		},
	}

	for _, v := range prompt.Context {
		messages = append(messages, map[string]string{
			"role":    "user",
			"content": v.Prompt,
		})
		messages = append(messages, map[string]string{
			"role":    "assistant",
			"content": v.Answer,
		})
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": prompt.Question,
	})

	return messages
}

func buildPromptText(systemPromptText string, prompt *Prompt) string {
	var sb strings.Builder

	system := strings.TrimSpace(systemPromptText)
	if system != "" {
		sb.WriteString(system)
		sb.WriteString("\n\n")
	}

	if len(prompt.Context) > 0 {
		sb.WriteString("Conversation so far:\n")
		for _, msg := range prompt.Context {
			user := strings.TrimSpace(msg.Prompt)
			if user != "" {
				sb.WriteString("User: ")
				sb.WriteString(user)
				sb.WriteString("\n")
			}
			answer := strings.TrimSpace(msg.Answer)
			if answer != "" {
				sb.WriteString("Assistant: ")
				sb.WriteString(answer)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("User question: ")
	sb.WriteString(strings.TrimSpace(prompt.Question))
	sb.WriteString("\nProvide a concise, helpful answer in markdown.")

	return strings.TrimSpace(sb.String())
}
