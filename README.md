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
grumbler review --model claude-sonnet-5-5

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
| `--reasoning <effort>` | Effort for `claude-cli` or `codex-cli`: `low`, `medium`, `high`, `xhigh`, `max`; Codex also accepts `minimal` and `ultra` |
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

### Claude Code CLI models

The default is pinned to `claude-sonnet-5-5`. Current options, verified October 30,
2026, are:

| Model ID | CLI alias | Use |
|----------|-----------|-----|
| `claude-fable-5-1` | `fable` | Demanding reasoning and long-running work |
| `claude-opus-5-5` | `opus` | Complex coding and review |
| `claude-sonnet-5-5` | `sonnet` | Balance of speed and intelligence |
| `claude-haiku-5-5` | `haiku` | Fast, lightweight work |

Full IDs pin a release. Aliases follow Claude Code's provider-specific model
mapping, which can differ between direct Anthropic access, gateways, and cloud
providers. `best` selects Fable where available and otherwise Opus; `default`
uses the account default. Actual availability depends on your account and provider.
See the [Claude Code model configuration](https://code.claude.com/docs/en/model-config)
and [current model IDs](https://platform.claude.com/docs/en/models/overview).

```bash
grumbler review --provider claude-cli --model opus --reasoning high
grumbler review --provider claude-cli --model claude-sonnet-5-5
```

Grumbler forwards model names unchanged, including custom model IDs and older
pins. Existing project/global configuration overrides the built-in default;
update its `llm.model` value or pass `--model` to select a newer model.

## Review Pipeline

```
Prepare → Inspect → Compliance → CrossFile → Audit → Aggregate
```

| Stage | What it does | LLM calls |
|-------|-------------|-----------|
| **Prepare** | Compute diff, filter ignored files, read file content, annotate patches | 0 |
| **Inspect** | Primary defect detection over token-bounded batches (concurrent, bounded by `--concurrency`) | 1 per batch |
| **Compliance** | Discover located rule violations; merge matching findings and preserve new ones | 1 |
| **CrossFile** | Cross-file contract/dependency analysis | 1 |
| **Audit** | Verify findings with supporting changed files; keep, revise, reject, or mark unresolved (concurrent) | 1 per file with findings |
| **Aggregate** | Deduplicate, filter by severity, sort, cap | 0 |

CrossFile runs before Audit so its findings are validated like any other.

Audit checks a finding separately from its proposed fix. A valid finding survives
an unsafe proposal; Audit can revise or remove the proposal. Missing evidence,
incomplete model responses, and audit failures leave findings marked `unresolved`
rather than silently accepting or discarding them. Audit decisions include an
`auditRationale` in JSON, YAML, Markdown, terminal output, and SARIF properties.
Rejected findings and their rationales remain in the audit stage snapshot.

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

`passes.maxContextTokens` bounds the whole-diff context built by Compliance and
CrossFile, and the complete rendered Audit prompts. Audit prioritizes cited
supporting files, caps embedded source with `review.maxFileContentTokens`, and
discloses omissions. It marks findings unresolved if the required primary
context cannot fit. Supporting context increases Audit's token usage.

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
  model: claude-sonnet-5-5
  sandbox: read-only       # codex-cli sandbox policy
  callTimeout: 5m          # bounds a single provider call
  # reasoningEffort: high  # claude-cli and codex-cli; supported levels depend on the model
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

`llm.reasoningEffort` and `--reasoning` pass `--effort` to Claude CLI and
`model_reasoning_effort` to Codex CLI. For example:

```bash
grumbler review --provider claude-cli --reasoning high
```

When effort is unset, Grumbler leaves it to the provider. Claude Code's
`CLAUDE_CODE_EFFORT_LEVEL` environment variable takes precedence over its
`--effort` flag. Supported levels also depend on the selected model.

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
  maxContextTokens: 80000     # Compliance/CrossFile context and complete Audit prompts
  onOverflow: truncate        # truncate|skip when the diff exceeds that budget
```

Fields not set in project config inherit from global, which inherits from defaults.

## Custom Rules

Define rules in `.grumbler/rules.yaml`:

```yaml
rules:
  - id: PROJ-001
    name: "Explicit live-write policy"
    guidance: "Check write predicates against supported backend modes. Name existing caller guards; classify guarded invariant inconsistencies as low-severity maintainability notes."
    severity: low
    glob: "testing/**/*.ts"
    category: Maintainability
    tags: [Configuration, Reliability]
```

```bash
grumbler rules list              # show loaded rules
grumbler rules import rules.yaml # import from file
```

Without a nonempty rules file, Grumbler uses its built-in rules. Custom rules
replace that set; existing files are not automatically upgraded. New project
scaffolds contain the same rules as the built-in set.

The defaults include destructive-operation boundaries (`SEC-005`), dependency
contract coupling (`API-002`), and configuration capability invariants
(`ERR-004`). Concrete observations about these contracts use category
`maintainability` at low severity. They must acknowledge existing tests and
guards, and must not describe hypothetical or unreachable failures as current
bugs. Generic style advice remains out of scope.

Compliance returns rule IDs with file paths, changed line ranges, and evidence.
A violation becomes a finding even if Inspect missed it. A matching finding
receives that rule ID; unrelated findings do not inherit it. Legacy custom
compliance prompt overrides must return these located fields rather than only
rule IDs and reasons; unlocated responses produce warnings.

## Prompt Overrides

Place `.tmpl` files in `.grumbler/prompts/` to override built-in prompt templates. Available templates: `review_system`, `review_user`, `review_user_diff`, `review_team_system`, `review_team_user`, `safeguard_system`, `safeguard_user`, `guardian_system`, `guardian_user`.

## Development

```bash
make build                              # build binary → bin/grumbler
make install                            # go install
go test ./...                           # all tests
go test -v -run TestLive ./internal/llm/ # live LLM tests
go vet ./...                            # static analysis
```
