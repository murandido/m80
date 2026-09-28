#!/usr/bin/env bash
#
# CI vulnerability gate. Runs govulncheck and fails on any vulnerability our code CALLS —
# exactly what golang/govulncheck-action did — except for a short, documented allowlist of
# temporary exceptions. It exists only because govulncheck has no native per-ID ignore, and the
# action it replaced is all-or-nothing.
#
# The allowlist is for FALSE POSITIVES ONLY (a Go vuln DB entry that is wrong or incomplete),
# never for a real vulnerability we haven't fixed. Every entry carries the reason and the exact
# condition under which it must be removed. When the list is empty this is just govulncheck.
#
# ⚠️  TEMPORARY. A session TODO tracks removing these — see the memory note
#     `project_govulncheck_temp_exception`.
set -euo pipefail

# --- Temporary exceptions (id → why it is a false positive + when to remove) ---
#
# (none - GO-2026-6452 left once the Go vuln DB recorded `fixed: 2.11.0`.)
ALLOWLIST=()

report="${GOVULNCHECK_JSON:-govulncheck.json}"

echo "Running govulncheck…"
# Non-zero exit just means vulnerabilities were found; we decide pass/fail from the findings,
# so don't let `set -e` abort here.
go run golang.org/x/vuln/cmd/govulncheck@latest -format json ./... > "$report" || true

# The OSV ids of vulnerabilities our code actually calls (a finding with a symbol-level trace).
# This is the same set govulncheck reports as "your code is affected by".
called="$(jq -r '.finding | select(.trace[0].function != null) | .osv' "$report" | sort -u)"

allow_re="$(printf '%s\n' "${ALLOWLIST[@]}" | paste -sd'|' -)"
if [ -n "$allow_re" ]; then
  unexpected="$(printf '%s\n' $called | grep -vE "^(${allow_re})\$" || true)"
else
  unexpected="$called"
fi

if [ -n "${unexpected//[[:space:]]/}" ]; then
  echo "::error::govulncheck found called vulnerabilities that are not allowlisted:"
  printf '  %s\n' $unexpected
  echo "--- full report ---"
  go run golang.org/x/vuln/cmd/govulncheck@latest ./... || true
  exit 1
fi

if [ -n "${called//[[:space:]]/}" ]; then
  echo "PASS — only temporary allowlisted exceptions are called:"
  printf '  %s\n' $called
  echo "(these are documented false positives; see the allowlist in this script)"
else
  echo "PASS — no called vulnerabilities."
fi
