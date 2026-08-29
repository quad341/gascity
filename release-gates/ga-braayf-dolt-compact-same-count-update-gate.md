# Release gate: Dolt compact same-count concurrent-UPDATE defer

- Deploy bead: `ga-braayf`
- Build bead: `ga-vyrswz`
- Review bead: `ga-402ydt`
- Original build/review chain: `ga-h0bj7x` / `ga-loyfg8`
- Reviewed source: `6ec9344a46eba6493e453f0a77b3efc804c30161`
- Provenance branch: `builder/ga-vyrswz-samecount-rebase`
- Evidence branch: `deploy/ga-braayf-gate`
- Gate base: `origin/main@430af939081a010adc0af7aa216a9fc1d298fff8`
- Merge base: `655713ff4559c335ff17ec09c4a65f5cca2a5f27`
- Evaluation date: 2026-08-28
- Disposition: **FAIL — RETURN TO BUILDER, NO RELEASE PR**

## Gate checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | Review bead `ga-402ydt` is closed with `verdict: pass` and records `deploy_commit: 6ec9344a46eba6493e453f0a77b3efc804c30161`, exactly matching the commit evaluated here. |
| 2 | Acceptance criteria met | **PASS** | The same-count drift table accumulator is reset at both required sites and populated by `verify_counts`; the fourth defer path contains all eight fail-closed conditions and reuses `gain_drift_is_additive_only`; the stale category comment is corrected. Both added tests and both required negative controls executed and passed in the full suite. |
| 3 | Tests pass | **FAIL** | The documented 40-job full-suite command completed 35 PASS / 5 FAIL. It produced 46,467 top-level PASS / 5 FAIL / 189 SKIP results (77,341 PASS / 22 FAIL / 277 SKIP including subtests). `TestCompactScriptRealDoltRemotePush` timed out during the remote push after successful local compaction. Although the signature predates this run (`ga-ok3q3c`, `ga-4q1evf`), the test executes the changed compact script, so the mandatory no-path-overlap requirement for attribution is not met. Separately, `TestCleanInstallTutorialPath` hit a new post-step-6 two-minute timeout with no predating exact tracker; it is now `ga-thcffg`, which cannot retroactively waive this run. |
| 3a | Pre-existing failures may be attributed | **PARTIAL / DOES NOT CURE FAIL** | `TestSessionEventsLive` maps to `ga-at7jv0`; `TestBdFlagManifestCurrent` maps to `ga-f0uceo`; and `TestHumaBinary_CityCreateAsync` reproduced the beads#4566 dirty-table signature tracked by `ga-lpfjhc`. Those failures are outside the diff with decisive mechanisms. The compact remote-push timeout and new tutorial timeout remain unwaived for the reasons above. |
| 3b | Policy/lint lane | **PASS** | `make test-ci-policy fmt-check check-gomod-replace check-native-dependency-surface check-eventexport-isolation check-core-boundary check-docs`, `go vet ./...`, isolated-cache `make lint`, `sh -n`, and baseline-vs-head `shellcheck --severity=warning` all passed or showed only the same three predating shellcheck warnings at both endpoints. |
| 4 | No high-severity review findings open | **PASS** | The exact-SHA reviewer verdict records no style, security, or specification finding and closes with PASS. |
| 5 | Final branch is clean | **PASS** | The exact candidate was evaluated in an isolated worktree. Before this record was added, `git status --short --branch` showed only `## deploy/ga-braayf-gate`; generated `bin/gc` is ignored. Clean state is rechecked after committing this record. |
| 6 | Branch diverges cleanly from main | **PASS** | `git merge-tree --write-tree origin/main 6ec9344a46eba6493e453f0a77b3efc804c30161` exited 0 and produced tree `3bed8f61b5f4eac619858f1dd296eb2839813ab6`. `git diff --check` and `assert_deploy_ancestry_scope` passed for the deploy, review, prior deploy, prior review, and build bead chain. No self-rebase was required. |
| 7 | Single feature theme | **PASS** | The four-commit range changes only `examples/bd/dolt/commands/compact/run.sh` and `examples/bd/dolt/dog_exec_scripts_test.go`; all commits implement, guard, or test the same compact same-count writer-race defer behavior. |

## Acceptance evidence

The production path now:

- initializes `verify_counts_same_count_drift_tables` in both `verify_counts`
  and `flatten_database`;
- appends every same-count value-hash-drifted table;
- requires a HEAD-proven writer race, the same-count category, absence of gain,
  gain+drift, row decrease, table-list change, and probe failure, plus an
  additive-only `DOLT_DIFF` proof before deferring; and
- falls through to quarantine when that proof fails.

The full-suite logs contain named PASS results for:

- `TestCompactScriptDefersProvenWriterRaceSameCountHashDrift`
- `TestCompactScriptQuarantinesSameCountDriftWhenDiffProofFails`
- `TestCompactScriptQuarantinesSameRowCountWriterBeforeFullGC`
- `TestCompactScriptQuarantinesMixedSignalsDespiteWriterRace`
- `TestCompactScriptDefersWhenWriterCommitsCausingSameCountHashDrift`

The two diff-owned tests each ran twice, once in `unit-core` and once in
`integration-packages-core-1-of-4`, and passed every time. Therefore:

```text
diff_tests_executed:
  TestCompactScriptDefersProvenWriterRaceSameCountHashDrift PASS
  TestCompactScriptQuarantinesSameCountDriftWhenDiffProofFails PASS
waiver_ref: none
```

## Test evidence

Build and smoke passed:

```text
make build
./bin/gc version --long
./bin/gc --help
```

Policy and static checks passed:

```text
make test-ci-policy fmt-check check-gomod-replace \
  check-native-dependency-surface check-eventexport-isolation \
  check-core-boundary check-docs
go vet ./...
GOLANGCI_LINT_CACHE=<isolated-disk-path> make lint
sh -n examples/bd/dolt/commands/compact/run.sh
```

The head and merge-base each reported the same three shellcheck warnings:
`SC1083`, `SC2254`, and `SC2034`; only the last warning's line number shifted.
No new shellcheck finding was introduced.

The required full-scope command was:

```text
DOCKER_HOST=unix:///run/user/1000/podman/podman.sock \
TESTCONTAINERS_RYUK_DISABLED=true \
PUSH_GATE_MAX_CONCURRENT=1 PUSH_GATE_MAX_WAIT_SECONDS=600 \
LOCAL_TEST_JOBS=4 GOFLAGS=-v GO_TEST_TIMEOUT=30m \
LOCAL_TEST_LOG_DIR=/var/tmp/ga-braayf-gate.7zPen2/jobs \
make test-local-full-parallel
```

`test_cmd_scope: full-suite`.

The rootless Podman socket was reachable before the run. Nonzero skips are
pre-existing platform-specific, opt-in live-service/persistence, unsupported
OS, golden-regeneration, or helper-process cases. Neither diff-owned test
skipped.

The repository pre-push hook subsequently ran `make test-fast-parallel` while
publishing this FAIL evidence. Eight of ten jobs passed; `unit-core` failed on
two out-of-diff tests: transcript streaming omitted a turn across consecutive
compaction boundaries (`ga-k87o1z`, filed after this run), and the known
`TestCustomTypesCheck_TableDrift` TempDir cleanup race recurred (`ga-6pnurv`).
This later run does not alter the full-suite counts above; it independently
confirms that criterion 3 remains red.

Raw logs:

- wrapper: `/var/tmp/ga-braayf-gate.7zPen2/full-suite.log`
- per-job: `/var/tmp/ga-braayf-gate.7zPen2/jobs/`
- policy/static: `/var/tmp/ga-braayf-gate.7zPen2/{policy,vet,lint}.log`

## Failure disposition

| Failing test | Tracker | Attribution decision |
|---|---|---|
| `TestCompactScriptRealDoltRemotePush` | `ga-ok3q3c`, `ga-4q1evf` | **Unwaived.** The exact `remote push failed rc=124 after local compaction` signature is old, but this test executes the changed `run.sh`. The no-path-overlap clause fails, and the candidate adds test load. |
| `TestSessionEventsLive` | `ga-at7jv0` / upstream gascity#5653 | Attributed. Exact `getAgent evt-a: ok=false err=<nil>` signature; `internal/runtime/herdr` is outside and unreachable from the two changed Dolt paths. |
| `TestBdFlagManifestCurrent` | `ga-f0uceo` | Attributed. The installed `bd` exposes flags absent from `internal/bdflags`; neither the binary nor manifest is changed or reachable from this diff. |
| `TestHumaBinary_CityCreateAsync` | `ga-lpfjhc` / beads#4566 | Attributed. Exact dirty-table schema-migration failure in throwaway city initialization; no path or mechanism overlap. |
| `TestCleanInstallTutorialPath` | `ga-thcffg` | **Unwaived.** `gc init` stopped after step 6 and timed out after two minutes. Existing trackers cover different signatures; `ga-thcffg` was necessarily filed after this run and cannot satisfy the predating-tracker rule. |
| `TestStreamSessionTranscriptHistoryDoesNotSkipTurnsAcrossCompactionBoundaries` (later pre-push fast suite) | `ga-k87o1z` | Out of diff, but the tracker was filed after the run. This is additional unwaived failure evidence, not part of the full-suite counts. |
| `TestCustomTypesCheck_TableDrift` (later pre-push fast suite) | `ga-6pnurv` | Known out-of-diff TempDir cleanup race. This is additional failure evidence, not part of the full-suite counts. |

The run was not interrupted by disk exhaustion. Before it began, eleven
interrupted-run `/var/tmp/gotmp/go-build*` directories older than 30 minutes
were verified `lsof`-clean and removed, restoring headroom from 6.1 GiB to
7.8 GiB. The full suite then completed all 40 jobs, and headroom recovered to
6.1 GiB after normal cleanup. This temporary recovery is recorded on
`ga-vtthm5`; its structural leak-prevention work remains open.

## Decision

Gate FAIL on criterion 3. Do not open a release PR and do not publish deploy
clearance. Preserve this isolated evidence branch for the repository's
mandatory push/audit rule, route `ga-braayf` back to builder as
`ready-to-build`, and carry `ga-ok3q3c` / `ga-4q1evf` plus `ga-thcffg` as the
blocking failure context for the next exact-SHA review cycle.
