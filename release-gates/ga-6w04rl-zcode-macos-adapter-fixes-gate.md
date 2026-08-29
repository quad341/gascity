# Release gate: macOS zcode adapter fixes

- Deploy bead: `ga-6w04rl`
- Build bead: `ga-uqdim3`
- Review bead: `ga-afpjnw`
- Reviewed source: `352696426696487398349190f17a8f23e8c071d0`
- Provenance branch: `builder/ga-uqdim3`
- Isolated deploy branch: `deploy/ga-6w04rl-gate`
- Full-suite base: `origin/main@157858d9ee8bd6ab85e4a0d2128f34dc2e166a7f`
- Final criterion-6 base: `origin/main@7cf9c8b3fb0bb03dac7cc89683a5f1883a641c6c`
- Merge base: `430af939081a010adc0af7aa216a9fc1d298fff8`
- Re-evaluated: 2026-08-29; final staleness check: 2026-08-30
- Disposition: **FAIL — ROUTE BACK TO BUILDER, NO RELEASE PR**

## Gate checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | Review bead `ga-afpjnw` records PASS for exact commit `352696426696487398349190f17a8f23e8c071d0`; the provenance branch still resolves to that SHA. |
| 2 | Acceptance criteria met | **PASS** | The zcode drain guard uses `${more-}` without weakening `set -u`; the harness resolves its root once with `filepath.EvalSymlinks`; the subprocess resource census and generated ledger agree; all four acceptance tests named below passed without skips; and the handoff correctly reserves macOS confirmation for the PR's Mac Regression lane. |
| 3 | Tests pass | **PASS WITH ATTRIBUTED FAILURES** | The documented full-scope command completed all 40 jobs: 35 PASS and 5 raw FAIL. All five failures are non-diff-owned, have exact predating trackers, have cross-run/base or mechanism proof, and have no path overlap. Both changed packages passed in the full run; a supplemental exact-name run recorded 4 PASS, 0 FAIL, 0 SKIP. |
| 3a | Pre-existing failures may be attributed | **PASS** | Per-failure attribution below satisfies all four clauses of the release-gate convention. No inconclusive attribution path is used. |
| 3b | Policy/lint lane | **PASS WITH MAYOR WAIVER** | `make test-ci-policy`, clean-worktree isolated-cache `make lint`, formatting, module policy, event-export isolation, core boundary, docs synchronization, and `go vet ./...` passed. The native-size guard remains red on candidate and current main; `gm-wisp-ytuws9` is the mayor's criterion-3b waiver for this exact SHA. |
| 4 | No high-severity review findings open | **PASS** | The exact-SHA review records no style, security, specification, or coverage blocker. |
| 5 | Final branch is clean | **PASS** | The exact candidate was tested from a clean detached checkout. The isolated deploy branch contains the reviewed commits plus this gate record only; `git diff --check` passed. |
| 6 | Branch diverges cleanly from main | **FAIL** | The earlier check against `origin/main@157858d9` passed, but a mandatory final refresh to `origin/main@7cf9c8b3` found content conflicts in `TESTING.md`, `internal/testpolicy/resourcecensus/census.go`, and `test/test-resources.toml`. The formula's bounded self-rebase returned `rc=12`, aborted, and left `builder/ga-uqdim3` clean and unchanged at the reviewed SHA. |
| 7 | Single feature theme | **PASS** | The three-commit range contains the RED test, GREEN macOS zcode fixes, and required resource-census bookkeeping. All six changed files serve one adapter-fix theme. |

## Full-suite evidence

```text
test_cmd_scope: full-suite
test_cmd: DOCKER_HOST=unix:///run/user/1000/podman/podman.sock TESTCONTAINERS_RYUK_DISABLED=true make test-local-full-parallel
test_counts: 35/40 jobs PASS, 5/40 jobs raw FAIL, 0 skipped jobs
full_log: /var/tmp/ga-6w04rl-rerun-full.log
job_logs: /var/tmp/gc-local-tests.KL6Zhh
```

The runner does not emit an exhaustive test-event PASS/SKIP count, so none is
inferred from its job summary. It did run both changed packages twice: the
`unit-core` and `integration-packages-core-4-of-4` logs each report PASS for
`internal/worker/adapters/zcode` and
`internal/testpolicy/resourcecensus`.

Supplemental exact-name execution on the same reviewed SHA recorded **4 PASS,
0 FAIL, 0 SKIP**:

```text
diff_tests_executed:
  TestZcodeReplDrainGuardToleratesUnsetMore PASS
  TestResolveSymlinksFollowsSymlinkedRoot PASS
  TestInterruptMidTurnContinuesTheLoop PASS
  TestRepositoryLedgerMatchesCensusAndDocumentation PASS
diagnostic_log: /var/tmp/ga-6w04rl-diff-tests.json
```

### Raw failure attribution

| Raw failing test | Predating tracker | Clause-3a proof |
|---|---|---|
| `TestBdFlagManifestCurrent` | `ga-f0uceo` | Exact installed-bd manifest-skew signature with many unrelated-candidate and exact-base reproductions. The candidate neither touches `internal/bdflags` nor controls the installed `bd` binary. |
| `TestSessionEventsLive` | `ga-idsv6m` / `ga-at7jv0` | Exact `getAgent evt-a: ok=false err=nil` signature seen on unrelated SHAs. The candidate has no herdr path overlap; its sole production change is the zcode shell adapter. |
| `TestAdoptPRFormulaCompileAndRun` | `ga-lpfjhc` | Exact `gastownhall/beads#4566` pending dirty-table migration signature (`issues`) during fixture `gc init`, before formula or zcode execution. |
| `TestRetryManagedPooledWorkerRecoversClaimedAttemptAfterCrash` | `ga-lpfjhc` | Same exact beads#4566 fixture-bootstrap signature (`issue_snapshots`), before the recovery path executes. |
| `TestE2E_SuspendResume_City` | `ga-yc0e3a` | Exact missing-`citysus.report` timeout; the tracker contains exact-base failure and repeated unrelated-candidate evidence. The test does not invoke the changed zcode adapter. |

All five trackers predate this run. None of the failing tests or packages is in
the candidate's six-file diff. The proofs above are mechanism, cross-PR, or
base-reproduction proofs, so no inconclusive-guard exception is being used.

## Policy and build evidence

Passed on the exact reviewed SHA:

```text
make test-ci-policy
go vet ./...
GOLANGCI_LINT_CACHE=<fresh on-disk cache> make lint       0 issues
make fmt-check check-gomod-replace
make check-eventexport-isolation check-core-boundary check-docs
make build
./bin/gc version --long                                  commit: 3526964266
./bin/gc --help
sh -n internal/worker/adapters/zcode/zcode-repl
git diff --check origin/main...3526964266
```

The reused deployer worktree initially exposed three lint findings from its
ignored dashboard `node_modules`. A clean detached worktree at the exact SHA,
with a fresh on-disk golangci cache, reported `0 issues`; only that clean source
scan is used as candidate evidence.

The native dependency guard remains red:

```text
candidate 3526964266: 270,646,056 bytes > 270,000,000
origin/main 157858d9: 270,659,888 bytes > 270,000,000
candidate delta versus main: -13,832 bytes
tracker: ga-iuznq2 (P1; predates this re-run)
```

The candidate's production delta is a shell script; its Go changes are tests
and the test-policy census, which is not in `go list -deps ./cmd/gc`. Current
main reproduces the same guard failure and produces the larger binary.

`waiver_ref`: `gm-wisp-ytuws9` (mayor, 2026-08-29), scoped to criterion 3b at
exact reviewed SHA `352696426696487398349190f17a8f23e8c071d0`. The waiver does
not apply to another SHA. P1 `ga-iuznq2` owns restoring main below the cap.

## Final criterion-6 refresh

After the full gate had passed against `origin/main@157858d9`, main advanced.
The required pre-push refresh produced a conflicted merge tree:

```text
git merge-tree --write-tree --messages \
  origin/main \
  352696426696487398349190f17a8f23e8c071d0

CONFLICT TESTING.md
CONFLICT internal/testpolicy/resourcecensus/census.go
CONFLICT test/test-resources.toml
```

The formula-mandated bounded self-rebase was run in an isolated disposable
worktree on `builder/ga-uqdim3`. It returned `rc=12`, aborted the rebase, and
left the branch clean at `352696426696487398349190f17a8f23e8c071d0`. Nothing
was pushed to the builder branch.

## Decision

Gate FAIL on criterion 6. Do not open a release PR and do not publish deploy
clearance. Route `ga-6w04rl` back to builder for a conflict-resolved rebase and
fresh review. Mayor waiver `gm-wisp-ytuws9` is scoped to the current reviewed
SHA and does not carry to a rebased SHA.
