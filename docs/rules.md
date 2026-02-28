# Rules

Rules are declarative review directives that tell grumbler what to look for beyond its built-in defect detection. Each rule is matched against files via glob patterns and injected into the LLM system prompt so the model evaluates code against project-specific standards.

## Rule Schema

```yaml
rules:
  - id: SEC-001                          # unique identifier (required)
    title: Hardcoded Secrets             # short name (required)
    description: >-                      # what to detect and why (required)
      Detect hardcoded credentials, API keys, tokens, or passwords.
      Secrets must be loaded from environment variables or secret managers.
    severity: critical                   # critical | high | medium | low (required)
    category: Security                   # logical grouping (optional)
    tags: [Security, Secrets Management] # arbitrary labels (optional)
    path: "src/**/*.ts"                  # glob filter (optional; empty = all files)
    examples:                            # illustrative snippets (optional)
      - 'const API_KEY = "sk-abc123"'
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier (convention: `PREFIX-NNN`) |
| `title` | string | yes | Human-readable name |
| `description` | string | yes | What the rule detects, why it matters, and what to do instead |
| `severity` | string | yes | `critical`, `high`, `medium`, or `low` |
| `path` | string | no | Doublestar glob pattern restricting which files the rule applies to. Empty = global |
| `category` | string | no | Grouping label (e.g., Security, Performance) |
| `tags` | []string | no | Arbitrary tags for filtering/reporting |
| `examples` | []string | no | Code snippets illustrating violations |

## File Location

Grumbler loads rules from `.grumbler/rules.yaml` in the project root. If absent or empty, the built-in default rules are used.

```
project/
  .grumbler/
    rules.yaml    <-- your rules
    config.yaml   <-- general config
```

## File Matching

Rules are matched to files using the `path` field with [doublestar](https://github.com/bmatcuk/doublestar) glob syntax:

| Pattern | Matches |
|---------|---------|
| `src/**/*.ts` | All `.ts` files under `src/` at any depth |
| `**/*.go` | All Go files anywhere |
| `internal/api/**` | Everything under `internal/api/` |
| *(empty)* | All files (global rule) |

When a file is reviewed, only rules whose `path` matches that file (or rules with no `path`) are included in the prompt.

## Default Rules

When no `.grumbler/rules.yaml` exists, grumbler applies 15 built-in rules:

| ID | Title | Severity | Category |
|----|-------|----------|----------|
| SEC-001 | Hardcoded Secrets | critical | Security |
| SEC-002 | SQL Injection | critical | Security |
| SEC-003 | Command Injection | critical | Security |
| SEC-004 | Insecure Deserialization | high | Security |
| ERR-001 | Swallowed Errors | high | Error Handling |
| ERR-002 | Panic in Library Code | high | Error Handling |
| ERR-003 | Missing Error Context | medium | Error Handling |
| PERF-001 | Unbounded Collection Growth | high | Performance |
| PERF-002 | N+1 Query Pattern | medium | Performance |
| CONC-001 | Data Race Potential | high | Concurrency |
| LOG-001 | Sensitive Data in Logs | high | Observability |
| LOG-002 | Debug Artifacts in Production Code | low | Observability |
| TEST-001 | Missing Error Case Tests | medium | Testing |
| API-001 | Breaking API Contract | high | API Design |
| RES-001 | Resource Leak | high | Reliability |

All default rules are global (no `path` restriction). Providing a `.grumbler/rules.yaml` **replaces** the defaults entirely — copy the ones you want to keep.

## Pipeline Integration

Rules participate in two pipeline stages:

### 1. Inspect (Defect Detection)

Matched rules are formatted as structured text and appended to the LLM system prompt:

```
## Applicable Rules

The following project-specific rules MUST also be checked:
- Rule ID: SEC-001
  Title: Hardcoded Secrets
  Description: Detect hardcoded credentials...
  Severity: critical
```

In **diff mode** (default), rules are aggregated across all files in the batch — each file's matched rules are merged and deduplicated by ID. In **per-file mode**, each file gets only its own matched rules.

### 2. Compliance (Expert Panel)

When the `expertPanel` pass is enabled, all rules are serialized as JSON and sent to a separate LLM panel that evaluates the diff specifically for rule violations. The panel returns violated rule IDs, which are tagged onto the corresponding suggestions.

## Skipping Rules

Pass `--no-rules` to `grumbler review` to zero out all rules and skip the compliance pass entirely:

```bash
grumbler review --no-rules
```

This is useful for quick defect-only scans without expert-panel overhead.

## Lifecycle Summary

```
LoadRules(.grumbler/rules.yaml)
  │  ↓ fallback
  │  DefaultRules()
  ▼
--no-rules? → []          # empty → compliance skips
  │
ReviewContext.Rules = [...]
  │
  ├─ Prepare: filter files via ignorePaths
  │
  ├─ Inspect: MatchRules(rules, filePath) → formatForPrompt → system prompt
  │
  ├─ Compliance: rules JSON → expert panel → tag violated rule IDs
  │
  └─ Output: suggestions with ruleIds populated
```

## Writing Custom Rules

### Example: TypeScript-Specific Rules

```yaml
rules:
  # Keep defaults you care about
  - id: SEC-002
    title: SQL Injection
    description: >-
      Identify raw string concatenation or interpolation used to build
      SQL queries. Use parameterized queries or prepared statements.
    severity: critical
    category: Security

  # Add project-specific rules
  - id: PROJ-001
    title: No Try/Catch in Express Endpoints
    description: >-
      Express endpoint handlers must not use try/catch blocks. Errors
      must bubble up to the global error handler middleware.
    severity: high
    category: Project Convention
    path: "src/resources/**/*.ts"

  - id: PROJ-002
    title: Use asyncHandler Wrapper
    description: >-
      All async Express route handlers must be wrapped with asyncHandler
      to ensure rejected promises reach the error handler.
    severity: high
    category: Project Convention
    path: "src/resources/**/*.ts"
```

### Required ID Prefixes

Rule IDs **must** start with one of the following prefixes. Grumbler validates this at load time and rejects rules with unrecognized prefixes.

| Prefix | Domain |
|--------|--------|
| `SEC-` | Security |
| `ERR-` | Error handling |
| `PERF-` | Performance |
| `CONC-` | Concurrency |
| `LOG-` | Logging / observability |
| `TEST-` | Testing |
| `API-` | API design / compatibility |
| `RES-` | Resource management |
| `PROJ-` | Project-specific conventions |

IDs must be unique. The suffix after the prefix (e.g., `001`) is freeform.

### Tips

- **Be specific in descriptions.** The LLM uses the description verbatim. Vague rules produce vague findings.
- **Use `path` to reduce noise.** A Go-specific rule applied to `.ts` files wastes tokens and produces false positives.
- **Severity drives triage.** `critical` and `high` findings appear prominently in reports; `low` findings are collapsed.
- **Fewer targeted rules > many broad rules.** Each rule consumes prompt tokens. Prioritize rules that catch real defects in your codebase.
