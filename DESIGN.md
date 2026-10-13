# Design

## Motivation

Grumbler brings a full multi-pass LLM review pipeline to local development: mental simulation, 3-expert rule classification, 5-expert safeguard, and cross-file analysis — all from a single binary with no backend or webhooks.

## Architecture

### Provider Abstraction

Three provider categories behind `llm.Provider`:

```go
type Provider interface {
    Complete(ctx context.Context, msgs []Message, opts CompletionOpts) (string, error)
    Name() string
}
```

- **langchaingo** (`langchain.go`): API-key-based, wraps `tmc/langchaingo` for Anthropic/OpenAI/Google/any OpenAI-compatible endpoint.
- **claude-cli** (`claude_cli.go`): Shells out to `claude -p`, pipes prompt via stdin. Uses `--system-prompt` and `--output-format`.
- **gemini-cli** (`gemini_cli.go`): Shells out to `gemini -p`, pipes via stdin.
- **codex-cli** (`codex_cli.go`): Shells out to `codex exec`, pipes via stdin. Runs
  `--ephemeral` with a `read-only` sandbox and reads the agent's final message via
  `--output-last-message` rather than scraping stdout. Reasoning effort is passed
  explicitly (`-c model_reasoning_effort=…`) because `--ignore-user-config`
  discards the user's own `config.toml`.

CLI providers trade a per-call process spawn for zero API key configuration.
That spawn is a fixed cost of seconds regardless of prompt size, so call *count*
drives runtime more than token volume — hence batching in `Inspect` and a
per-call deadline (`llm.callTimeout`) on every CLI provider.

### Pipeline

Sequential stage executor. Each stage implements:

```go
type Stage interface {
    Name() string
    Execute(ctx context.Context, rc *ReviewContext) error
}
```

`ReviewContext` is mutable shared state passed through stages. Stages append suggestions, record errors, and read config; `AddSuggestions` and `AddError` are mutex-guarded because stages fan out. Parallelism uses `errgroup` with a configurable semaphore (default 10) — over batches in `Inspect` and over files in `Audit`. Configured concurrency always goes through `effectiveConcurrency`, since `SetLimit(0)` blocks forever.

`Inspect` collects batch results into indexed slots and drains them in order after `Wait`, so output does not depend on which batch finishes first — `Aggregate` dedups first-wins and can truncate to `filter.maxSuggestions`.

Pipeline halts on first error. Individual file failures within a stage are non-fatal (recorded on `rc.Errors`). The whole run is bounded by `review.timeout` and each provider call by `llm.callTimeout`.

### Diff Modes

| Mode | Git command | Default? |
|------|------------|----------|
| Staged | `git diff --cached` | Yes |
| All | `git diff HEAD` | `--all` flag |
| Branch | `git diff base...HEAD` | `--base` flag |

Staged-by-default avoids reviewing uncommitted work-in-progress. All modes use
`--unified=<review.diffContextLines>` (default 15): a wider hunk window is the
cheap way to give the model surrounding-code context, since full file content is
opt-in (`review.includeFileContent`).

### Prompt System

Templates embedded at compile time via `go:embed`. Rendered with `text/template`. Users override by placing `.tmpl` files in `.grumbler/prompts/` — override templates take precedence over embedded ones.

Prompt templates:
- `review_system.tmpl` — mental simulation review
- `expert_panel_system.tmpl` — 3-expert rule classification panel
- `safeguard_system.tmpl` — 5-expert validation panel
- `guardian_system.tmpl` — rule violation gate

### Config Resolution

Four-layer cascade, each only overwrites fields explicitly present:

```
compiled defaults → global config → project-local config → CLI flags
```

- **Global**: `~/.config/grumbler/config.yaml` (respects `XDG_CONFIG_HOME`). Shared preferences: LLM credentials, provider, model, concurrency, output format, severity filter.
- **Project-local**: `.grumbler/config.yaml`. Per-repo settings: base branch, ignore paths, pipeline passes, rules, prompt overrides.
- **Env vars**: `${VAR}` syntax expanded in both YAML files before parsing.
- **Flags**: `--provider`, `--model`, `--base`, `--format`, etc.

`yaml.Unmarshal` merges naturally — fields absent from a YAML file retain their prior value. No custom merge logic needed.

`config.Dir` (`.grumbler`) and `config.GlobalDir()` are the single sources of truth for path construction.

### Output Formats

- **terminal**: Lipgloss-styled severity-colored output with inline code blocks.
- **json**: Structured `ReviewResult` for programmatic consumption.
- **sarif**: SARIF 2.1.0 for CI/IDE integration (GitHub Code Scanning, VS Code).

## Key Decisions

1. **Staged by default** — Dev crud stays out of reviews. `--all` available when needed.
2. **No streaming** — Batch completion simplifies retry logic and JSON extraction.
3. **Non-fatal file errors** — One file failing doesn't abort the entire review.
4. **No daemon** — Stateless single-shot binary. No background processes.
5. **Module path is `henrocdotnet/grumbler`** — Standalone repository.
6. **No viper** — Config is simple enough for direct YAML + env expansion. Avoids the dependency.
