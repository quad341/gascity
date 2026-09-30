**Verdict:** **FAIL**

# PR #6821 shared real-bd fixture cleanup re-gate

Deploy bead `ga-he21s7`; exact reviewed PR head `8e39cfb1c5e2b6bcf4df11bc301ca1471e34cca3`. The full test suite passed, but criterion 3b fails because `make bazel-sync` changes a tracked BUILD file on the merge-validated tree. No deploy clearance or merge request may be published for this head.

## Criteria

| # | Result | Evidence |
|---|---|---|
| 1 Review | PASS | Reviewer bead `ga-h6fznz` records PASS on the exact PR head; `gh pr view 6821` still reports that head. |
| 2 Acceptance | PASS | All 23 diff-owned tests have a PASS. Twenty ran in the documented full suite; the three acceptance-tagged tests ran and passed separately with the CI-pinned bd. |
| 3 Tests and policy | **FAIL (3b)** | `make test-local-full-parallel LOCAL_TEST_JOBS=4` passed all 40 jobs. `make test-ci-policy` passed. On the same materialized merge tree, `make bazel-sync` added `//internal/beads/beadstest` to `go_test.deps` in `test/acceptance/tier_b/BUILD.bazel`. The PR adds that import to `pool_workquery_test.go` but omits its Bazel dependency. `git diff --exit-code` failed with this one-line generated diff. |
| 4 Review findings | PASS | The exact-head reviewer recorded no open HIGH finding. P4 follow-ups are tracked in `ga-p6nwt8`. |
| 5 Branch clean | PASS | The reviewed PR branch remains unchanged at `8e39cfb1c5e2b6bcf4df11bc301ca1471e34cca3`. The test worktree was clean before the intentional `bazel-sync` probe. |
| 6 Main merge | PASS | Against frozen base `0e53ee7f48a4d68da4ba95cf1135f518c6e43ac5`, the materialized merge tree was `0b7d9db9a7a5ca3718634ededa54f2463ee13194`; `go build ./...` and `go vet ./...` passed there. A fresh merge-tree against current `origin/main` `ada6ccd6964956506a593cbc3d0dbf529e7a8bb4` is conflict-free (`54d9ddef60a85e4884acb0e388e350e5211b4092`). The later main commits are not part of the full-suite run. |
| 7 Theme | PASS | The PR is one shared real-bd fixture cleanup and HOME-isolation change, with matching tests, CI and resource-census updates. |

## Criterion 3 evidence

- `test_cmd: make test-local-full-parallel LOCAL_TEST_JOBS=4` through `load-gate-run.sh` and `isolated-test-run.sh` in existing detached service `gc-heavy-ga-he21s7.c3-1790752956-3344789.service`. `GATE_RUN_EXIT rc=0 state=complete`; `test_cmd_scope: full-suite`; 40/40 jobs passed.
- `test_counts: 97,448 PASS, 0 FAIL, 322 SKIP` test and subtest terminal lines from 40 shard logs. Four diff-owned `cmd/gc` process tests had expected fast-partition SKIPs and PASS in the required process partition. Remaining skips were not fully attributed because criterion 3b already fails.
- `diff_tests_executed: 23 PASS, 0 FAIL, 0 unexecuted`. The 20 full-suite owned tests passed by name; supplementary acceptance runs passed `TestRunBDIsolatesHOMEFromSharedServerConfig`, `TestBdRunWithEnvIsolatesHOMEFromSharedServerConfig`, and `TestPoolWorkQueryFromWorktree` with zero FAIL or SKIP. Ownership came from `diff-owned-tests.py` against `origin/main...8e39cfb1c5`.
- `test_bd: GASCITY_TEST_BD version=v1.3.1-rc.2`, bin `/home/jaword/.local/bd-versions/v1.3.1-rc.2/bd`, ref check matched the repo pin. `GC_INTEGRATION_BD_PERSISTENCE=1` enabled nine Dolt persistence tests.
- `policy_lane: run-pinned-lint.sh -- make test-ci-policy -> PASS`; `GLT_RECORD: {"golangci_lint":{"pin":"2.12.0","resolved":"2.12.0","mismatch":false}}`. `make bazel-sync -> command PASS, idempotence FAIL`: one generated dependency line was missing. Formatting check was not run after the decisive fail.
- `heavy_mode: none`; `waiver_ref: none`; `ci_lane_run: Linux CI 36653499623 completed success on the exact reviewed head`, including the new HOME-isolation steps. The changed Mac acceptance job remains assigned to post-merge verification under operator decision `ga-yv9wqa` and bead `ga-cdr5n7`.
- `load_threshold=15; load_waited_seconds=1380; load_wait_timed_out=0; load_start=28.43; load_max=46.72; load_mean=26.79; samples=127; read_errors=0`.
- Raw evidence: `/var/tmp/deploy-ga-he21s7-regate.GZZQAK/{full-suite.log,shards/,acceptance-bd.log,acceptance-b.log,policy.log,bazel-sync.log,bazel-sync.diff}` and `/var/tmp/gc-heavy-gate/runs/ga-he21s7.c3`.

## Disposition

Return this technical gate failure to the builder. Add the generated `//internal/beads/beadstest` dependency, rerun `make bazel-sync` to verify an empty diff, obtain a reviewer PASS on the new exact PR head, and re-gate. PR #6821 stays unchanged with no clearance status and no merge request from this run.
