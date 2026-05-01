# Release Gate: ga-s760.1 + ga-s760.2 — fingerprint versioning + per-entry CopyFiles drift diff

**Deploy beads:** ga-34pf (s760.1), ga-it5y (s760.2)
**Source branch:** `gc-builder-1-01561d4fb9ea` (fork)
**Commits intended for release:**
- ga-s760.1: c7f8054e (test), c5ee4e75 (impl)
- ga-s760.2: fa056311 (test), 5edf7cce (drop legacy test), b658d67c (impl)

**Verdict:** FAIL — routing back to builder for clean feature branches

## Failure summary

Same root cause as ga-a3ry.1: the source branch `gc-builder-1-01561d4fb9ea` carries ~100 unrelated commits, so the deployer cannot push the whole branch via the single-bead path. The deployer prompt's rollup-ship path requires explicit `CHERRY_PICKS` and ideally `rollup-ship` label — neither is present.

Cherry-pick verification:

| Bead | Commits | Cherry-pick result |
|------|---------|-------------------|
| ga-34pf (s760.1) | c7f8054e, c5ee4e75 | clean against origin/main |
| ga-it5y (s760.2) | fa056311, 5edf7cce, b658d67c | **conflict on b658d67c** in `cmd/gc/session_reconciler.go` (LogCoreFingerprintDrift signature change collides with new code on origin/main) |

ga-34pf cherry-picks cleanly today, but routing it through ad-hoc cherry-picks (without `rollup-ship` semantics) violates deployer ZFC. Both should be on clean feature branches.

## Builder ask

Push the s760.1 + s760.2 commits to clean feature branches rebased on current `origin/main`:

- `builder/ga-s760-1`: c7f8054e + c5ee4e75 rebased onto origin/main
- `builder/ga-s760-2`: fa056311 + 5edf7cce + b658d67c rebased onto origin/main (likely needs s760.1 to land first OR rebased on the s760-1 branch)

For ga-s760.2: resolve the b658d67c conflict in `cmd/gc/session_reconciler.go` — origin/main has a code path that calls `runtime.LogCoreFingerprintDrift(stderr, name, storedBreakdown, agentCfg)` after unmarshaling the metadata to a `map[string]string`. b658d67c changed the signature to take a JSON string directly, so the call site needs to be re-updated against current main.

After re-push, update both bead descriptions with the new branch names and re-route to deployer.
