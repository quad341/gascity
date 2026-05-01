# Release Gate: ga-a3ry.1 — supervisor binary drift detection (phases 1+2)

**Deploy beads:** ga-fd01 (phase 1), ga-xxqx (phase 2)
**Originating bead:** ga-a3ry.1
**Branch:** `gc-builder-1-01561d4fb9ea` (fork: `quad341/gascity`)
**Commits intended for release:** 3f8ccbcb, c1b203c7, a1a8c094, 62f3e26c, f7f59f54
**Verdict:** FAIL — routing back to builder for rebase

## Failure summary

The source branch `gc-builder-1-01561d4fb9ea` is a long-running development branch carrying ~100 commits unrelated to ga-a3ry.1 (ga-s760.1, ga-s760.2, ga-921b, ga-evjp, mail/beads/dolt/etc.). The deployer cannot push the whole branch — only the 5 ga-a3ry.1 commits should ship in this PR.

Cherry-picking the 5 commits onto a fresh branch off `origin/main` produces genuine content conflicts on commit `a1a8c094` ("feat(api): expose supervisor BuildID on /health, ga-a3ry.1 phase 2a"). Conflicts surface in API-surface files that have moved on `origin/main` since the source branch's merge base (`9f61eb826`, 2026-04-13):

- `cmd/gc/cmd_supervisor.go`
- `cmd/gc/controller.go`
- `cmd/gen-client/main.go`
- `cmd/genspec/main.go`
- `internal/api/openapi_sync_test.go`
- `internal/api/server.go`
- `internal/api/supervisor.go`
- `internal/api/supervisor_test.go`
- `internal/api/test_helpers_test.go`

Per the deployer prompt: "Never resolve genuine content conflicts from the deployer seat."

## Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | reviewer-1 PASS verdicts on both ga-fd01 (phase 1) and ga-xxqx (phase 2). |
| 2 | Acceptance criteria met | PASS | All TestDetectBinaryDrift / TestDetectPackDrift / TestPollReady / TestRestartLoopGuard / TestDecideDriftAction / TestPrintSupervisorIdentity / TestPrintDriftReport / TestRestartSupervisor_* / TestHTTPSupervisorClient_* / TestSupervisorHealth{Includes,Empty}BuildID / TestDaemonAutoRestartOnDrift cases passed at the source-branch HEAD. |
| 3 | Tests pass | not run | Tests not run on a clean assembled branch — cherry-pick failed. |
| 4 | No high-severity review findings open | PASS | No blockers from either review. |
| 5 | Final branch is clean | n/a | No final branch produced. |
| 6 | Branch diverges cleanly from main | **FAIL** | a1a8c094 conflicts on rebase against current origin/main. |

## Builder ask

Rebase the 5 ga-a3ry.1 commits (3f8ccbcb, c1b203c7, a1a8c094, 62f3e26c, f7f59f54) onto current `origin/main` and push to a CLEAN feature branch (e.g., `builder/ga-a3ry-1`). The current source branch carries unrelated work that must not ship in this PR. After rebase, update both ga-fd01 and ga-xxqx description/notes with the new branch name and re-route to deployer.
