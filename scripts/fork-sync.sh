#!/usr/bin/env bash
# fork-sync.sh — fork-factory helper. Canonical template (skill: fork-maintenance).
#
# Copy into <fork>/scripts/fork-sync.sh and customize the 3 marked spots:
#   [CUSTOMIZE 1] upstream URL in require_upstream()
#   [CUSTOMIZE 2] test-command hint in cmd_sync()   (make test | cargo test | …)
#   [CUSTOMIZE 3] overlap keywords in cmd_status()  (topics likely to collide on sync)
#
# This fork carries owned patches on an upstream we don't control. Pull upstream fixes
# WITHOUT losing our patches. No auto-merge: a sync is a real rebase that can conflict,
# so the default is read-only reporting + a non-mutating conflict preview.
#
# Usage:
#   scripts/fork-sync.sh status            # ahead/behind, behind-commits, owned-commits, overlap warnings
#   scripts/fork-sync.sh preview           # non-mutating 3-way merge preview: which files would conflict
#   scripts/fork-sync.sh sync [--execute]  # rebase owned patches onto upstream on a fresh branch (dry by default)
#
# Safe by default: status/preview never mutate; sync is dry unless --execute and refuses
# on a dirty tree. Never force-pushes; never touches main directly.
set -euo pipefail

UPSTREAM_REMOTE="${FORK_UPSTREAM_REMOTE:-upstream}"
UPSTREAM_BRANCH="${FORK_UPSTREAM_BRANCH:-main}"
BASE="${UPSTREAM_REMOTE}/${UPSTREAM_BRANCH}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

die() { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; exit 1; }
hdr() { printf '\n\033[1m%s\033[0m\n' "$*"; }

require_upstream() {
  git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1 \
    || die "no '$UPSTREAM_REMOTE' remote. Add it: git remote add $UPSTREAM_REMOTE https://github.com/gastownhall/gascity.git"  # [CUSTOMIZE 1]
}

fetch_upstream() { hdr "Fetching ${UPSTREAM_REMOTE}..."; git fetch "$UPSTREAM_REMOTE" --quiet; }

cmd_status() {
  require_upstream; fetch_upstream
  local ahead behind forkpoint
  ahead="$(git rev-list --count "${BASE}..HEAD")"
  behind="$(git rev-list --count "HEAD..${BASE}")"
  forkpoint="$(git merge-base HEAD "$BASE")"

  hdr "Divergence vs ${BASE}"
  printf '  ahead  (owned patches) : %s\n' "$ahead"
  printf '  behind (upstream fixes): %s\n' "$behind"
  printf '  fork-point            : %s  %s\n' "$(git rev-parse --short "$forkpoint")" "$(git log -1 --format=%s "$forkpoint")"
  printf '  our HEAD              : %s  %s\n' "$(git rev-parse --short HEAD)" "$(git log -1 --format=%s HEAD)"

  hdr "Upstream commits we are BEHIND (${behind}) — fixes a sync would pull in"
  git log --oneline "HEAD..${BASE}" | head -40 || true

  hdr "Our OWNED commits ahead of upstream (${ahead}) — what a sync must replay"
  git log --oneline "${BASE}..HEAD" || true

  hdr "Overlap warnings (topics changed on BOTH sides → likely conflict on sync)"
  local found=0 kw
  for kw in tmux supervisor kill beads sling reviewquorum launchd session; do  # [CUSTOMIZE 3]
    if git log --oneline "${BASE}..HEAD"   | grep -qi "$kw" \
    && git log --oneline "HEAD..${BASE}"   | grep -qi "$kw"; then
      printf '  ⚠ "%s" — changed in both owned and upstream commits; expect conflicts\n' "$kw"
      found=1
    fi
  done
  [ "$found" -eq 0 ] && printf '  (none detected by keyword heuristic)\n'
  printf '\nNext: scripts/fork-sync.sh preview   (non-mutating conflict preview)\n'
}

cmd_preview() {
  require_upstream; fetch_upstream
  local mergebase; mergebase="$(git merge-base HEAD "$BASE")"
  hdr "3-way merge preview (non-mutating): HEAD vs ${BASE}"
  local out; out="$(git merge-tree --write-tree --name-only HEAD "$BASE" 2>/dev/null || true)"
  if printf '%s' "$out" | grep -qiE 'CONFLICT|<<<<<<<'; then
    printf '\033[31m  Conflicts detected. Files/regions:\033[0m\n'
    printf '%s\n' "$out" | grep -iE 'CONFLICT|^[A-Za-z]' | head -40
  elif [ -n "$out" ]; then
    printf '\033[33m  merge-tree reported changes; review before syncing:\033[0m\n'
    printf '%s\n' "$out" | head -40
  else
    printf '\033[32m  Clean — no conflicts predicted against %s.\033[0m\n' "$BASE"
  fi
  printf '\nfork-point: %s\n' "$(git rev-parse --short "$mergebase")"
}

cmd_sync() {
  local execute=0
  [ "${1:-}" = "--execute" ] && execute=1
  require_upstream
  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "tracked changes present — commit/stash first."
  fi
  fetch_upstream
  local cur branch
  cur="$(git rev-parse --abbrev-ref HEAD)"
  branch="fork-sync/$(git rev-parse --short "$BASE")"
  hdr "Plan: rebase owned patches from '$cur' onto ${BASE} on new branch '$branch'"
  git log --oneline "${BASE}..HEAD" || true
  if [ "$execute" -eq 0 ]; then
    printf '\n\033[33mDRY RUN.\033[0m Re-run with --execute to perform:\n'
    printf '  git switch -c %s %s && git rebase %s\n' "$branch" "$cur" "$BASE"
    printf '  # resolve conflicts, run make check (CGO: icu4c via Makefile), then fast-forward main + push origin\n'  # [CUSTOMIZE 2]
    return 0
  fi
  git switch -c "$branch"
  printf '\033[33mRebasing onto %s. Resolve conflicts, then run make check (icu4c CGO via Makefile).\033[0m\n' "$BASE"  # [CUSTOMIZE 2]
  git rebase "$BASE" || die "rebase hit conflicts — resolve, 'git rebase --continue', then re-run tests. main is untouched."
  hdr "Rebase clean on '$branch'. Validate before fast-forwarding main."
}

case "${1:-status}" in
  status)  cmd_status ;;
  preview) cmd_preview ;;
  sync)    shift; cmd_sync "${1:-}" ;;
  *) die "usage: fork-sync.sh {status|preview|sync [--execute]}" ;;
esac
