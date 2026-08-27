# Release gate: `ga-yxgivi` — Dolt orphan sweeper live-flock safety

- **Result:** FAIL
- **Evaluated:** 2026-08-26 (America/Los_Angeles)
- **Reviewed commit:** `a2d50d633971c31ddaea7bf619f7e0c3c4be20c5`
- **Source branch (provenance only):** `builder/ga-63rfxj`
- **Base:** `origin/main@d7fe11583675375132ff25adc2ebb1ee252a9d84`
- **Deploy mode:** remote
- **Gate branch:** `deploy/ga-yxgivi-gate-fail-20260826-r4`

The required full-suite gate is red. No PR, push to a deployable PR head, or deploy-clearance status was created.

## Criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | PASS | Review bead `ga-dkh58q` is closed with `verdict: pass`; its recorded commit resolves exactly to the reviewed commit above. |
| 2 | Acceptance criteria met | PASS | `go test -count=1 -v ./internal/doltorphan/...` passed all 19 top-level tests. The real-Dolt acceptance check `go test -count=1 -v -tags integration ./examples/gastown/... -run '^TestSweep_ReapsRealDoltDataDirAfterSIGKILL$'` also passed. Together these exercise the second `lsof` confirmation scan and real NBS flock behavior. |
| 3 | Tests pass | **FAIL** | Required command: `GO_TEST_TIMEOUT=30m make test-local-full-parallel`; `test_cmd_scope: full-suite`. Attempt 1 completed 27/40 jobs with 20 unique top-level failures. The verbose evidence run, `GOFLAGS=-v GO_TEST_TIMEOUT=30m make test-local-full-parallel`, completed 22/40 jobs and reported 78,106 PASS, 46 FAIL, and 275 SKIP. All six diff-owned tests passed and none skipped, but the suite itself is red. `waiver_ref: none applicable`. |
| 3a | Pre-existing failures may be attributed | **FAIL** | Attempt 1 included `TestStartDrift_RestartLoopGuard_RefusesFourthInWindow`, which expected the loop guard but restarted and then encountered unavailable systemd / an already-running supervisor. No tracker covering that test predated the run. Tracker `ga-hisb4f` was opened afterward and therefore cannot satisfy the pre-existing-evidence clause for this gate. |
| 3b | Policy/lint lane | PASS | `make test-ci-policy`, `make check-gomod-replace`, `make check-native-dependency-surface`, `make check-eventexport-isolation`, `make check-core-boundary`, `make fmt-check`, `go vet ./...`, `make check-docs`, and `go build ./...` passed. `make lint` reached only three findings in ignored generated `internal/api/dashboardspa/web/node_modules/flatted/.../flatted.go`; pre-existing tracker `ga-di310j` covers this exact repository-wide lint traversal and predates the run. The diff changes only `go.mod` and `internal/doltorphan`, with no path overlap. |
| 4 | No high-severity review findings open | PASS | Reviewer recorded no blocker, high, or major finding. The noted residual time-of-check/time-of-use window was explicitly non-blocking. |
| 5 | Final branch is clean | PASS | Before writing this gate artifact, `git status --short` was empty. `git diff --check origin/main...a2d50d633971c31ddaea7bf619f7e0c3c4be20c5`, formatting, vet, docs checks, and build all passed. |
| 6 | Branch diverges cleanly from main | PASS | No PR carries the reviewed commit. `git merge-tree --write-tree origin/main a2d50d633971c31ddaea7bf619f7e0c3c4be20c5` succeeded with tree `d243ef5fff51af4481bbb26d5a999d0142db2d06`; no self-rebase was needed. |
| 7 | Single feature theme | PASS | The four-commit range, including sibling work `ga-vbyn8v`, changes only Dolt orphan-sweeper safety: `go.mod`, `internal/doltorphan/sweep.go`, and `internal/doltorphan/sweep_test.go`. |

## Diff-owned tests

Each test below reported PASS in the required verbose full-suite output:

- `TestSweep_ConfirmsUnheldWithSecondLsofScanBeforeRemoving`
- `TestSweep_SkipsConfirmScanWhenFirstScanAlreadyHeld`
- `TestSweep_RemovesWhenBothScansAgreeUnheld`
- `TestSweep_ConfirmScanErrorFailsClosed`
- `TestSweep_RespectsRealDoltLockEvenWhenLsofMissesIt`
- `TestSweep_RemovesWhenDoltLockFileExistsButIsUnheld`

The 275 full-suite skips are existing conditional or infrastructure skips; none belongs to a test added or modified by this diff. Examples include opt-in persistence coverage and an existing known-issue skip. They do not change the decisive criterion-3 failure.

The verbose full-suite run also failed `TestSweep_ReapsRealDoltDataDirAfterSIGKILL` during test setup because `dolt init` was killed. Its independent, container-capable focused run passed. That supports criterion 2, but a focused rerun cannot replace or repair the red full-suite evidence required by criterion 3.

## Logs

- First full-suite wrapper: `/var/tmp/ga-yxgivi-gate-r4.fyt38U/full-suite.log`
- First full-suite job logs: `/var/tmp/gc-local-tests.D5rE2O`
- Verbose full-suite wrapper: `/var/tmp/ga-yxgivi-gate-r4-verbose.7wWLRr/full-suite.log`
- Verbose full-suite job logs: `/var/tmp/gc-local-tests.Zrn2mp`
- Focused and policy logs: `/var/tmp/ga-yxgivi-gate-r4-verbose.7wWLRr/`

## Disposition

Route `ga-yxgivi` back to the builder with `ready-to-build`. A future independent gate run may cite `ga-hisb4f` only if that tracker still covers the observed failure and predates the new run; it cannot retroactively validate this one.
