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
| `--provider` | LLM provider (`anthropic`, `openai`, `google`, `claude-cli`, `gemini-cli`) |
| `--model` | LLM model override |
| `-f`, `--format` | Output format (`terminal`, `json`, `sarif`) |
| `--config` | Path to config directory (default: `.`) |

### `review`

| Flag | Description |
|------|-------------|
| `--base <branch>` | Review commits on current branch vs base |
| `--all` | Include staged + unstaged changes |
| `--fast` | Diff-only mode (skip full file content) |
| `--concurrency <n>` | Max concurrent file reviews |
| `--min-severity <level>` | Filter output (`low`, `medium`, `high`, `critical`) |

## LLM Providers

| Provider | Flag | Auth |
|----------|------|------|
| Anthropic API | `--provider anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI API | `--provider openai` | `OPENAI_API_KEY` |
| Google API | `--provider google` | `GEMINI_API_KEY` |
| Claude CLI | `--provider claude-cli` | Existing `claude` auth |
| Gemini CLI | `--provider gemini-cli` | Existing `gemini` auth |

CLI providers shell out to `claude -p` or `gemini -p` — zero API key setup if you already have the CLI installed.

## Review Pipeline

```
Prepare → ReviewFiles → ClassifyRules → Safeguard → CrossFile → Aggregate
```

| Stage | What it does |
|-------|-------------|
| **Prepare** | Compute diff, filter ignored files, read file content, annotate patches |
| **ReviewFiles** | Mental simulation review per file (concurrent, bounded by `--concurrency`) |
| **ClassifyRules** | 3-expert panel (Alice/Bob/Charles) checks rule violations |
| **Safeguard** | 5-expert panel (Edward/Alice/Bob/Charles/Diana) validates suggestions |
| **CrossFile** | Cross-file contract/dependency analysis |
| **Aggregate** | Deduplicate, filter by severity, sort, cap |

Stages are conditionally skipped based on config (`passes.expertPanel`, `passes.safeguard`, `passes.crossFile`).

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
review:
  concurrency: 5
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
  expertPanel: true
  safeguard: true
  crossFile: true
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
