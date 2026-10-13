# Grumbler

Standalone Go CLI for AI code review on local git diffs. Multi-pass LLM review pipeline in a single binary — no backend, no webhooks, no infrastructure.

## Install

```bash
go install github.com/henrocdotnet/grumbler/cmd/grumbler@latest
```

Or build from source:

```bash
make build    # outputs bin/grumbler
```

## Quick Start

```bash
# Initialize config (optional — sensible defaults built in)
grumbler init

# Review staged changes (default)
grumbler review

# Review all uncommitted changes
grumbler review --all

# Review branch commits against main
grumbler review --base main

# Use a specific model
grumbler review --model claude-sonnet-4-6

# Use Gemini CLI (no API key needed)
grumbler review --provider gemini-cli

# Use Codex CLI (no API key needed)
grumbler review --provider codex-cli

# Filter by severity
grumbler review --min-severity high

# JSON output
grumbler review -f json

# SARIF output (for CI integration)
grumbler review -f sarif
```

## Flags

### Global (all commands)

| Flag | Description |
|------|-------------|
| `--provider` | LLM provider (`anthropic`, `openai`, `google`, `claude-cli`, `gemini-cli`, `codex-cli`) |
| `--model` | LLM model override |
| `-f`, `--format` | Output format (`terminal`, `json`, `sarif`) |
| `--config` | Path to config directory (default: `.`) |

### `review`

| Flag | Description |
|------|-------------|
| `--base <branch>` | Review commits on current branch vs base |
| `--all` | Include staged + unstaged changes |
| `--fast` | Diff-only mode (force full file content off) |
| `--include-file-content` | Embed each changed file's full content alongside its diff (slower, deeper) |
| `--concurrency <n>` | Max concurrent inspect batches and audit calls |
| `--reasoning <effort>` | Reasoning effort for `codex-cli` (`low`…`ultra`; `minimal` can return empty responses) |
| `--max-batch-tokens <n>` | Token budget per inspect batch |
| `--max-file-tokens <n>` | Per-file cap on embedded content (`0` = unlimited) |
| `--timeout <dur>` | Overall review timeout (e.g. `30m`; `0` = none) |
| `--min-severity <level>` | Filter output (`low`, `medium`, `high`, `critical`) |
| `--no-rules` | Skip the compliance pass |
| `--publish` | Repo-relative links in markdown (for PR comments) |

## LLM Providers

| Provider | Flag | Auth |
|----------|------|------|
| Anthropic API | `--provider anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI API | `--provider openai` | `OPENAI_API_KEY` |
| Google API | `--provider google` | `GEMINI_API_KEY` |
| Claude CLI | `--provider claude-cli` | Existing `claude` auth |
| Gemini CLI | `--provider gemini-cli` | Existing `gemini` auth |
| Codex CLI | `--provider codex-cli` | Existing `codex` auth |

CLI providers shell out to `claude -p`, `gemini -p`, or `codex exec` — zero API key setup if you already have the matching CLI installed and authenticated.

## Review Pipeline

```
Prepare → Inspect → Compliance → CrossFile → Audit → Aggregate
```

| Stage | What it does | LLM calls |
|-------|-------------|-----------|
| **Prepare** | Compute diff, filter ignored files, read file content, annotate patches | 0 |
| **Inspect** | Primary defect detection over token-bounded batches (concurrent, bounded by `--concurrency`) | 1 per batch |
| **Compliance** | Rule-violation panel over the combined diff | 1 |
| **CrossFile** | Cross-file contract/dependency analysis | 1 |
| **Audit** | Validation panel that keeps, revises, or rejects each suggestion (concurrent) | 1 per file with findings |
| **Aggregate** | Deduplicate, filter by severity, sort, cap | 0 |

CrossFile runs before Audit so its findings are validated like any other.

Stages are conditionally skipped based on config (`passes.reviewTeam`,
`passes.audit`, `passes.crossFile`).

### Performance

`Inspect` dominates a run. Three settings control its cost:

- **`review.concurrency`** — batches run in parallel up to this limit.
- **`review.includeFileContent`** (default `true`) — embedding each changed
  file's full content alongside its diff costs 2.1× the prompt tokens but is
  *faster* end to end and detects substantially more, because per-call reasoning
  time drops when the model can read instead of infer. Turn it on unless you are
  Set it false only if you are token-cost constrained; `review.diffContextLines`
  (default 15) is then the cheaper partial substitute.
- **`review.maxBatchTokens`** (default 10000) — the most important quality
  setting. Detection collapses as the reviewed diff grows: on a 143-file branch,
  6 batches of ~50k tokens found 9 issues while 40 batches of ~10k found 30,
  including two critical command injections the large-batch run missed. Raise it
  only to cut cost, and expect to lose findings.

`passes.maxContextTokens` bounds the whole-diff prompts built by Compliance and
CrossFile; without it they overflow the model's context window on a large
branch and return nothing.

## Configuration

Config cascades through four layers — each only overwrites fields it explicitly sets:

```
compiled defaults → ~/.config/grumbler/config.yaml → .grumbler/config.yaml → CLI flags
```

### Global Config (shared across projects)

```bash
grumbler init --global    # scaffolds ~/.config/grumbler/config.yaml
```

Put LLM credentials and personal preferences here:

```yaml
llm:
  provider: claude-cli
  model: claude-sonnet-4-6
  sandbox: read-only       # codex-cli sandbox policy
  callTimeout: 5m          # bounds a single provider call
  # reasoningEffort: low   # codex-cli: minimal|low|medium|high|xhigh|max|ultra
review:
  concurrency: 10
  timeout: 30m
  includeFileContent: true
  maxBatchTokens: 10000
  diffContextLines: 15
filter:
  minSeverity: low
output:
  format: terminal
```

Respects `XDG_CONFIG_HOME` — defaults to `~/.config/grumbler/`.

### Project Config (per-repo)

```bash
grumbler init             # scaffolds .grumbler/config.yaml
```

Put project-specific settings here:

```yaml
llm:
  model: gpt-4o               # override just this field
review:
  baseBranch: develop
  ignorePaths: ["yarn.lock", "**/*.min.js", "vendor/**"]
passes:
  reviewTeam: true
  audit: true
  crossFile: true
  maxContextTokens: 80000     # caps the whole-diff Compliance/CrossFile prompts
  onOverflow: truncate        # truncate|skip when the diff exceeds that budget
```

Fields not set in project config inherit from global, which inherits from defaults.

## Custom Rules

Define rules in `.grumbler/rules.yaml`:

```yaml
rules:
  - id: no-console-log
    title: "No console.log in production code"
    description: "Remove console.log statements; use a proper logger instead."
    severity: medium
    path: "src/**/*.ts"
    tags: [logging, cleanup]
```

```bash
grumbler rules list              # show loaded rules
grumbler rules import rules.yaml # import from file
```

## Prompt Overrides

Place `.tmpl` files in `.grumbler/prompts/` to override built-in prompt templates. Available templates: `review_system`, `review_user`, `expert_panel_system`, `expert_panel_user`, `safeguard_system`, `safeguard_user`, `guardian_system`, `guardian_user`.

## Development

```bash
make build                              # build binary → bin/grumbler
make install                            # go install
go test ./...                           # all tests
go test -v -run TestLive ./internal/llm/ # live LLM tests
go vet ./...                            # static analysis
```
