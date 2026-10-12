#!/usr/bin/env bash
set -euo pipefail

RACE="${RACE:-on}"
REPORT="$(mktemp)"
trap 'rm -f "$REPORT"' EXIT

step() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
ok() { printf '    \033[32m✔\033[0m %s\n' "$*"; }
fail() { printf '    \033[31m✘\033[0m %s\n' "$*"; }

step "Checking formatting (gofmt)"
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  fail "unformatted files:"
  printf '      %s\n' $unformatted
  exit 1
fi
ok "all files are formatted"

step "Running static analysis (go vet)"
go vet ./...
ok "no issues found"

flags=(-count=1 -cover -v)
if [ "$RACE" = "on" ]; then
  flags+=(-race)
fi
step "Running unit tests (race detector: $RACE)"
start=$(date +%s)
if ! go test "${flags[@]}" ./... >"$REPORT" 2>&1; then
  fail "unit tests failed, details:"
  grep -E -- '--- FAIL|^FAIL|panic:|_test\.go:[0-9]+:|DATA RACE' "$REPORT" | sed 's/^/      /' || cat "$REPORT"
  exit 1
fi
elapsed=$(( $(date +%s) - start ))

step "Coverage by package"
awk '
  $1 == "ok" {
    cov = "n/a"
    for (i = 1; i <= NF; i++) if ($i == "coverage:") cov = $(i + 1)
    printf "    %-52s %8s\n", $2, cov
  }
  $1 == "?" { printf "    %-52s %8s\n", $2, "no tests" }
' "$REPORT"

passed=$(grep -c -- '--- PASS' "$REPORT" || true)
skipped=$(grep -c -- '--- SKIP' "$REPORT" || true)
step "Summary"
ok "tests passed: $passed, skipped: $skipped, time: ${elapsed}s"
