# boshu2/gascity — owned fork contract

> This is Bo's **owned fork** of [gastownhall/gascity](https://github.com/gastownhall/gascity)
> (MIT — LICENSE preserved intact). Decision: 2026-07-06, after the fork carried its second and
> third load-bearing patches. Prior posture ("read-only managed fork, never push") is retired.
> Fact-owner for divergence state: `dotfiles/claude/reference/FORKS-MAP.md` **F-4**.

## Remotes and branches

| Remote | URL | Role |
|---|---|---|
| `origin` | `git@github.com:boshu2/gascity.git` | the owned fork — push here |
| `upstream` | `https://github.com/gastownhall/gascity.git` | Steve Yegge's line — read-only, PR target |

- **`main` carries the owned patches** and periodically merges `upstream/main`, preserving both histories without force-pushing.
- Upstream syncs go through the fork factory (below) on a fresh `fork-sync/<sha>` branch — **never merge/rebase/reset main by hand.**
- Historical May-era contribution branches (`adopt-pr/*`, `codex/*`) and `archive/*` tags on origin are records; leave them.
- `agentops-patches` is the retired pre-fork patch branch (tagged `archive/pre-fork-factory-20260706-local`); superseded by owned `main`.

## Owned patches (the divergence)

Each owned commit is prefixed `patch(<area>):` and ALSO preserved as a `.patch` file in
`~/dev/agentops/docs/audits/gc-mvp-2026-07-05/patches/` (belt-and-suspenders; the fork is primary).

1. `patch(tmux)` cycle-guard `getAllDescendants` — reconciler livelock on PID-reuse cycles (age-gc-adoption-u0he.7).
2. `patch(tmux)` atomic-snapshot + identity-verified session teardown, including the upstream-PR review follow-up — fixes the PID-reuse kill-massacre TOCTOU (2026-07-06 session-massacre RCA, upstream PR #3985).
3. `patch(tmux)` pure `killIdentityMatches` seam + PID-reuse skip table test.
4. `patch(tmux)` degraded-server latch-and-recover — bounded escalation instead of permanent spawn refusal.
5. `patch(beads)` strip bd `sync.remote` from canonical config — rig-add public-repo dolt-ref leak.
6. `patch(supervisor)` launchd KeepAlive + throttle interval — restart parity with systemd.
7. `patch(dispatch)` require the engine attempt-log fingerprint before a Ralph gate can finalize PASS.
8. `patch(dispatch)` opt-in terminal HOLD exit for checked-step escalation — closes the
   logical step without spawning another retry while preserving default nonzero semantics.
9. `chore(fork)` the fork contract and history-preserving sync factory.

## Fork factory (sync discipline)

```bash
make fork-status     # divergence vs upstream/main + overlap warnings
make fork-preview    # non-mutating 3-way merge preview
make fork-sync       # dry-run merge plan; ARGS=--execute on a fresh branch (main untouched)
```

After a sync: resolve and stage conflicts on the `fork-sync/<sha>` branch, run `make check`, commit the merge, fast-forward `main`, then push `origin main` normally. The merge-based procedure is deliberate: rebasing owned commits would require rewriting `main`, so it cannot be followed by a fast-forward.

## Rules

- **Upstream-first for generic correctness fixes:** anything not Bo-specific gets a PR to `gastownhall/gascity` from this fork; if merged upstream, drop the owned patch on next sync.
- **Build with `make build`** (icu4c is a keg-only CGO dep the Makefile wires; bare `go build` fails on macOS). `bin/gc` is gitignored — never commit it. Launchd plists point at `~/dev/gascity/bin/gc`; rebuilds are in-place.
- **Tests:** `make check` for the canonical gate; `go test ./internal/runtime/tmux/` needs the Makefile's CGO env. On bo-mac, the static portions of `make check` pass, while the full Go phase has ten upstream-baseline failures differential-verified against clean `upstream/main` on 2026-07-18: the two wrapped-pane tmux tests; two managed-Dolt environment tests in `cmd/gc`; two `/bin/true` hook tests; two managed-Dolt connection tests in `internal/beads/contract`; and the two deep-path product-metrics test groups. A fork sync is acceptable only when its remaining failures reproduce on the exact upstream SHA and every fork-touched package passes after excluding those named baseline cases.
- **Tracker:** work on this fork is tracked in the agentops br ledger (`age-gc-*` beads), not here.
- **Do not** wire `internal/reviewquorum.Finalize` (the agentops-membrane pack's `finalize.jq` is deliberately stronger) and do not patch the file-backend `bd` coupling (`internal/config` is upstream's hottest surface; native bd/dolt avoids the class).
- MIT LICENSE file stays intact.
