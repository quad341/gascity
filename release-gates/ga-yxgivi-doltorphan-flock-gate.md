# Release Gate: Dolt orphan flock liveness guard

- Deploy bead: `ga-yxgivi`
- Build lineage: `ga-vbyn8v`, `ga-63rfxj`
- Review bead: `ga-dkh58q`
- Reviewed source: `a2d50d633971c31ddaea7bf619f7e0c3c4be20c5`
- Base evaluated: `origin/main@7c817e0640fae801631043005f1d54b17ce3e97c`
- Deploy mode: remote
- Overall verdict: **FAIL**

The already-merged preflight found no base-repository pull request carrying the
reviewed source. Criterion 6 passed first and no bounded self-rebase was needed.
The candidate fixes its owned regression: all six diff-owned unit tests passed,
the real SIGKILL acceptance test passed in isolation, and the complete
`examples/gastown` package passed under the required parallel union. The release
cannot be certified because that union nevertheless had 13 red shard jobs. At
least one red test passed at the exact base ref, so the failures do not satisfy
the protocol's pre-existing-failure attribution rule.

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | PASS | Review bead `ga-dkh58q` is closed with verdict PASS for exact source `a2d50d633971c31ddaea7bf619f7e0c3c4be20c5`. |
| 2 | Acceptance criteria met | PASS | `go test -tags=integration -count=1 -run '^TestSweep_ReapsRealDoltDataDirAfterSIGKILL$' -v -timeout=120s ./examples/gastown/...` passed with a real Dolt server in 33.42s (package 59.810s). The required union's `integration-packages-core-2-of-4` shard also passed the complete `examples/gastown` package in 175.652s under concurrent load. |
| 3 | Tests pass | **FAIL** | With rootless Podman configured, `GO_TEST_TIMEOUT=30m make test-local-full-parallel` ran all 40 documented local-union jobs: **27 PASS jobs, 13 FAIL jobs, 0 reported top-level SKIP jobs**. The 13 red jobs contained 14 top-level/subtest failures. `TestCityRuntimeForceShutdownTearsDownAfterLateAsyncSweep`, one non-diff-owned failure, then passed at exact `BASE_REF` (0.724s), so attribution clause 3a(iii) is missing and the criterion remains FAIL. Logs: `/var/tmp/gc-local-tests.OAvgLB`. `failure_attribution: rejected — base-ref result PASS`. `waiver_ref: none`. |
| 3b | Policy/lint lane | SKIPPED | The gate stopped after the decisive criterion-3 failure; no later policy result is claimed. |
| 4 | No high-severity review findings open | PASS | Review bead `ga-dkh58q` records no blocker/major finding and no unresolved HIGH finding. |
| 5 | Final branch is clean | PASS | The exact reviewed source was checked out detached with an empty `git status --short` before this checklist was written; this checklist is the only deployer-authored artifact. |
| 6 | Branch diverges cleanly from main | PASS | `git merge-tree --write-tree origin/main a2d50d633971c31ddaea7bf619f7e0c3c4be20c5` exited 0 and produced tree `42a8e7c9474e70f3b5b42f5d066ce8728566915f`. |
| 7 | Single feature theme | PASS | The four-commit stack changes `internal/doltorphan` plus the existing `fslock` dependency classification for one feature: fail-closed orphan removal when process-list scans miss a live Dolt holder. |

## Diff-owned test execution

`go test -count=1 -v ./internal/doltorphan/...` passed the complete package:
**19 PASS, 0 FAIL, 0 SKIP**. All six diff-owned tests executed and passed:

- `TestSweep_ConfirmsUnheldWithSecondLsofScanBeforeRemoving`
- `TestSweep_SkipsConfirmScanWhenFirstScanAlreadyHeld`
- `TestSweep_RemovesWhenBothScansAgreeUnheld`
- `TestSweep_ConfirmScanErrorFailsClosed`
- `TestSweep_RespectsRealDoltLockEvenWhenLsofMissesIt`
- `TestSweep_RemovesWhenDoltLockFileExistsButIsUnheld`

`diff_tests_executed`: all six tests above. `waiver_ref`: none.

## Full-union failure inventory

The red jobs covered timing-sensitive `cmd/gc` lifecycle tests, tmux default
key-binding reads, installed-`bd` manifest drift, review-formula/Dolt schema
collisions, and Dolt server readiness. The exact failures are preserved in the
job logs; none was rewritten as green evidence. The decisive base probe was:

```text
go test -count=1 -run '^TestCityRuntimeForceShutdownTearsDownAfterLateAsyncSweep$' -v ./cmd/gc
--- PASS: TestCityRuntimeForceShutdownTearsDownAfterLateAsyncSweep (0.00s)
PASS
ok github.com/gastownhall/gascity/cmd/gc 0.724s
```

Because this failure did not reproduce at `BASE_REF`, criterion 3a cannot be
used to certify the red union as pre-existing. No pull request or deploy
clearance was created.
