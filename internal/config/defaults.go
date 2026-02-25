package config

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:  "claude-cli",
			Model:     "claude-sonnet-4-6",
			MaxTokens: 16384,
		},
		Review: ReviewConfig{
			BaseBranch:  "main",
			Concurrency: 5,
			Mode:        "diff",
			IgnorePaths: []string{
				"yarn.lock",
				"package-lock.json",
				"pnpm-lock.yaml",
				"go.sum",
				"**/*.min.js",
				"**/*.min.css",
				"vendor/**",
				"node_modules/**",
				".git/**",
			},
		},
		Passes: PassesConfig{
			ExpertPanel: true,
			Safeguard:   true,
			CrossFile:   true,
		},
		Filter: FilterConfig{
			MinSeverity:    "low",
			MaxSuggestions: 0,
		},
		Output: OutputConfig{
			Format: "terminal",
		},
	}
}

// DefaultConfigYAML is the scaffold written by the init command.
const DefaultConfigYAML = `# Review CLI configuration
# See: https://github.com/henrocdotnet/grumbler

llm:
  # Option A: Claude CLI (no API key needed — uses existing CLI auth)
  provider: claude-cli           # shells out to 'claude -p'
  model: claude-sonnet-4-6

  # Option B: Gemini CLI (no API key needed — uses existing CLI auth)
  # provider: gemini-cli
  # model: gemini-3.1-pro-preview

  # Option C: API via langchaingo (requires API key)
  # provider: anthropic          # anthropic|openai|google|...
  # model: claude-sonnet-4-6
  # apiKey: ${ANTHROPIC_API_KEY} # env var expansion supported
  # maxTokens: 16384

review:
  baseBranch: main
  concurrency: 5
  mode: diff                     # diff (single-call) or file (per-file)
  ignorePaths:
    - "yarn.lock"
    - "package-lock.json"
    - "pnpm-lock.yaml"
    - "go.sum"
    - "**/*.min.js"
    - "**/*.min.css"
    - "vendor/**"
    - "node_modules/**"

passes:
  expertPanel: true
  safeguard: true
  crossFile: true

filter:
  minSeverity: low
  maxSuggestions: 0              # 0 = unlimited

output:
  format: terminal               # terminal|json|sarif
`

// DefaultGlobalConfigYAML is the scaffold for ~/.config/grumbler/config.yaml.
// Contains only shared preferences — project-specific settings go in .grumbler/.
const DefaultGlobalConfigYAML = `# Global defaults (shared across all projects)
# Project-local .grumbler/config.yaml overrides these values.

llm:
  # Option A: Claude CLI (no API key needed — uses existing CLI auth)
  provider: claude-cli           # shells out to 'claude -p'
  model: claude-sonnet-4-6

  # Option B: Gemini CLI (no API key needed — uses existing CLI auth)
  # provider: gemini-cli
  # model: gemini-3.1-pro-preview

  # Option C: API via langchaingo (requires API key)
  # provider: anthropic          # anthropic|openai|google|...
  # model: claude-sonnet-4-6
  # apiKey: ${ANTHROPIC_API_KEY} # env var expansion supported
  # maxTokens: 16384
  # baseUrl:                     # custom endpoint (corp proxy, local LLM)

review:
  concurrency: 5
  mode: diff                     # diff (single-call) or file (per-file)

filter:
  minSeverity: low
  maxSuggestions: 0              # 0 = unlimited

output:
  format: terminal               # terminal|json|sarif
`

// DefaultRulesYAML is the scaffold written by the init command.
const DefaultRulesYAML = `# Review rules
# Each rule has: id, title, description, severity, path (glob), and tags.
# Rules are evaluated by the expert panel to classify violations.

rules: []

# Example:
# rules:
#   - id: no-console-log
#     title: "No console.log in production code"
#     description: "Remove console.log statements; use a proper logger instead."
#     severity: medium
#     path: "src/**/*.ts"
#     tags: [logging, cleanup]
`
