# Release gate: macOS zcode adapter fixes

- Deploy bead: `ga-6w04rl`
- Build bead: `ga-uqdim3`
- Review bead: `ga-afpjnw`
- Reviewed source: `352696426696487398349190f17a8f23e8c071d0`
- Provenance branch: `builder/ga-uqdim3`
- Evidence branch: `deploy/ga-6w04rl-gate`
- Gate base: `origin/main@7f622528d988905f5a5c3721c040373a8a073250`
- Merge base: `430af939081a010adc0af7aa216a9fc1d298fff8`
- Evaluation date: 2026-08-28
- Disposition: **FAIL — RETURN TO BUILDER, NO RELEASE PR**

## Gate checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | Review bead `ga-afpjnw` records `REVIEW VERDICT: PASS` for exact commit `352696426696487398349190f17a8f23e8c071d0`; the provenance branch tip resolves to the same SHA. |
| 2 | Acceptance criteria met | **PASS** | The zcode drain guard uses `${more-}` without weakening `set -u`; the harness resolves its root once with `filepath.EvalSymlinks`; `TestInterruptMidTurnContinuesTheLoop` passes; and the handoff explicitly identifies Mac Regression CI as still required rather than claiming Linux as macOS verification. |
| 3 | Tests pass | **FAIL** | The mandatory policy lane failed before the expensive full-suite phase: `make check-native-dependency-surface` reports a 270,612,512-byte `gc` binary against a 270,000,000-byte ceiling. The full 40-job suite was deliberately not launched after this fail-fast result, so the command scope is focused and criterion 3 cannot pass. |
| 3a | Pre-existing failures may be attributed | **DOES NOT CURE FAIL** | Current `origin/main@7f622528d9` independently reproduces the same check at 270,633,240 bytes, 20,728 bytes larger than the candidate. This proves the candidate did not cause the overage. No tracker predated the first gate run; `ga-iuznq2` was filed from this evidence and therefore cannot retroactively waive it. |
| 3b | Policy/lint lane | **FAIL** | `test-ci-policy`, formatting, go.mod replacement, event-export isolation, core-boundary, docs, `go vet ./...`, and isolated-cache `make lint` passed. The native-dependency size guard failed as described above. |
| 4 | No high-severity review findings open | **PASS** | The exact-SHA review records no style, security, specification, or coverage blocker and closes its findings with PASS. |
| 5 | Final branch is clean | **PASS** | The candidate was evaluated in an isolated worktree; only this gate record is added on the evidence branch. |
| 6 | Branch diverges cleanly from main | **PASS** | `git merge-tree --write-tree origin/main 352696426696487398349190f17a8f23e8c071d0` exited 0 and produced tree `3243a34f208cdad951dbba549276c1e4c1de99a8`; `git diff --check` and ancestry-scope validation also passed. |
| 7 | Single feature theme | **PASS** | The three-commit range contains the RED test, GREEN macOS zcode fixes, and the resource-census bookkeeping required by the new subprocess-owning regression. All six changed files serve that one adapter-fix theme. |

## Acceptance evidence

Independent inspection confirmed both functional changes:

- the timeout/EOF drain path no longer dereferences an unset `more` variable
  under `set -u` on bash 3.2;
- `newHarness` resolves the temporary root before deriving `workDir`, `home`,
  and mirror paths, eliminating `/var` versus `/private/var` comparisons; and
- the subprocess resource census, TOML ledger, and generated `TESTING.md`
  ledger agree on the single new call site and Medium owner.

Focused execution on the reviewed SHA:

```text
go test ./internal/worker/adapters/zcode/...        36 PASS / 0 FAIL / 0 SKIP
go test ./internal/testpolicy/resourcecensus/...    81 PASS / 0 FAIL / 0 SKIP

TestZcodeReplDrainGuardToleratesUnsetMore            PASS
TestResolveSymlinksFollowsSymlinkedRoot              PASS
TestInterruptMidTurnContinuesTheLoop                 PASS
TestRepositoryLedgerMatchesCensusAndDocumentation   PASS
```

```text
diff_tests_executed:
  TestZcodeReplDrainGuardToleratesUnsetMore PASS
  TestResolveSymlinksFollowsSymlinkedRoot PASS
test_cmd_scope: focused
waiver_ref: none
```

The macOS behavior still needs the PR's Mac Regression lane; no local macOS
claim is made.

## Test and policy evidence

Passed:

```text
make build
./bin/gc version --long
./bin/gc --help
go vet ./...
GOLANGCI_LINT_CACHE=<isolated-disk-path> make lint
make test-ci-policy fmt-check check-gomod-replace
make check-eventexport-isolation check-core-boundary check-docs
sh -n internal/worker/adapters/zcode/zcode-repl
make test-fast-parallel                              10/10 jobs PASS (pre-push)
```

Baseline and head `shellcheck --severity=warning` each report only the same
pre-existing `SC2115` warning at line 172; this diff adds no shellcheck finding.
The fast pre-push sweep passed after the gate record was first committed; it is
useful corroboration but is not the required full 40-job suite and does not cure
the deterministic policy-lane failure.

Failed required check:

```text
candidate 3526964266: 270,612,512 bytes > 270,000,000
origin/main 7f622528d9: 270,633,240 bytes > 270,000,000
tracker: ga-iuznq2 (filed after the gate run)
```

Raw logs are under `/var/tmp/ga-6w04rl-gate.ZD86MD/` (`policy.log`,
`policy-rest.log`, `vet.log`, `lint.log`, `zcode.log`,
`resourcecensus.log`, and the shellcheck logs).

## Decision

Gate FAIL on criterion 3/3b. Do not open a release PR and do not publish deploy
clearance. Preserve this isolated evidence branch for the repository's
mandatory push/audit rule and route `ga-6w04rl` back to builder. The next gate
cycle must start from a reviewed SHA whose required native-dependency policy
lane is green; `ga-iuznq2` tracks the deterministic current-main blocker.
