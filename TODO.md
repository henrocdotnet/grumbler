# TODO

## High Priority

- [ ] Integration test: run full pipeline on a small real diff with claude-cli
- [ ] Guardian stage: wire `guardian_system.tmpl`/`guardian_user.tmpl` into pipeline (currently prompts exist but no stage invokes them)
- [ ] Suggestion ID generation: assign stable IDs (hash of file+line+title) for safeguard matching
- [ ] Exit code: non-zero when critical/high severity issues found (for CI gating)

## Medium Priority

- [ ] Streaming output: show suggestions as they arrive per-file instead of waiting for all
- [ ] `--watch` mode: re-run review on file changes
- [ ] `--diff-file` flag: accept a pre-computed diff file instead of calling git
- [ ] Cost estimation: count tokens via tiktoken before sending, warn if large
- [ ] Config validation: `grumbler config check` to validate YAML and test LLM connectivity
- [ ] Config inspect: `grumbler config show` to display merged config (all layers resolved)
- [ ] Prompt template listing: `grumbler prompts list` showing available template names
- [ ] Global rules: load rules from `~/.config/grumbler/rules.yaml` merged under project rules

## Low Priority

- [ ] GitHub Actions integration: example workflow using SARIF output
- [ ] `grumbler rules generate` — LLM-assisted rule generation from codebase patterns
- [ ] Caching: skip re-reviewing files whose diff hasn't changed since last run
- [ ] `--model` flag for CLI providers (claude-cli currently uses whatever model is default)
- [ ] Progress bar / spinner during long reviews (lipgloss + bubbletea)
- [ ] Man page generation via cobra
- [ ] Homebrew formula
- [ ] Suggestion dedup across stages (review_files + crossfile may find same issue)

## Technical Debt

- [ ] `prompt/render.go` `injectDate` uses type switch — consider interface or struct embedding
- [ ] Safeguard verdict matching relies on suggestion ID which isn't reliably set
- [ ] `llm/claude_cli.go` `extractClaudeJSONResult` is fragile manual JSON parsing — use `encoding/json`
- [ ] `git/diff.go` `splitOnPrefix` is naive — won't handle edge cases in binary diffs
- [ ] Pipeline stages import `prompt` package directly — consider injecting rendered prompts via context
