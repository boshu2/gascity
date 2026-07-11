# Phase 7 Kickoff Prompt

Paste the block below to start the next session.

---

Continue the gascity feature-flag rollout on branch `worktree-reconciler` at
`/data/projects/gascity/.claude/worktrees/reconciler`. **PR-S2a is
code-complete**, committed, local/UNPUSHED, inert: `bec9156b1` (S2-T1),
`ec0bccd04` (S2-T2/T3-mem), `da0d073a6` (S2-T3-file), `6c0160669` (S2-T4/T5),
`3f113a52a` (S2-T6/T7 + F2), `2e634a093` (Close/Reopen refresh-adoption fix),
`cadf003ea` (S2-T8 CachingStore forward-and-EVICT + merge gate). Read
`engdocs/plans/feature-flags/PHASE7-HANDOFF.md` first — status, what Phase 6
landed, the S2b task map with DESIGN pointers, and the new gotchas
(write-through-not-verbatim, seq-protected dirty marks, the orphan-dirty
residue). Build spec: `engdocs/plans/feature-flags/PR-S2a-BUILD-SPEC.md` (keep
its Progress block current). Authoritative S2b design:
`engdocs/plans/feature-flags/DESIGN.md` §5 (factory mode-stamp), §6.4
(`ResolveConditionalWriter(store)` — NO mode parameter), §12.2 (degraded
event); the PR-S1 rollout machinery is `internal/rollout/`.

Now build **PR-S2b**:

1. **S2-T10** — the beads factory stamps the resolved
   `beads.conditional_writes` Mode onto every store it opens (ONE home;
   no caller-facing mode option), plus the thin
   `beads.ResolveConditionalWriter(store)` seam over the general rollout
   resolver returning writer / nil+once-latched-diagnostic / typed
   fail-closed refusal for require∧incapable. No consumer call sites — C4/C6
   are Stage 3.
2. **S2-T11** — register the typed `beads.conditional_writes.degraded` event
   (constant + `events.RegisterPayload` sample `{store, mode, reason,
   bd_version}`), REGISTERED only, no emission. Read
   `engdocs/architecture/api-control-plane.md` before touching
   `internal/events/`; `TestEveryKnownEventTypeHasRegisteredPayload` and the
   wire gate must stay green.
3. **S2-T12** — the `//go:build integration` BdStore conformance row against
   a #4682-capable bd (adversarial inputs + provisional body codes recorded
   in the build spec ~line 250). Must SKIP cleanly against the live incapable
   bd v1.1.0 and run for real once bd #4682 lands.

Process (non-negotiable): **bounded Fable design pass** (model `fable`,
numbered questions, facts inlined — include the factory/mode-stamp seam shape
and where the once-latch lives) → **TDD** red-first, ≤5 files per commit →
**mutation battery** (backup to `$CLAUDE_JOB_DIR/tmp`, python string-replace,
expect FAIL, `cp -f` restore — NEVER `git checkout`) → **Fable red-team BEFORE
the commit** (read-only; it proposes, you run) → full gates (both build tags,
`-race` on Conditional tests, vet, golangci-lint with the native-tag baseline
diff recipe — baseline BEFORE coding — gofumpt, wire gate) → commit with
trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.

**Do NOT push. Do NOT start S3** without checking in — S3 is outward-facing
(deploy-lineage sync + the live maintainer-city flip).
