# Release Gate: PR #1177 gc status prefetch

**Verdict:** FAIL

- Deploy bead: `ga-avh93`
- Existing PR: https://github.com/gastownhall/gascity/pull/1177
- Branch checked: `tracking/ga-bxq5-gc-status-perf-blocked`
- Head checked: `6cab176c853a60766f297b7a8a51c065e1ac449d`
- Required prerequisite head now checked: `fork/feat/adr-0001-status-routing` at `36c06ab02f78189db7b14affdc3edfa0d288207f`
- Manifest note: `docs/PROJECT_MANIFEST.md` is not present in this worktree. This gate applies the deployer prompt's six release criteria plus the repo guidance in `TESTING.md`.

## Criteria

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | `ga-avh93` notes contain `VERDICT: pass` and `Findings: none` for the incremental PR #1177 diff. |
| 2 | Acceptance criteria met | FAIL | The incremental test change is reviewed, but the stacked branch is stale against its required prerequisite. PR #1177 was rebased onto PR #1149 at `6a029a4d2`; deployer then added the #1149 release-gate commit `36c06ab02`. `git diff --stat fork/feat/adr-0001-status-routing..fork/tracking/ga-bxq5-gc-status-perf-blocked` shows PR #1177 would delete `release-gates/pr-1149-status-routing-gate.md` and add only `cmd/gc/city_status_snapshot_test.go`. That is not an acceptable release stack. |
| 3 | Tests pass | PASS | Current GitHub checks for PR #1177 show required CI passing: 77 pass, 15 skipped by policy, and 2 older cancelled runs from superseded attempts. Builder also reported focused regression, `go test ./cmd/gc`, `go vet ./...`, `git diff --check`, `make test-fast-parallel`, and `.githooks/pre-commit` passing. Deployer did not run a local suite after criterion 2 failed. |
| 4 | No high-severity review findings open | PASS | Review notes list no findings and no HIGH issues. |
| 5 | Final branch is clean | PASS | `git status --short --branch` was clean before writing this gate artifact. |
| 6 | Branch diverges cleanly from main | PASS | GitHub reports `mergeStateStatus: CLEAN`; the failure is not a merge conflict with `origin/main`, it is stale stacked-branch content relative to the required #1149 prerequisite. |

## Failure Diagnosis

PR #1177 is intended to merge after PR #1149. The branch currently contains the
#1149 code through `6a029a4d2`, then the PR #1177 regression-test commit
`6cab176c8`. PR #1149 is now at `36c06ab02` because the deployer added
`release-gates/pr-1149-status-routing-gate.md` and pushed it to the PR head.

If PR #1177 proceeds without being rebased, it will be stale against the
prerequisite stack and can remove the #1149 release-gate artifact after #1149
merges. Deployer must not rebase or force-push the contributor branch from this
seat.

## Required Builder Action

Rebase or otherwise refresh PR #1177 onto one of:

- current `fork/feat/adr-0001-status-routing` at `36c06ab02`, if #1149 is still open; or
- current `origin/main`, after #1149 is merged by a human.

After the refresh, the incremental diff over the prerequisite should be limited
to `cmd/gc/city_status_snapshot_test.go` plus a new release gate for PR #1177,
with no deletion of `release-gates/pr-1149-status-routing-gate.md`.
