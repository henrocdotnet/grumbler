package config

// Defaults returns a Config with sensible default values.
func Defaults() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:    "claude-cli",
			Model:       "claude-sonnet-4-6",
			MaxTokens:   16384,
			Sandbox:     "read-only",
			CallTimeout: "5m",
		},
		Review: ReviewConfig{
			BaseBranch:           "main",
			Concurrency:          10,
			Mode:                 "diff",
			IncludeFileContent:   true,
			MaxFileContentTokens: 4000,
			MaxBatchTokens:       10000,
			DiffContextLines:     15,
			Timeout:              "30m",
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
			ReviewTeam:       true,
			Audit:            true,
			CrossFile:        true,
			MaxContextTokens: 80000,
			OnOverflow:       "truncate",
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

  # Option C: Codex CLI (no API key needed — uses existing CLI auth)
  # provider: codex-cli
  # model: gpt-6.1-sol           # see 'codex debug models' for the live list
  #   gpt-6.1-sol    latest workhorse (codex default)
  #   gpt-6-astra    frontier; most capable, slowest
  #   gpt-6-sol      previous-gen workhorse
  #   gpt-6-luna     fast, cheap; no 'ultra' effort
  #   gpt-5.6-sol    older workhorse
  #   gpt-5.6-terra  older balanced
  #   gpt-5.6-luna   older fast; no 'ultra' effort

  # Option D: API via langchaingo (requires API key)
  # provider: anthropic          # anthropic|openai|google|...
  # model: claude-sonnet-4-6
  # apiKey: ${ANTHROPIC_API_KEY} # env var expansion supported
  # maxTokens: 16384

  # Speed / behaviour knobs (codex-cli)
  # reasoningEffort: low         # low|medium|high|xhigh|max|ultra ('ultra' unsupported on luna)
  #                              # NOTE: 'minimal' returns an empty final message
  #                              # on some models — measure before adopting it.
  sandbox: read-only             # read-only|workspace-write|danger-full-access
  callTimeout: 5m                # bounds a single provider call; 0 = none

review:
  baseBranch: main
  concurrency: 10                # parallel inspect batches and audit calls
  mode: diff                     # diff (single-call) or file (per-file)
  timeout: 30m                   # bounds the whole run; 0 = none

  # Embed each changed file's full content alongside its diff. Measured on a
  # 143-file branch: 2.1x the prompt tokens, but FASTER end to end and markedly
  # better detection, because per-call reasoning time drops when the model can
  # read instead of infer. Set false only if you are token-cost constrained.
  includeFileContent: true
  maxFileContentTokens: 4000     # per-file cap when includeFileContent is on; 0 = unlimited
  maxBatchTokens: 10000          # token budget for one inspect batch
                                 # smaller batches detect more; raise only to cut cost
  diffContextLines: 15           # git diff --unified=N

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
  reviewTeam: true
  audit: true
  crossFile: true
  # The reviewTeam and crossFile passes send the whole diff in one call. Without
  # a cap they overflow the model's context window on a large branch and return
  # nothing.
  maxContextTokens: 80000
  onOverflow: truncate           # truncate|skip

filter:
  minSeverity: low
  maxSuggestions: 0              # 0 = unlimited

output:
  format: terminal               # terminal|json|yaml|sarif
  publish: false                 # true = repo-relative links (PR comments)
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

  # Option C: Codex CLI (no API key needed — uses existing CLI auth)
  # provider: codex-cli
  # model: gpt-6.1-sol           # see 'codex debug models' for the live list
  #   gpt-6.1-sol    latest workhorse (codex default)
  #   gpt-6-astra    frontier; most capable, slowest
  #   gpt-6-sol      previous-gen workhorse
  #   gpt-6-luna     fast, cheap; no 'ultra' effort
  #   gpt-5.6-sol    older workhorse
  #   gpt-5.6-terra  older balanced
  #   gpt-5.6-luna   older fast; no 'ultra' effort

  # Option D: API via langchaingo (requires API key)
  # provider: anthropic          # anthropic|openai|google|...
  # model: claude-sonnet-4-6
  # apiKey: ${ANTHROPIC_API_KEY} # env var expansion supported
  # maxTokens: 16384
  # baseUrl:                     # custom endpoint (corp proxy, local LLM)

  # Speed / behaviour knobs (codex-cli)
  # reasoningEffort: low         # low|medium|high|xhigh|max|ultra ('ultra' unsupported on luna)
  #                              # NOTE: 'minimal' returns an empty final message
  #                              # on some models — measure before adopting it.
  sandbox: read-only             # read-only|workspace-write|danger-full-access
  callTimeout: 5m                # bounds a single provider call; 0 = none

review:
  concurrency: 10                # parallel inspect batches and audit calls
  mode: diff                     # diff (single-call) or file (per-file)
  timeout: 30m                   # bounds the whole run; 0 = none
  includeFileContent: true       # embed full file content alongside diffs (more tokens, better + faster)
  maxFileContentTokens: 4000     # per-file cap when includeFileContent is on; 0 = unlimited
  maxBatchTokens: 10000          # token budget for one inspect batch
                                 # smaller batches detect more; raise only to cut cost
  diffContextLines: 15           # git diff --unified=N

filter:
  minSeverity: low
  maxSuggestions: 0              # 0 = unlimited

output:
  format: terminal               # terminal|json|yaml|sarif
  publish: false                 # true = repo-relative links (PR comments)
`

// DefaultRulesYAML is the scaffold written by the init command.
const DefaultRulesYAML = `# Review rules
# Each rule has: id, name, guidance, severity, glob (file pattern), tags, and category.
# Rules are evaluated by the review team to classify violations.
# Remove or modify rules to suit your project. Use --no-rules to skip compliance entirely.

rules:
  - id: SEC-001
    name: "Hardcoded Secrets"
    guidance: "Detect hardcoded credentials, API keys, tokens, or passwords in source code. Secrets must be loaded from environment variables, vaults, or secret managers."
    severity: critical
    category: Security
    tags: [Security, Secrets Management]

  - id: SEC-002
    name: "SQL Injection"
    guidance: "Identify raw string concatenation or interpolation used to build SQL queries. Use parameterized queries or prepared statements instead."
    severity: critical
    category: Security
    tags: [Security, Input Validation]

  - id: SEC-003
    name: "Command Injection"
    guidance: "Flag shell command construction from unsanitized user input. Use parameterized execution APIs (e.g., exec.Command with separate args) instead of shell interpolation."
    severity: critical
    category: Security
    tags: [Security, Input Validation]

  - id: SEC-004
    name: "Insecure Deserialization"
    guidance: "Warn when untrusted input is deserialized without validation (e.g., pickle, yaml.Unsafe, eval). Restrict deserialization to safe loaders and validated schemas."
    severity: high
    category: Security
    tags: [Security, Input Validation]

  - id: ERR-001
    name: "Swallowed Errors"
    guidance: "Detect empty catch blocks, ignored error returns, or discarded error values. Errors must be handled, logged, or explicitly propagated."
    severity: high
    category: Error Handling
    tags: [Error Handling, Reliability]

  - id: ERR-002
    name: "Panic in Library Code"
    guidance: "Library and shared packages should return errors, not panic. Panics are reserved for truly unrecoverable states in main/top-level code only."
    severity: high
    category: Error Handling
    tags: [Error Handling, API Design]

  - id: ERR-003
    name: "Missing Error Context"
    guidance: "Errors propagated up the call stack should be wrapped with context (e.g., fmt.Errorf with %%w) so the origin is traceable without a debugger."
    severity: medium
    category: Error Handling
    tags: [Error Handling, Observability]

  - id: PERF-001
    name: "Unbounded Collection Growth"
    guidance: "Detect slices, maps, or channels that grow without bounds (e.g., in-memory caches with no eviction, append in infinite loops). Enforce size limits or TTL."
    severity: high
    category: Performance
    tags: [Performance, Reliability]

  - id: PERF-002
    name: "N+1 Query Pattern"
    guidance: "Identify database or API calls executed inside loops that could be batched into a single query. Use bulk fetch, JOINs, or preloading instead."
    severity: medium
    category: Performance
    tags: [Performance, Data Access]

  - id: CONC-001
    name: "Data Race Potential"
    guidance: "Flag shared mutable state accessed from multiple goroutines/threads without synchronization (mutexes, channels, atomics)."
    severity: high
    category: Concurrency
    tags: [Concurrency, Reliability]

  - id: LOG-001
    name: "Sensitive Data in Logs"
    guidance: "Detect logging of passwords, tokens, PII, or full request bodies. Log only safe identifiers; redact or omit sensitive fields."
    severity: high
    category: Observability
    tags: [Observability, Security]

  - id: LOG-002
    name: "Debug Artifacts in Production Code"
    guidance: "Flag TODO/FIXME/HACK comments, console.log, fmt.Println, print() statements, and other debug artifacts that should not ship."
    severity: low
    category: Observability
    tags: [Observability, Code Quality]

  - id: TEST-001
    name: "Missing Error Case Tests"
    guidance: "When new error paths are introduced, corresponding test cases for those error conditions should be present or added."
    severity: medium
    category: Testing
    tags: [Testing, Reliability]

  - id: API-001
    name: "Breaking API Contract"
    guidance: "Detect changes to public function signatures, REST endpoints, protobuf fields, or exported types that break backward compatibility without a version bump."
    severity: high
    category: API Design
    tags: [API Design, Compatibility]

  - id: RES-001
    name: "Resource Leak"
    guidance: "Detect opened files, connections, transactions, or locks that are not closed/released via defer, try-with-resources, context managers, or equivalent."
    severity: high
    category: Reliability
    tags: [Reliability, Resource Management]
`
