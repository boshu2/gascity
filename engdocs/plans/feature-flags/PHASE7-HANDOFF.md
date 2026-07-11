# Phase 7 Handoff — PR-S2b: ResolveConditionalWriter seam + degraded event + integration row (S2-T10/T11/T12)

Pick-up doc for the next session. **PR-S2a is code-complete** (Phases 1–6:
interface + typed errors, conformance harness, MemStore + FileStore native CAS,
BdStore classifier + probe + verbs + CAS emulation + F2 Doltlite loud-degrade,
and CachingStore forward-and-EVICT with the livelock merge gate). All still
**INERT** — zero consumers resolve a conditional writer anywhere; the
`internal/rollout` flag (`beads.conditional_writes`, Off/Auto/Require) shipped
in PR-S1 but nothing reads it on a write path yet.

The kickoff prompt is `PHASE7-PROMPT.md`. The build spec is
`PR-S2a-BUILD-SPEC.md` (Progress block current through Phase 6; keep it
current). DESIGN references mean `engdocs/plans/feature-flags/DESIGN.md`.
Unlike the Phase 4–6 handoffs, this doc was written by the Phase-6 session
without a dedicated multi-agent verification pass: every file:line below was
verified in that session, but S2b design detail is POINTERED to DESIGN
sections rather than restated — read them at source.

---

## Status — what's committed (branch `worktree-reconciler`, local/UNPUSHED)

| Commit | Task | Content |
|--------|------|---------|
| `bec9156b1` | S2-T1 | `ConditionalWriter` interface + 4 typed errors + `Bead.Revision json:"-"` + `ConditionalWriterFor` |
| `ec0bccd04` | S2-T2/T3-mem | conformance harness + MemStore native CAS + `DisableConditionalWrites` |
| `da0d073a6` | S2-T3-file | FileStore native CAS + out-of-band revision persistence |
| `6c0160669` | S2-T4/T5 | BdStore classifier + four-verb capability probe + latch |
| `3f113a52a` | S2-T6/T7 + F2 | BdStore `*IfMatch` verbs + `runConditionalWrite` + CAS emulation + Doltlite loud-degrade |
| `2e634a093` | fix | **Close/Reopen adopt the successful refresh read** (pre-existing bug: cache served the PRE-close revision forever; found by Phase 6's conformance work) |
| `cadf003ea` | **S2-T8** | CachingStore ConditionalWriter forward-and-EVICT + merge gate + conformance row (`caching_store_conditional.go` + internal/external tests) |

**Do not push** (local integration stack). Two untracked dirs are NOT ours —
`engdocs/plans/beads-cas/`, `engdocs/plans/reconciler-redesign/`. Never `git add` them.

## What Phase 6 landed that S2b consumes

- `var _ ConditionalWriter = (*CachingStore)(nil)` — so
  `ConditionalWriterFor(store)` now resolves on every store shape in the
  in-process matrix. The two production CachingStore constructions (verified
  this session): `cmd/gc/api_state.go:234` (`beads.NewCachingStore(baseStore,
  onChange)`) and `internal/runtime/t3bridge/provider.go:1700`
  (`beadStoreForWatcher`). Still inert — nothing calls the resolver.
- Cache rule (caching_store_conditional.go, read its file-header comment
  before touching anything): success+refresh → adopt fresh + write through
  what the verb proved committed; success+failed-refresh → EVICT; every
  `PreconditionFailedError` → evict + forward untouched; `(false,nil)` CAS
  loss → evict; gate-refusal/exhaustion/unsupported → no cache action;
  ambiguous → seq-protected dirty mark. Merge gate:
  `TestCachingStoreCASRetryLoopConverges`.
- The conformance matrix is green over MemStore, FileStore, and
  CachingStore(MemStore) in unit CI, both build tags, `-race` on the
  Conditional tests.

## PR-S2b tasks (the authoritative specs live in DESIGN.md — read at source)

- **S2-T10 — factory mode-stamp + `ResolveConditionalWriter(store)` seam.**
  DESIGN §5 (factory stamps the resolved Mode onto every store it opens; the
  beads factory is the ONE home — `WithConditionalWrites` as a caller option
  is explicitly deleted/forbidden), §6.4 (the seam takes NO mode parameter;
  returns writer / nil+once-latched diagnostic / typed refusal for
  require∧incapable). The rollout flag machinery is
  `internal/rollout/flag_beads_conditional_writes.go` + `resolve.go` (PR-S1,
  already shipped). Wire the thin adapter over the general resolver; consumers
  stay untouched (C4/C6 call sites are Stage 3, NOT this PR).
- **S2-T11 — `beads.conditional_writes.degraded` typed event, REGISTERED
  only.** DESIGN §12.2 (payload `{store, mode, reason, bd_version}`, latched
  once per store — the latch/emission is stage-2/3; this PR registers the
  constant + payload). Verified this session: the constant does not exist yet
  anywhere in `internal/events/` or `internal/rollout/`. CI teeth: every
  constant in `events.KnownEventTypes` needs `events.RegisterPayload` (or
  `events.NoPayload`) — `TestEveryKnownEventTypeHasRegisteredPayload` — and
  the wire gate `go test ./internal/api/ -run 'OpenAPISpecInSync|EventPayload'`
  must stay green. Read `engdocs/architecture/api-control-plane.md` before
  touching `internal/events/`.
- **S2-T12 — `//go:build integration` BdStore conformance row** against a
  #4682-capable bd. PR-S2a-BUILD-SPEC.md line ~250 records the adversarial
  inputs and the provisional body codes
  (`precondition-failed`/`conditional-write-unsupported`) this row is the
  authoritative guard for. Slots into the contract-test system (memory:
  `contract-test-system-build`, PR #3714). The live bd (v1.1.0) has NO
  `--if-revision` — the row must skip cleanly (not fail) against an incapable
  bd, and run for real once #4682 lands.
- S2-T9 sqlite stays deferred out of S2.

## Gotchas — new ones learned in Phase 6 (standing ones: see PHASE6-HANDOFF)

- **Backing reads can lag this process's own committed writes.** The
  `staleAfterCloseStore` tests (caching_store_test.go:2764) pin write-through
  behavior on Close; "adopt the fresh read verbatim because it must be
  our-write-or-newer" is a FALSE premise in this codebase. Any future refresh
  logic must write through what the write proved committed. The converse
  (refresh observing a LATER state gets stomped) is a documented, accepted
  hazard — see the caching_store_conditional.go header.
- **Dirty marks need seq protection.** Setting `c.dirty[id]` without
  `noteLocalMutationLocked` lets a concurrent `List(Live)` merge-back
  (caching_store_reads.go:~233) or prime rebuild erase the mark and install a
  stale row as clean. Red-team found this in Phase 6's first cut; the
  regression test is `TestCachingStoreAmbiguousConditionalFailureDirtySurvivesConcurrentScan`
  (uses `casBackingStore.onListOnce` to fire mid-scan, deterministic).
- **Known residue (documented, NOT fixed):** a tolerated CloseIfMatch over a
  close-hiding backing leaves an orphan `dirty` entry that only the
  quiet-window reconcile branch reaps; while present, cached list serving
  degrades to backing pass-through. The proper fix is in `runReconciliation`'s
  concurrent-mutation branch (reap dirty ids in neither freshByID nor
  c.beads). Pre-existing class (unconditional Update's not-cached
  refresh-fail branch also orphans); fix belongs in its own change if it
  starts mattering.
- **Test-fake promotion trap (bit again, by design):** every backing wrapper
  in cache CAS tests must DEFINE the four verbs delegating inward
  (`casBackingStore` in caching_store_conditional_internal_test.go is the
  reusable one: Get/CAS counting, fail-next-get, stale-next-get,
  hide-closed, err-override, onListOnce).
- The native-tag lint baseline recipe works as documented (32 pre-existing
  doltlite issues; diff the before/after files, require zero new lines).
- The pre-commit hook (absolute hooksPath into the main checkout) ran clean
  from this worktree in Phase 6 — lint-changed + doc-gen + vet + docsync all
  green; no `--no-verify` needed.

## Process (the user's standing method — non-negotiable)
1. **Bounded Fable design pass** (Agent tool, `model: "fable"`, numbered
   questions, facts inlined, DECISION + short justification). Phase 6's pass
   settled 8 questions; 2 of its recommendations were later amended by test
   evidence (write-through vs verbatim) — inline the `staleAfterCloseStore`
   constraint in any future cache-semantics question.
2. **TDD red-first**, ≤5 files per commit; split pre-existing-bug fixes into
   their own commit (Phase 6 precedent: `2e634a093`).
3. **Mutation battery**: backup to `$CLAUDE_JOB_DIR/tmp`, python
   string-replace, run the target subtest expect FAIL, `cp -f` restore,
   `diff` byte-identical. NEVER `git checkout`.
4. **Fable red-team BEFORE the commit** (read-only on the shared worktree; it
   PROPOSES, the main session RUNS). Phase 6's found 1 real BUG + 5 killed
   mutants — it pays.
5. **Full gates** (both build tags, `-race` on Conditional, vet,
   golangci-lint with the native baseline diff, gofumpt, wire gate), then
   commit with trailer
   `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
   **Do NOT push.**

## After PR-S2b
- **Checkpoint with the user before S3** — S3 is outward-facing
  (deploy-lineage sync + the live maintainer-city flip). Do not start it
  unprompted.
