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
			Vet:         true,
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
  # model: gemini-3-flash-preview

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
  vet: true
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
  # model: gemini-3-flash-preview

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
# Each rule has: id, title, description, severity, path (glob), tags, and category.
# Rules are evaluated by the expert panel to classify violations.
# Remove or modify rules to suit your project. Use --no-rules to skip compliance entirely.

rules:
  - id: SEC-001
    title: "Hardcoded Secrets"
    description: "Detect hardcoded credentials, API keys, tokens, or passwords in source code. Secrets must be loaded from environment variables, vaults, or secret managers."
    severity: critical
    category: Security
    tags: [Security, Secrets Management]

  - id: SEC-002
    title: "SQL Injection"
    description: "Identify raw string concatenation or interpolation used to build SQL queries. Use parameterized queries or prepared statements instead."
    severity: critical
    category: Security
    tags: [Security, Input Validation]

  - id: SEC-003
    title: "Command Injection"
    description: "Flag shell command construction from unsanitized user input. Use parameterized execution APIs (e.g., exec.Command with separate args) instead of shell interpolation."
    severity: critical
    category: Security
    tags: [Security, Input Validation]

  - id: SEC-004
    title: "Insecure Deserialization"
    description: "Warn when untrusted input is deserialized without validation (e.g., pickle, yaml.Unsafe, eval). Restrict deserialization to safe loaders and validated schemas."
    severity: high
    category: Security
    tags: [Security, Input Validation]

  - id: ERR-001
    title: "Swallowed Errors"
    description: "Detect empty catch blocks, ignored error returns, or discarded error values. Errors must be handled, logged, or explicitly propagated."
    severity: high
    category: Error Handling
    tags: [Error Handling, Reliability]

  - id: ERR-002
    title: "Panic in Library Code"
    description: "Library and shared packages should return errors, not panic. Panics are reserved for truly unrecoverable states in main/top-level code only."
    severity: high
    category: Error Handling
    tags: [Error Handling, API Design]

  - id: ERR-003
    title: "Missing Error Context"
    description: "Errors propagated up the call stack should be wrapped with context (e.g., fmt.Errorf with %%w) so the origin is traceable without a debugger."
    severity: medium
    category: Error Handling
    tags: [Error Handling, Observability]

  - id: PERF-001
    title: "Unbounded Collection Growth"
    description: "Detect slices, maps, or channels that grow without bounds (e.g., in-memory caches with no eviction, append in infinite loops). Enforce size limits or TTL."
    severity: high
    category: Performance
    tags: [Performance, Reliability]

  - id: PERF-002
    title: "N+1 Query Pattern"
    description: "Identify database or API calls executed inside loops that could be batched into a single query. Use bulk fetch, JOINs, or preloading instead."
    severity: medium
    category: Performance
    tags: [Performance, Data Access]

  - id: CONC-001
    title: "Data Race Potential"
    description: "Flag shared mutable state accessed from multiple goroutines/threads without synchronization (mutexes, channels, atomics)."
    severity: high
    category: Concurrency
    tags: [Concurrency, Reliability]

  - id: LOG-001
    title: "Sensitive Data in Logs"
    description: "Detect logging of passwords, tokens, PII, or full request bodies. Log only safe identifiers; redact or omit sensitive fields."
    severity: high
    category: Observability
    tags: [Observability, Security]

  - id: LOG-002
    title: "Debug Artifacts in Production Code"
    description: "Flag TODO/FIXME/HACK comments, console.log, fmt.Println, print() statements, and other debug artifacts that should not ship."
    severity: low
    category: Observability
    tags: [Observability, Code Quality]

  - id: TEST-001
    title: "Missing Error Case Tests"
    description: "When new error paths are introduced, corresponding test cases for those error conditions should be present or added."
    severity: medium
    category: Testing
    tags: [Testing, Reliability]

  - id: API-001
    title: "Breaking API Contract"
    description: "Detect changes to public function signatures, REST endpoints, protobuf fields, or exported types that break backward compatibility without a version bump."
    severity: high
    category: API Design
    tags: [API Design, Compatibility]

  - id: RES-001
    title: "Resource Leak"
    description: "Detect opened files, connections, transactions, or locks that are not closed/released via defer, try-with-resources, context managers, or equivalent."
    severity: high
    category: Reliability
    tags: [Reliability, Resource Management]
`
