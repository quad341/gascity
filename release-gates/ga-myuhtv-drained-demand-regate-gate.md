# Release gate: drained named-session routed demand, PR #6692

**Verdict:** **PASS**

Evaluated 2026-09-29T03:25:06.699904+00:00. Deploy bead `ga-myuhtv`; original deploy `ga-6t11ku`; build `ga-j4lqwa.1`; review `ga-lklarp`.

This is a fresh re-gate of [our existing PR #6692](https://github.com/gastownhall/gascity/pull/6692), after its MPR corrections. It preserves that PR's head. The evidence branch adds only this gate record; it is not a replacement PR or a request to merge the evidence branch.

## Coordinates

- `deploy_mode`: remote; push remote: fork (`quad341/gascity`); base repo: `gastownhall/gascity`.
- Reviewed/current PR head: `57b66bf252c95dd99c26a8bb338f2831648fb0ac` (`deploy/ga-6t11ku-gate`). Resolved against the object store and checked against the live PR.
- Pinned `origin/main`: `58022309bdb11d24930ca545e7aeca8d0208b184`.
- Materialized two-parent merge: `5982ac32a72316f1074336f68f63488329876b4d`.
- Merge tree: `64a267439919d88d2b783837072636f5243fdf73`; exactly equals `git merge-tree --write-tree` for the pinned head/base.
- Logs and runners: `/var/tmp/ga-myuhtv-gate.Lh4Fxr`. Temporary test worktrees are removed by their EXIT traps; the evidence directory remains.
- `docs/PROJECT_MANIFEST.md` is absent. Release criteria come from the supplied deployer protocol, with full scope from `TESTING.md` and the Makefile.

## Criteria

| # | Result | Evidence |
|---|---|---|
| 1. Review PASS | PASS | Original reviewer PASS plus MPR's exact-head `auto-merge` verdict in [comment 5857838848](https://github.com/gastownhall/gascity/pull/6692#issuecomment-5857838848). No patch carryover substitution. |
| 2. Acceptance criteria | PASS | Routed demand wakes a drained named session, including the blocked-named/live-routed combination; ready assigned work wakes through assigned-work; blocked named demand and work-query preserve the drain guard. All five full-suite regression results below PASS. |
| 3. Full tests, build, vet | PASS | Completed full 40-job union; build/vet exit 0. Two independent, non-owned failure conditions are attributed under the supplied four-clause protocol below. Raw exit/counts are retained. No waiver. |
| 4. Open high-severity findings | PASS | Zero unresolved HIGH findings; exact-head MPR verdict has no new concerns. MPR owns the nonblocking scale-check readiness follow-up. |
| 5. Clean final branch | PASS | Materialized merge checkout was clean before and after tests/policy (`merge-status`, `final-status` empty). Source worktree was clean before creating this record; record is committed separately on an isolated evidence branch. |
| 6. Clean merge with main | PASS | Mandated `materialize_merge_tree` exit 0 and exact tree equality at the pinned coordinates. No self-rebase, source push, force-push, or merge. |
| 7. One feature theme | PASS | Wake/demand handling for drained named sessions only: `cmd/gc/compute_awake_set.go`, its existing test file, and the original gate record. All non-merge commits cite confirmed build/deploy bead IDs; ancestry guard exit 0, no `.claude/**` additions. |

## Full-scope test evidence

`test_cmd_scope: full-suite`

`test_cmd`: `load-gate-run.sh --threshold 15 --max-wait 1800 -- isolated-test-run.sh -- make test-local-full-parallel`

The two wrappers are the absolute paths under `/home/jaword/projects/gc-management/packs/actual/all/scripts/`. `run.sh` records the complete environment and command. The repo's documented union expands its own shard filters; no deployer-created test/package filter narrowed this run.

Environment: `LOCAL_TEST_JOBS=4`, `GOFLAGS=-v`, `GO_TEST_TIMEOUT=30m`, short on-disk `TMPDIR=/var/tmp`, private test HOME, hermetic Git config from the documented runners. Rootless Podman socket was active before tests; `DOCKER_HOST=unix:///run/user/1000/podman/podman.sock`, `TESTCONTAINERS_RYUK_DISABLED=true`, `BEADS_ALLOW_UNREAPED_TESTCONTAINERS=1`. Cached `dolthub/dolt-sql-server:2.2.0` matches the linked test code pin; host sweep provides cleanup. No Go cache purge or live-store migration.

```
LOAD_GATE_SUMMARY threshold=15 waited_seconds=541 wait_timed_out=0 load_start=16.89 load_max=29.70 load_mean=21.13 samples=103 read_errors=0
load_threshold: 15
load_waited_seconds: 541
load_wait_timed_out: 0
load_start: 16.89
load_max: 29.70
load_mean: 21.13
```

All 40 jobs completed: 38 PASS, 2 FAIL (attributed). The full make command's actual exit is 2 (runner error propagated through make); attribution makes criterion 3 PASS under the supplied protocol, without rewriting that exit.

| Counting unit | PASS | FAIL | SKIP |
|---|---:|---:|---:|
| All terminal test/subtest executions | 96872 | 4 | 331 |
| Top-level terminal executions | 54028 | 2 | 236 |

Counts come from per-shard terminal output, excluding non-test package result lines and avoiding JSON/plain duplicate parsing. They include repeated executions across the unit, process and integration lanes; they are not unique test counts. Build/compiler/selftest jobs contribute job results rather than invented test counts. Parser: `summarize.py`; complete event ledger: `events.json`.

`diff_tests_executed: yes` — five unique changed regressions, ten full-suite PASS executions, zero FAIL, zero SKIP.

`waiver_ref: none` — Gas City has no waiver path; these are supported attributions, not waivers.

| Diff-owned regression | Result | Full-suite source |
|---|---|---|
| `TestNamedOnDemand_RoutedDemandWakesDrainedSession` | PASS | `shards/cmd-gc-process-5-of-6.log:4595`; `shards/integration-packages-cmd-gc-2-of-6.log:4510` |
| `TestNamedOnDemand_DrainedSessionWithReadyAssignedWorkWakes` | PASS | `shards/cmd-gc-process-6-of-6.log:4597`; `shards/integration-packages-cmd-gc-3-of-6.log:4539` |
| `TestNamedOnDemand_NamedDemandDoesNotWakeDrainedSessionWithBlockedWork` | PASS | `shards/cmd-gc-process-1-of-6.log:4575`; `shards/integration-packages-cmd-gc-4-of-6.log:4542` |
| `TestNamedOnDemand_RoutedDemandWakesDrainedSessionDespiteBlockedNamedDemand` | PASS | `shards/cmd-gc-process-2-of-6.log:4552`; `shards/integration-packages-cmd-gc-5-of-6.log:4538` |
| `TestNamedOnDemand_WorkQueryDoesNotWakeDrainedSession` | PASS | `shards/cmd-gc-process-3-of-6.log:4327`; `shards/integration-packages-cmd-gc-6-of-6.log:4459` |

`skip_justification`: no changed regression skipped. Unit-only process exclusions are paired with the documented process/integration lanes in this same union. Other logged skips cover Darwin/root/SSH-only behavior, subprocess helper entry points, optional live catalog/MCP/tmux/provider setup, intentionally tested provider-error skip handling, unsupported backend/provider contract rows, fixed characterization/golden-update tests, safety-refused ambient city discovery, and explicit missing external pack/formula assets or upstream capability gates. All skips have captured test diagnostics in `skip-ledger.tsv`; those raw premises and `events.json` remain with the logs. A skip is not claimed as an executed PASS, and missing environment is not used to excuse a changed regression.

## Failure attribution

### Herdr socket path — pre-run tracker ga-0og0ry

`failure_attribution: TestSessionEventPumpLiveHerdr -> ga-0og0ry | clause 3: c — ComputeAwakeSet 0.0% coverage`

Full-suite failure: `shards/cmd-gc-process-3-of-6.log:15244`, ConfigureServer did not become ready within 10.06s. Tracker was opened and predates this run (2026-09-25); it names this test and the long-HOME Unix socket limit. The private HOME used here exceeds the documented 30-character limit once the hermetic runner drops XDG_CONFIG_HOME. Fix `ga-yxizvd` remains unlanded; sighting/proof were appended and read back.

Measured proof at this run's exact merge: isolated `go test -count=1 -run '^TestSessionEventPumpLiveHerdr$' -coverprofile=... ./cmd/gc` with `GC_FAST_UNIT=0`; test really executed (PASS, 16.69s), and `go tool cover -func` reports the sole changed production function, ComputeAwakeSet, at 0.0%. Files: `herdr-coverage.log`, `herdr-coverage.out`, `herdr-coverage-functions.txt`. The diagnostic inherited XDG_CONFIG_HOME, unlike the full hermetic lane, and is not represented as a base reproduction or a replacement full-suite PASS.

`clause-4-guard: same_package=yes proof=c added_test_load=no` — failing test file unchanged; no census bump, new test target, or new test file in cmd/gc. Five pure cases were added to the existing wake-set test file. This is the first occurrence in this bead's fresh gate; the earlier same-PR re-gate had different prefix fixture failures. No repeat/fix-carrying escape is claimed.

### Legacy proxied backup-status response — tracker ga-l49tcc

`failure_attribution: TestMaintenanceOrdersOnRealBdTopologies (root and two proxied subcases) -> ga-l49tcc | clause 3: d — identical exact-base failure`

Full-suite failure: `shards/integration-packages-core-2-of-4.log:309-334`, subcases `proxied_city_and_proxied_rigs` and `mixed:_proxied_city_with_a_direct-server_rig`. Host bd 1.1.0 returns error-only JSON with schema_version 1 for unsupported proxied backup status; fixture recognizes only code `proxy.backup.unsupported`. Direct-server control PASSes. Fixture documents CI's bd 1.3.0 pin; this run used the unchanged host default.

Searched existing open/in-progress test/condition trackers before creating one condition tracker. `ga-l49tcc` was created during this run, not before it. The same-round timing escape (gm-sf3238) is supported by an independent reproduction in this run at exact base `58022309bdb11d24930ca545e7aeca8d0208b184`, without this PR's diff:

```
isolated-test-run.sh -- go test -tags=integration -count=1 \
  -run '^TestMaintenanceOrdersOnRealBdTopologies/proxied_city_and_proxied_rigs$' ./examples/gastown
```

Actual exit 1; subtest FAIL 30.27s (package 36.988s), identical unsupported JSON for city and both rigs. `base-maintenance.sh`, `base-maintenance.log` and `base-maintenance-master.log` record the checkout/command/result. This reproduces the condition (CLI version/error shape), which does not require contention. The narrow diagnostic is attribution evidence only.

Clauses 1/4 clear: unchanged failing test file/package, no census/target/new-file load. The fixture's fake shell gc router executes bd directly and stubs session/mail operations; it does not execute real gc or ComputeAwakeSet. Proof(d) landed and the gm-sf3238 timing escape qualifies this discovering round. All three raw failing terminal events remain in the counts. Tracker stays open until the chosen CLI pin/compatibility correction lands and is verified; mayor has the reproduction and selects follow-up ownership. No host client upgrade or schema migration was attempted.

## Policy and CI

`policy_lane: PASS` — `make test-ci-policy lint-affected fmt-check-changed GOLANGCI_LINT=/var/tmp/mpr-toolchains/golangci-lint-2.12.0/gopath/bin/golangci-lint LINT_CHANGED_SCOPE=tracked LINT_CHANGED_REF=58022309bdb11d24930ca545e7aeca8d0208b184` (actual exit 0; `static-policy.log`).

`native_policy_lane: PASS` — `make check-gomod-replace check-eventexport-isolation check-core-boundary check-native-dependency-surface test-native-doltlite-beads check-docs` with the verified Podman environment (actual exit 0; `native-policy.log`).

`go build ./...`: PASS, actual exit 0 (`build.log`). `go vet ./...`: PASS, actual exit 0 (`vet.log`). The merge materialization's active pre-commit also ran pinned lint, codegen, vet and docsync (`materialize.log`). `.githooks` ownership was verified with `make check-hooks`; it is checked again for this record's staged commit.

`ci_lane_run`: [36318971464](https://github.com/gastownhall/gascity/actions/runs/36318971464), completed SUCCESS at the exact PR head. Required aggregate, static/generated/acceptance A, process, productmetrics, beads topology/proxied and worker Phase 2 lanes are green. Conditional CI skips are not called PASS. Optional [Bazel side-by-side](https://github.com/gastownhall/gascity/actions/runs/36318971510/job/108618980734) remains FAILURE from a runner sandbox missing `/tmp/bt` before project tests; exact-head MPR explicitly classified it nonblocking and owns its rerun. This PR changes no CI config, dependency/import/API/dashboard/schema, or Bazel package definition.

## Completed job ledger

| Job | Actual result |
|---|---|
| `fsys-darwin-compile` | PASS |
| `unit-core` | PASS |
| `push-gate-lock-selftest` | PASS |
| `local-concurrency-selftest` | PASS |
| `cmd-gc-process-1-of-6` | PASS |
| `cmd-gc-process-2-of-6` | PASS |
| `cmd-gc-process-3-of-6` | FAIL (attributed) |
| `cmd-gc-process-4-of-6` | PASS |
| `cmd-gc-process-5-of-6` | PASS |
| `cmd-gc-process-6-of-6` | PASS |
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

## Handoff

Publish `release-gate/deploy-clearance=success` only on `57b66bf252c95dd99c26a8bb338f2831648fb0ac` after verifying the PR still has that head. Record and verify the exact status and mayor merge-request separately in the bead; this file does not claim those future side effects already happened. Merge authority is mpr via mayor; this deployer performs no merge. The gate evidence branch is separate and must not be substituted for PR #6692's head.
