package config

var (
	codexModels = []string{
		"",
		"gpt-5.1-codex-max", // default per Codex docs
		"gpt-5.1-codex",
		"gpt-5.1",
		"gpt-4.1",
		"o4-mini", // CLI README default
	}
	codexThinking = []string{
		"",
		"minimal",
		"low",
		"medium",
		"high",
	}
)

// CodexModels returns available Codex model options ("" = default).
func CodexModels() []string {
	return codexModels
}

// CodexThinkingLevels returns available thinking levels ("" = default).
func CodexThinkingLevels() []string {
	return codexThinking
}
