#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Build
echo "==> Building grumbler..."
go build -o "$REPO_ROOT/bin/grumbler" "$REPO_ROOT/cmd/grumbler"

# Run review against main
echo "==> Reviewing branch diff against main..."
"$REPO_ROOT/bin/grumbler" review --base main

# Show log tail
LOG="$REPO_ROOT/logs/grumbler.log"
if [ -f "$LOG" ]; then
  echo ""
  echo "==> Last 40 log lines:"
  tail -40 "$LOG"
fi
