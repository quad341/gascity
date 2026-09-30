**Verdict:** **FAIL**

# PR #6821 shared real-bd fixture cleanup re-gate

Deploy bead `ga-he21s7`; reviewed source `8e39cfb1c5e2b6bcf4df11bc301ca1471e34cca3`; existing own PR [#6821](https://github.com/gastownhall/gascity/pull/6821). The gate is held on the first observed parallel tmux sweep report failure, tracker `ga-ok9ick`. No push, PR update, clearance status or merge-request is authorized from this run. This is a dependency hold for fresh evaluation after a fix lands, not a claim that the feature code caused the failure.

## Criteria

| # | Result | Evidence |
|---|---|---|
| 1 Review | PASS | Review bead `ga-h6fznz` records PASS on the exact resolved source commit. |
| 2 Acceptance | PASS | Source inspection found guarded temporary-directory cleanup, test-owned HOME and bd test-mode overrides applied at the relevant real-bd fixtures. The reviewer exercised all 23 changed test bodies by name; the fresh full-scope run exercised 20 of them, with the three acceptance-only bodies unexecuted in this held gate. Reviewer evidence is context, not criterion-3 substitute. |
| 3 Tests | FAIL | Documented 40-job full-scope target completed 39 jobs successfully and one failed. Exact test result was `TestSweepStaleTmuxTestServers_ReapsRootGoneKeepsRootPresent`: its fixture PID disappeared, while the test's own sweep report was empty. That unchanged file is outside the diff, but no independent proof establishes why it disappeared. The diff raises the subprocess census baseline 717 to 723 and adds test work, so the inconclusive attribution guard refuses a PASS. The three acceptance-only changed bodies also need independent execution on a future complete gate. |
| 4 Review findings | PASS | Exact-source reviewer recorded no unresolved HIGH finding. Its P4 findings are tracked separately in `ga-p6nwt8`. |
| 5 Branch clean | PASS | The disposable tested checkout and the role worktree were clean before this gate record was written; no feature change was made by the deployer. This record is committed only to a local isolated gate branch. |
| 6 Main merge | PASS | Frozen base `d1aa1cd5237d451cdacadce9a74bf8c6ddaa4586` merged without conflict; materialized tree `439184e5930535365a238ed928be83af842ecb99` matched merge-tree. Main later advanced to `7072b91ea8a9b502687383c6e96b892a5d1f23c1`. A fresh merge-tree against that base exits 0 with tree `ba3173de550ada70a411a67c61211da709ba3a63`; GitHub reports PR #6821 mergeable. No self-rebase or force push was attempted. |
| 7 Theme | PASS | The 29-file PR is one shared real-bd fixture cleanup and HOME-isolation theme; CI, tests and resource census support it. No independent feature theme was found. |

## Criterion 3 evidence

- `test_cmd: make test-local-full-parallel LOCAL_TEST_JOBS=4`, through `load-gate-run.sh` and `isolated-test-run.sh` in systemd user service `deploy-ga-he21s7-suite.service`; `test_cmd_scope: full-suite` for the documented core/process/integration target.
- `test_counts: 54,344 PASS, 1 FAIL, 227 SKIP` test/subtest terminal lines; 39 job PASS markers, one job FAIL marker. Raw shard logs and exact test-name map: `/var/tmp/deploy-ga-he21s7.6xsgmI`.
- `diff_tests_executed: 20/23 changed bodies have PASS in the full-scope logs`. Four cmd/gc bodies also have expected partition SKIPs in a fast integration shard, but each passed in the required real-process shard. `TestRunBDIsolatesHOMEFromSharedServerConfig`, `TestBdRunWithEnvIsolatesHOMEFromSharedServerConfig` and `TestPoolWorkQueryFromWorktree` are acceptance-tier bodies absent from this 40-job target; their supplementary lane was not run after the blocking failure. No owned body failed in the executed logs.
- `failure_attribution: none`. `ga-ok9ick` was opened during this run, so it cannot establish a pre-existing condition without a landed independent clause-3 proof. The test's own comment names a possible sibling-sweep race, but the current log does not identify the other sweeper. Earlier records of this test's socket-path error are a different condition. The census bump/new test load trips the inconclusive guard. The failing file `cmd/gc/tmux_leak_guard_check_test.go` is unchanged, but this fact alone is insufficient for attribution.
- `policy_lane: not run after criterion-3 blocker`; `waiver_ref: none` (Gas City has no waiver path). `heavy_mode: none` from `heavy-composite-gate.sh classify`.
- `ci_lane_run: [Linux CI run 36653499623](https://github.com/gastownhall/gascity/actions/runs/36653499623) completed success on the exact reviewed head`, including the three added HOME-isolation steps in minimum-supported, current and main-HEAD contract jobs. The changed Mac acceptance job has not run on this fork PR. Operator decision `ga-yv9wqa` explicitly hands that first real run to post-merge verification bead `ga-cdr5n7`; a skipped Mac summary is not recorded as execution proof.
- `load_threshold=15; load_waited_seconds=0; load_wait_timed_out=0; load_start=13.40; load_max=39.71; load_mean=25.37; samples=86; read_errors=0`. Rootless Podman socket was present and the repository's pinned Dolt image tags were cached. Every test ran with CI-pinned bd v1.3.1-rc.2 first on PATH; the nested Go runner set `GC_TEST_BD_BIN`, `GC_INTEGRATION_REAL_BD`, `GC_ACCEPTANCE_BD_BIN` and the persistence opt-in.

## Hold and exit

The exact failure is recorded on tracker `ga-ok9ick`, linked by a same-store blocks dependency from `ga-he21s7`. Reproduce the condition independently on an untouched base or unrelated PR, then file one fix bead stamped `gc.fixes_tracker=ga-ok9ick`; re-point this deploy bead's dependency to that fix. Keep the tracker open until the fix actually lands. After landing, release the dependency, obtain a fresh review if the PR head changes, and run the full deploy gate again. PR #6821 remains at the reviewed source head and receives no clearance from this failed evaluation.
