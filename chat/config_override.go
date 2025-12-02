package chat

import "mu/config"

// summaryOverrides lets generateSummaries temporarily override model/thinking.
var summaryOverrides struct {
	model    string
	thinking string
}

func currentModelThinking() (model, thinking string) {
	cfg := config.Get()
	model = cfg.ChatModel
	thinking = cfg.ChatThinking

	// Apply summary overrides when set (only used during summary generation)
	if summaryOverrides.model != "" {
		model = summaryOverrides.model
	}
	if summaryOverrides.thinking != "" {
		thinking = summaryOverrides.thinking
	}
	return
}

func setSummaryModelThinking(model, thinking string) {
	summaryOverrides.model = model
	summaryOverrides.thinking = thinking
}
