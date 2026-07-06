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

- **`main` carries the owned patches** on top of `upstream/main`. Linear: upstream tip + `patch(...)` commits.
- Upstream syncs go through the fork factory (below) on a fresh `fork-sync/<sha>` branch — **never rebase/reset main by hand.**
- Historical May-era contribution branches (`adopt-pr/*`, `codex/*`) and `archive/*` tags on origin are records; leave them.
- `agentops-patches` is the retired pre-fork patch branch (tagged `archive/pre-fork-factory-20260706-local`); superseded by owned `main`.

## Owned patches (the divergence)

Each owned commit is prefixed `patch(<area>):` and ALSO preserved as a `.patch` file in
`~/dev/agentops/docs/audits/gc-mvp-2026-07-05/patches/` (belt-and-suspenders; the fork is primary).

1. `patch(tmux)` cycle-guard `getAllDescendants` — reconciler livelock on PID-reuse cycles (age-gc-adoption-u0he.7).
2. `patch(tmux)` atomic-snapshot + identity-verified session teardown — fixes the PID-reuse kill-massacre TOCTOU (2026-07-06 session-massacre RCA).
3. `patch(beads)` strip bd `sync.remote` from canonical config — rig-add public-repo dolt-ref leak.
4. `patch(tmux)` pure `killIdentityMatches` seam + PID-reuse skip table test.

## Fork factory (sync discipline)

```bash
make fork-status     # divergence vs upstream/main + overlap warnings
make fork-preview    # non-mutating 3-way merge preview
make fork-sync       # dry-run rebase plan; ARGS=--execute on a fresh branch (main untouched)
```

After a sync: resolve conflicts on the `fork-sync/<sha>` branch, run `make check`, fast-forward `main`, push `origin main`.

## Rules

- **Upstream-first for generic correctness fixes:** anything not Bo-specific gets a PR to `gastownhall/gascity` from this fork; if merged upstream, drop the owned patch on next sync.
- **Build with `make build`** (icu4c is a keg-only CGO dep the Makefile wires; bare `go build` fails on macOS). `bin/gc` is gitignored — never commit it. Launchd plists point at `~/dev/gascity/bin/gc`; rebuilds are in-place.
- **Tests:** `make check` for the fast gates; `go test ./internal/runtime/tmux/` needs the Makefile's CGO env. Known pre-existing upstream failures (differential-verified 2026-07-06): `TestFindAgentPane_WrappedPane`, `TestWaitForCommand_WrappedPane`.
- **Tracker:** work on this fork is tracked in the agentops br ledger (`age-gc-*` beads), not here.
- **Do not** wire `internal/reviewquorum.Finalize` (the agentops-membrane pack's `finalize.jq` is deliberately stronger) and do not patch the file-backend `bd` coupling (`internal/config` is upstream's hottest surface; native bd/dolt avoids the class).
- MIT LICENSE file stays intact.
