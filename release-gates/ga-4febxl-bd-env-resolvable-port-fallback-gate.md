# Release Gate: bd_env Resolvable Dolt Port Fallback

Date: 2026-06-06

Deploy bead: ga-4febxl
Source bug: ga-crh00
Review bead: ga-slqpm2
PR: https://github.com/gastownhall/gascity/pull/3146
Reviewed head: 96ccfefd1e576de3ed3f633d311042155cb68489
Base: origin/main at 2538b89a6b8b469c61f17d855b3ffa0855788ed5

Note: `docs/PROJECT_MANIFEST.md` is not present in this checkout. This gate uses the deployer release-gate criteria and `TESTING.md` as the available release/test references.

## Summary

The change adds a last-resort recovery path in `resolvedRuntimeCityDoltTarget`: when normal managed Dolt port publishing or recovery fails, and recovery is allowed, the supervisor can read the port from provider state through `currentResolvableManagedDoltPort`. The fallback remains guarded by `managedDoltLifecycleOwned`, so it does not return ports for unmanaged Dolt processes.

## Gate Criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | `bd show ga-slqpm2` is closed with close reason `pass` and notes contain `REVIEWER VERDICT: PASS (auto-merge)`. |
| 2 | Acceptance criteria met | PASS | `ga-crh00` requires supervisor bd subprocesses to avoid resolving managed Dolt as `127.0.0.1:0` when publish state is unavailable. `cmd/gc/bd_env.go` now falls back to `currentResolvableManagedDoltPort` under the existing ownership guard, and the regression test covers the publish-write-failure path. |
| 3 | Tests pass | PASS | `go test ./cmd/gc -run 'TestResolvedRuntimeCityDoltTargetFallsBackToResolvablePortWhenPublishWriteFails\|TestBdRuntimeEnv' -count=1` passed (`ok github.com/gastownhall/gascity/cmd/gc 4.117s`). `make test-fast-parallel` passed all fast jobs. `go vet ./...` passed. PR CI for reviewed head had required checks green before the gate commit. |
| 4 | No high-severity review findings open | PASS | Reviewer notes list only INFO/PASS findings; no HIGH findings are present or unresolved. |
| 5 | Final branch is clean | PASS | Isolated deploy worktree was clean before writing this gate. After committing the gate, `git status --short --branch` was checked again and showed no uncommitted changes. |
| 6 | Branch diverges cleanly from main | PASS | `git merge-tree --write-tree origin/main HEAD` exited 0 and produced tree `d98a91c46e46a01c0ceab05d4747bda2f992ee23`. GitHub merge state was `CLEAN` at review time. |
| 7 | Single feature theme | PASS | Commit set touches only `cmd/gc/bd_env.go` and `cmd/gc/bd_env_test.go`, all for the managed Dolt port fallback used by bd subprocess environment resolution. |

## Commands Run

```bash
go test ./cmd/gc -run 'TestResolvedRuntimeCityDoltTargetFallsBackToResolvablePortWhenPublishWriteFails|TestBdRuntimeEnv' -count=1
make test-fast-parallel
go vet ./...
git merge-tree --write-tree origin/main HEAD
git status --short --branch
git config core.hooksPath
```

## Verdict

PASS. The branch is eligible for PR release evaluation and merge-authority handoff.
