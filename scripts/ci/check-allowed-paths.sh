#!/usr/bin/env bash
# check-allowed-paths.sh — enforce per-task Allowed Paths whitelist on PR diff.
#
# Phase 1 strategy (TODO post-Phase-1: migrate task defs to docs/tasks/*.md):
#   1. Parse PR description for `Refs: P1-T-XXX` (single task per PR; per agent-coordination §5.2)
#   2. Grep docs/phase1-plan.md for the matching `### P1-T-XXX <title>` section
#   3. Extract paths from the `**Allowed Paths**:` block (between that line and the next
#      `**<header>**:` line)
#   4. Run `git diff --name-only origin/dev...HEAD` (excluding origin/dev to scope to this PR)
#   5. For each changed file, check it matches at least one allowed path glob; fail otherwise.
#
# Permissive behavior (per task spec): on parse failure (no Refs:, task section not found,
# Allowed Paths block missing), emit `::warning::` and exit 0. We don't block non-task PRs
# like RFC docs, dependabot bumps, etc.
#
# Inputs (env):
#   GITHUB_EVENT_NAME    — "pull_request" or "push"; we no-op on push
#   GITHUB_REPOSITORY    — "owner/repo" (used by `gh`)
#   PR_NUMBER            — PR number (CI passes ${{ github.event.pull_request.number }})
#   PR_BODY              — fallback: raw PR description (when gh not available)
#   GITHUB_BASE_REF      — base branch name (defaults to "dev")
#
# Usage:
#   ./scripts/ci/check-allowed-paths.sh
#
# Exit codes:
#   0  — all changed paths within allowed globs (or permissive skip)
#   1  — at least one changed path outside allowed globs

set -euo pipefail

PLAN="docs/phase1-plan.md"
BASE_REF="${GITHUB_BASE_REF:-dev}"

# ---------- helpers ----------

warn() { printf '::warning::%s\n' "$*" >&2; }
err()  { printf '::error::%s\n'  "$*" >&2; }
info() { printf '%s\n' "$*"; }

# Skip on non-PR events (push to dev, manual dispatch).
if [[ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]]; then
  info "Not a pull_request event (got ${GITHUB_EVENT_NAME:-unset}); skipping Allowed Paths check."
  exit 0
fi

# ---------- 1. fetch PR description ----------

pr_body=""
if [[ -n "${PR_BODY:-}" ]]; then
  pr_body="$PR_BODY"
elif command -v gh >/dev/null 2>&1 && [[ -n "${PR_NUMBER:-}" ]]; then
  pr_body="$(gh pr view "$PR_NUMBER" --json body --jq .body 2>/dev/null || true)"
fi

if [[ -z "$pr_body" ]]; then
  warn "Could not fetch PR description (no PR_BODY, no gh, or PR not found). Skipping."
  exit 0
fi

# ---------- 2. extract Refs: P1-T-XXX ----------

# Accept "Refs: P1-T-XXX" or "Refs:P1-T-XXX" (with optional whitespace); match first occurrence.
task_id="$(printf '%s\n' "$pr_body" | grep -oE 'Refs:[[:space:]]*P1-T-[0-9]+' | head -n1 | sed -E 's/^Refs:[[:space:]]*//' || true)"

if [[ -z "$task_id" ]]; then
  warn "No \`Refs: P1-T-XXX\` found in PR description. Skipping Allowed Paths check (likely RFC / non-task PR)."
  exit 0
fi

info "Task ID extracted: $task_id"

# ---------- 3. extract Allowed Paths block from phase1-plan.md ----------

if [[ ! -f "$PLAN" ]]; then
  warn "$PLAN not found; skipping Allowed Paths check."
  exit 0
fi

# Find the `### P1-T-XXX ...` heading line number.
task_line="$(grep -nE "^### ${task_id}([[:space:]]|$)" "$PLAN" | head -n1 | cut -d: -f1 || true)"
if [[ -z "$task_line" ]]; then
  warn "Task section \`### ${task_id}\` not found in $PLAN. Skipping."
  exit 0
fi

# Find the `**Allowed Paths**:` line after that heading. Both half-width `:` and
# full-width Chinese `：` (U+FF1A, bytes EF BC 9A) are supported — phase1-plan.md uses
# the latter throughout. We use `LC_ALL=C grep -E` so the BRE engine treats input
# as raw bytes (avoiding locale-dependent PCRE; ubuntu-22.04 grep lacks PCRE multibyte).
fw_colon="$(printf '\xef\xbc\x9a')"
ap_line="$(LC_ALL=C tail -n "+$task_line" "$PLAN" | LC_ALL=C grep -nE "^\*\*Allowed Paths\*\*(:|${fw_colon})" | head -n1 | cut -d: -f1 || true)"
if [[ -z "$ap_line" ]]; then
  warn "\`**Allowed Paths**:\` block not found under ${task_id}. Skipping."
  exit 0
fi
# Convert relative offset back to absolute line number in $PLAN.
ap_line=$((task_line + ap_line - 1))

# Extract paths from the Allowed Paths line forward, stopping at the next `**Header**:` line
# or a horizontal rule `---`. Paths are inside backticks: `path/to/glob`.
# Same caveat: both half-width `:` and full-width `：` need to terminate the block.
# We use `LC_ALL=C awk` so regex operates on raw bytes; the full-width colon's first byte
# is 0xEF (decimal 239), which doesn't appear in any ASCII-safe regex metachar, so we can
# match its byte pattern via gawk's printf-built dynamic regex.
block="$(LC_ALL=C awk -v start="$ap_line" -v fw_colon="$fw_colon" '
  BEGIN {
    hdr_re = "^\\*\\*[^*]+\\*\\*(:|" fw_colon ")"
  }
  NR==start { in_block=1; print; next }
  NR>start && $0 ~ hdr_re { exit }
  NR>start && /^---[[:space:]]*$/ { exit }
  in_block { print }
' "$PLAN")"

# Extract every backtick-delimited token from the block; each is one glob.
# Filter out empty / structural tokens.
mapfile -t globs < <(printf '%s\n' "$block" | grep -oE '`[^`]+`' | sed 's/^`//; s/`$//' | grep -v '^[[:space:]]*$' || true)

if [[ ${#globs[@]} -eq 0 ]]; then
  warn "Allowed Paths block found for ${task_id} but no backtick-quoted globs parsed. Skipping."
  exit 0
fi

info "Allowed Paths for ${task_id}:"
for g in "${globs[@]}"; do
  info "  - $g"
done

# ---------- 4. compute diff ----------

# Ensure we have the base ref locally (fetch-depth: 0 in workflow already does this, but
# guard for shallow / forks).
if ! git rev-parse --verify --quiet "origin/${BASE_REF}" >/dev/null; then
  warn "origin/${BASE_REF} not present locally; fetching."
  git fetch --no-tags --depth=50 origin "${BASE_REF}" >/dev/null 2>&1 || true
fi

# `git diff --name-only origin/dev...HEAD` lists files changed on the PR branch
# since it diverged from the base. (Three-dot syntax = symmetric difference from merge base.)
if ! mapfile -t changed < <(git diff --name-only "origin/${BASE_REF}...HEAD" 2>/dev/null); then
  warn "git diff against origin/${BASE_REF} failed; skipping."
  exit 0
fi

if [[ ${#changed[@]} -eq 0 ]]; then
  info "No changed files vs origin/${BASE_REF}; nothing to check."
  exit 0
fi

info ""
info "Changed files:"
for f in "${changed[@]}"; do
  info "  - $f"
done

# ---------- 5. match each file against globs ----------

# Convert a glob to a bash extended-pattern match. The phase1-plan globs we see in practice:
#   `.github/workflows/**`
#   `backend/**`
#   `frontend/src/components/TopologyGraph/**`
#   `hack/poc/`
#   `scripts/install*.sh`
#
# We use bash's `[[ $f == $pattern ]]` with `shopt -s extglob globstar` so `**` recurses.
# Trailing slash (`hack/poc/`) → treat as `hack/poc/**`.
shopt -s extglob globstar nocaseglob 2>/dev/null || true
# Disable nocaseglob — path comparisons should be case-sensitive (Linux FS).
shopt -u nocaseglob 2>/dev/null || true

# Normalize globs: trailing slash → /**, no glob char → exact match.
normalized=()
for g in "${globs[@]}"; do
  # strip surrounding whitespace
  g="${g#"${g%%[![:space:]]*}"}"
  g="${g%"${g##*[![:space:]]}"}"
  if [[ "$g" == */ ]]; then
    g="${g}**"
  fi
  normalized+=("$g")
done

violations=0
for f in "${changed[@]}"; do
  matched=0
  for g in "${normalized[@]}"; do
    # shellcheck disable=SC2053
    if [[ "$f" == $g ]]; then
      matched=1
      break
    fi
  done
  if [[ $matched -eq 0 ]]; then
    err "Path outside Allowed Paths for ${task_id}: $f"
    violations=$((violations + 1))
  fi
done

if [[ $violations -gt 0 ]]; then
  err ""
  err "${violations} file(s) violate Allowed Paths for ${task_id}."
  err "If this is intentional (e.g. task scope expanded), update ${PLAN} \`${task_id}\` Allowed Paths block in a separate PR (RFC if shared contract)."
  exit 1
fi

info ""
info "All ${#changed[@]} changed file(s) within Allowed Paths for ${task_id}."
exit 0
