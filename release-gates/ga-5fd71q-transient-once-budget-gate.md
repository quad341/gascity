# Release gate: retain transient-once retry budget, PR #6379

**Verdict:** **PASS**

Evaluated 2026-09-29T06:39:13.759766+00:00.

Deploy bead `ga-5fd71q`; fix `ga-662krp`; designated tracker `ga-b2gik5`.

This fresh re-gate preserves [our existing PR #6379](https://github.com/gastownhall/gascity/pull/6379) and its source head. The isolated evidence branch will add only this record; it is not a replacement PR or a branch to merge instead of the existing PR. The bead's explicit current-head contract supplies the exact-head publication path.

## Coordinates

- Remote mode; push remote fork (`quad341/gascity`); base repo `gastownhall/gascity`.
- Reviewed/live source: `0f982c49fad8668e7e26f6a3d5e2832d4b1567a7`, `fix/ga-njhh8e-transient-once-budget`. Object resolved and PR author quad341/head verified.
- Captured base: `ff1a334d604197ddc129a8bafc29547404196a2a`.
- Canonical materialized merge: `073bfa48930e302f4df6fa9db89da2c180d5a72e`.
- Tree: `00f72fb784741b7ee62fce0e3addb5d4816c569e`; verified equal to merge-tree at the captured base/source, including after active pre-commit materialization.
- Artifacts: `/var/tmp/ga-5fd71q-gate.jdx4t1sy`; full logs, event parser, skip ledger, runner, policy logs, and attribution measurements remain there. Scratch checkout and private HOME are temporary and removed by the runner's EXIT trap.
- `docs/PROJECT_MANIFEST.md` is absent. Supplied release-gate criteria, `TESTING.md`, and the Makefile define this gate.

## Criteria

| # | Result | Evidence |
|---|---|---|
| 1. Review PASS | PASS | Exact-head MPR auto-merge review 5882782775 accepted by the bead's explicit existing-PR contract. |
| 2. Acceptance | PASS | Independent code inspection and all five required full-suite retry/recovery names PASS; all three new lost-close subcases PASS. |
| 3. Tests/build/vet | PASS | Complete 40-job full union; two non-owned conditions attributed below; all changed regressions PASS, no waiver. Build/vet/integration-vet actual exits 0. |
| 4. Open HIGH findings | PASS | Zero; MPR's shard placement follow-up is nonblocking and owned by MPR. |
| 5. Clean branch | PASS | Canonical checkout clean before/after the tests and policy (merge-status/final-status empty); source/evidence record staged separately on isolated branch. |
| 6. Clean divergence | PASS | Canonical merge-tree materialization exit0; exact tree checked after active hooks. Fresh origin fetch still resolves to the same captured base. |
| 7. One feature theme | PASS | Transient-once injection budget/diagnostics/regression proof; synchronized census baseline. Ancestry accepts only confirmed bead IDs; no unrelated source or .claude paths. |

## Review and acceptance

[Exact-head MPR comment 5882782775](https://github.com/gastownhall/gascity/pull/6379#issuecomment-5882782775) carries auto-merge after Qwen, Opus, and Codex review. The bead explicitly accepts this review in place of an internal review bead for the already-open PR. No HIGH finding is unresolved. MPR's optional recovery-shard follow-up belongs to MPR and does not change this gate's full-scope command.

Independent code inspection: `should_fail_transient_once` only checks the existing marker. `commit_transient_once` writes it after the bead is observed closed with fail outcome. A dropped close leaves the budget unspent for a later retry; the diagnostic records an uncommitted injection. The modified missing-attempt assertion dumps workflow state. The new three-case harness test and the existing full review/recovery scenarios exercise these outcomes. Census/documentation baseline +1 is intentional, present in all three required places, and not trimmed to excuse failure attribution.

## Full command and environment

`test_cmd_scope: full-suite`

`test_cmd`: `load-gate-run.sh --threshold 15 --max-wait 1800 -- isolated-test-run.sh -- make test-local-full-parallel`.

Both wrappers use absolute paths under `/home/jaword/projects/gc-management/packs/actual/all/scripts/`. `run.sh` records the complete command/environment. `LOCAL_TEST_JOBS=4`, `GOFLAGS=-v`, `GO_TEST_TIMEOUT=30m`, short private on-disk HOME, `TMPDIR=/var/tmp`, normal shared build cache, and documented hermetic Git config. Full union's own shard selectors are not deployer narrowing.

Rootless Podman and its socket were live before tests; `DOCKER_HOST=unix:///run/user/1000/podman/podman.sock`, `TESTCONTAINERS_RYUK_DISABLED=true`, `BEADS_ALLOW_UNREAPED_TESTCONTAINERS=1`. Cached `dolthub/dolt-sql-server:2.2.0` matches the linked test pin. Host cleanup sweep supplies reaping. Host bd remains 1.1.0; no live-store client upgrade or schema migration. An initial detached launch was terminated only by its own captured process group before any full-suite command started; its archived logs are excluded. The retained run uses setsid --wait.

All 40 jobs completed: 38 PASS, 2 FAIL (attributed). Raw make exit 2 and runner 123 remain recorded; no raw result is rewritten. Terminal executions: 97024 PASS,4 FAIL,331 SKIP; top-level executions: 54093 PASS,2 FAIL,236 SKIP. These include repeats across lanes, not unique names. Complete events.json and skip-ledger.tsv contain 331 skip rows, all with captured diagnostic premises. All required policy/native lanes completed successfully, actual exits 0.

```
load_threshold: 15
load_waited_seconds: 391
load_wait_timed_out: 0
load_start: 19.01
load_max: 55.23
load_mean: 30.66
samples: 141
read_errors: 0
```

`diff_tests_executed: yes` — two changed top-level tests plus three new subcases PASS; all three supplemental retry scenarios also PASS. No diff-owned FAIL or SKIP.

| Full-suite regression | Outcome | Elapsed | Log |
|---|---|---|---|
| `TestGraphDispatchTransientOnceBudgetSurvivesLostClose` | PASS | 0.03s | `shards/integration-rest-full-1-of-8.log:37` |
| `TestGraphDispatchTransientOnceBudgetSurvivesLostClose/budget_survives_a_close_that_never_landed` | PASS | 0.01s | `shards/integration-rest-full-1-of-8.log:38` |
| `TestGraphDispatchTransientOnceBudgetSurvivesLostClose/budget_is_spent_once_the_close_lands` | PASS | 0.01s | `shards/integration-rest-full-1-of-8.log:39` |
| `TestGraphDispatchTransientOnceBudgetSurvivesLostClose/unselected_ref_is_never_injected` | PASS | 0.00s | `shards/integration-rest-full-1-of-8.log:40` |
| `TestRetryManagedPooledWorkerRecoversClaimedAttemptAfterCrash` | PASS | 277.32s | `shards/integration-review-formulas-recovery.log:5` |
| `TestAdoptPRFormulaRetriesTransientReviewerStep` | PASS | 367.69s | `shards/integration-review-formulas-retries-1-of-2.log:5` |
| `TestReviewLoopApprovesInIterationTwoWithBodyRetry` | PASS | 572.09s | `shards/integration-review-formulas-retries-1-of-2.log:7` |
| `TestAdoptPRFormulaSoftFailsGeminiAfterTransientRetries` | PASS | 288.19s | `shards/integration-review-formulas-retries-2-of-2.log:5` |

Other skips are unit-lane process exclusions exercised in the union’s process/integration lanes, platform/root/SSH-only cases, helper entry points, unsupported provider/error-path contract rows, explicitly gated persistence/live inference/Postgres/k8s scenarios, and unavailable external pack/provider prerequisites. No changed regression skipped; no skipped test is claimed executed PASS. The complete logged-premise ledger retains individual justifications.

`waiver_ref: none`; gascity has no waiver path.

## Failure attribution

### Pool cleanup: ga-aik16g

`failure_attribution: TestBuildDesiredState_MinZeroDefaultScaleCheckRoutedWorkCreatesPoolSession -> ga-aik16g | clause 3: c — changed resourcecensus package is 0.0% covered; harness path excluded`.

Raw full-suite failure at `shards/cmd-gc-process-6-of-6.log:2428`, 12.64s: TempDir cleanup finds directory not empty. Bounded inspection records the surviving `001/.beads/eventsData/eventkit.lock` in `cleanup-leftover.json`. Tracker predates this run, was opened in full, and covers this exact condition. Sighting `39387b63-75e9-5cde-8caf-23b29fe7c5c7` was appended and read back verbatim.

The failing package imports resourcecensus; this is not a no-import claim. Independent isolated coverage on this run's exact canonical merge shows every changed resourcecensus function and its policy consumers at 0.0% (`cleanup-reachability.cover`, `cleanup-reachability-functions.log`). The failing test uses real bd, fake runtime and StartCommand=true; its path never executes graph-dispatch.sh. The added harness regression had not begun when the cleanup failed. The attribution-only test's PASS (23.47s) is not a replacement for the raw full-suite FAIL, which remains in the exit and counts.

Clauses 1 and 4 pass: no changed test or non-test path in cmd/gc. Declared census +1 is disclosed; this is a measured proof, not the inconclusive no-added-load escape. This deploy's own `gc.fixes_tracker=ga-b2gik5` stamp makes it fix-carrying. Blocking condition has designated fix `ga-zq8iwb`, stamped `gc.fixes_tracker=ga-aik16g`, closed/blocked on still-unlanded commit `271d711c80239ffbf2ab5b287a8cceda7ef92722`, resolved and not ancestor of captured main. Therefore the documented repeat exception applies; it does not excuse any changed regression.

### Legacy proxied backup response: ga-l49tcc

`failure_attribution: TestMaintenanceOrdersOnRealBdTopologies (parent and two proxied subcases) -> ga-l49tcc | clause 3: a — shell gc fixture does not execute changed harness/resourcecensus`.

Raw events in `shards/integration-packages-core-2-of-4.log:330-333`: parent FAIL 64.32s, proxied city/rigs FAIL 29.47s, mixed city/direct rig FAIL 14.74s. Host bd 1.1.0 returns unsupported proxied backup-status error-only JSON with schema_version 1; direct-server control PASS 20.12s. The condition tracker predates this run, was opened in full, and covers this exact response. Sighting `1f7d388f-a1f2-5043-9be9-e9b886952604` was appended and read back.

Independent import/path checks find no changed resourcecensus or graph-dispatch references in examples/gastown. Its fixture installs a shell gc router that directly execs bd and stubs session/mail; it never invokes the real gc or graph-dispatch worker. Clauses 1/4 pass, with no changed file in this failing package. The disclosed census +1 is not an inconclusive-path escape; definite mechanism proof settles clause 3. Fix-carrying repeat exception applies via this deploy's own stamp and the blocking condition's stamped unlanded HQ fix `gm-2z9uot`. Mayor chose a test-only bd 1.3 CLI pin; this run does not upgrade the host or migrate shared data.

## Required policy evidence

`policy_lane: PASS` — actual exit 0: `make test-ci-policy lint-affected fmt-check-changed GOLANGCI_LINT=/var/tmp/mpr-toolchains/golangci-lint-2.12.0/gopath/bin/golangci-lint LINT_CHANGED_SCOPE=tracked LINT_CHANGED_REF=ff1a334d604197ddc129a8bafc29547404196a2a` (`static-policy.log`). This is the documented conservative PR affected/reverse-dependent closure, with standalone vet, not the smaller pre-commit lint-changed target.

`native_policy_lane: PASS` — actual exit 0: `make check-gomod-replace check-eventexport-isolation check-core-boundary check-native-dependency-surface test-native-doltlite-beads check-docs` with verified Podman configuration (`native-policy.log`).

`go build ./...`, `go vet ./...`, `go vet -tags integration ./test/integration`: each actual exit 0. `make bazel-sync` actual exit 0 and following `git diff --exit-code`0; no generated BUILD drift (`bazel-sync.log`, `bazel-sync-diff.log`). Active materialization hook completed; make check-hooks verifies .githooks ownership. This gate record's staged commit also runs the active hook. No import/new-package/CI/API/schema/dashboard change calls for a separate codegen or dashboard lane.

## Completed job ledger

| Job | Actual result |
|---|---|
| `fsys-darwin-compile` | PASS |
| `unit-core` | PASS |
| `push-gate-lock-selftest` | PASS |
| `local-concurrency-selftest` | PASS |
| `cmd-gc-process-1-of-6` | PASS |
| `cmd-gc-process-2-of-6` | PASS |
| `cmd-gc-process-3-of-6` | PASS |
| `cmd-gc-process-4-of-6` | PASS |
| `cmd-gc-process-5-of-6` | PASS |
| `cmd-gc-process-6-of-6` | FAIL (attributed) |
| `productmetrics-testhook` | PASS |
| `integration-packages-core-1-of-4` | PASS |
| `integration-packages-core-2-of-4` | FAIL (attributed) |
| `integration-packages-core-3-of-4` | PASS |
| `integration-packages-core-4-of-4` | PASS |
| `integration-packages-cmd-gc-1-of-6` | PASS |
| `integration-packages-cmd-gc-2-of-6` | PASS |
| `integration-packages-cmd-gc-3-of-6` | PASS |
| `integration-packages-cmd-gc-4-of-6` | PASS |
| `integration-packages-cmd-gc-5-of-6` | PASS |
| `integration-packages-cmd-gc-6-of-6` | PASS |
| `integration-packages-runtime-tmux-1-of-3` | PASS |
| `integration-packages-runtime-tmux-2-of-3` | PASS |
| `integration-packages-runtime-tmux-3-of-3` | PASS |
| `integration-review-formulas-basic-1-of-2` | PASS |
| `integration-review-formulas-basic-2-of-2` | PASS |
| `integration-review-formulas-retries-1-of-2` | PASS |
| `integration-review-formulas-retries-2-of-2` | PASS |
| `integration-review-formulas-recovery` | PASS |
| `integration-bdstore` | PASS |
| `integration-rest-smoke-1-of-2` | PASS |
| `integration-rest-smoke-2-of-2` | PASS |
| `integration-rest-full-1-of-8` | PASS |
| `integration-rest-full-2-of-8` | PASS |
| `integration-rest-full-3-of-8` | PASS |
| `integration-rest-full-4-of-8` | PASS |
| `integration-rest-full-5-of-8` | PASS |
| `integration-rest-full-6-of-8` | PASS |
| `integration-rest-full-7-of-8` | PASS |
| `integration-rest-full-8-of-8` | PASS |

## Scope and publication

One feature: correct transient-once retry injection and its diagnostics/regression proof, plus synchronized resource baseline. No CI job/matrix/timeout/required-check change; criterion 3c has no new lane to await. Existing GitHub checks were independently observed with 69 SUCCESS, 31 SKIPPED, no failing or pending result. Conditional skips are not claimed as executed PASS.

Ancestry scope accepts only ga-5fd71q/ga-662krp/ga-njhh8e; no .claude additions or unrelated feature. Final publication rechecks safe isolated evidence branch, source presence, current PR head, clean tree, and active hooks.

Publish clearance only on the exact live source `0f982c49fad8668e7e26f6a3d5e2832d4b1567a7`, with a link to the separately committed evidence record. Verify status and peek-verify mayor merge-request before claiming either happened. MPR owns the merge; the evidence branch must never replace the existing PR head.
