# Release gate: macOS mechanical test fixes (`ga-vx0kno`)

Gate result: **FAIL**

- Evaluated: 2026-08-29
- Deploy mode: `remote`
- Base: `origin/main@157858d9ee8bd6ab85e4a0d2128f34dc2e166a7f`
- Reviewed source: `builder/ga-bxlbi6@4441c9e404df7fff2bb28e1b41b50e34171a63b3`
- Review bead: `ga-ll4xto` (round 2 verdict: PASS)
- Existing pull request: [#5746](https://github.com/gastownhall/gascity/pull/5746), open at the exact reviewed SHA
- Gate evidence branch: `deploy/ga-vx0kno-gate` (evidence only; no replacement PR)

## Checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Reviewer PASS present | **PASS** | `ga-ll4xto` records a round-2 PASS for the exact deploy SHA. |
| 2 | Acceptance criteria met | **PASS** | The two herdr tests call `skipUnlessXDGPlatform`; the pidutil test probes PID 1 and fails closed when its argv is unreadable; the Linux affected-package check had no diff-owned failure; `go vet ./...` passed; and the builder explicitly flagged Darwin-only verification. This is a check of the bead's written done-when conditions, not an exception to criterion 3's stricter no-skip rule. |
| 3 | Tests pass | **FAIL** | The documented full suite completed with 23/40 jobs PASS and 17/40 jobs FAIL. More importantly, all three changed tests are skipped on one exercised platform: pidutil skips on Linux, while both herdr tests skip on Darwin. No mayor/operator waiver exists. Four additional first-sighting failures lacked a predating tracker at run time and therefore also fail closed. Details follow. |
| 3b | Policy/lint lane | **PASS** | `make test-ci-policy` passed; `go vet ./...` passed. |
| 4 | No high-severity review findings open | **PASS** | The independent review reports no style or security findings, and the gate inspection found no high-severity production risk in this test-only diff. |
| 5 | Final branch is clean | **PASS** | The reviewed source worktree was clean before this gate artifact was added; `git diff --check` on the candidate diff passed. The only gate-branch delta is this evidence file. |
| 6 | Branch diverges cleanly from main | **PASS** | `git merge-tree --write-tree origin/main 4441c9e404df7fff2bb28e1b41b50e34171a63b3` succeeded with tree `01cf58874b51fd8ae5720fd51416cdd0e9ab97d1`; merge base is `655713ff4559c335ff17ec09c4a65f5cca2a5f27`. |
| 7 | Single feature theme | **PASS** | The diff is two test files, 34 insertions and 11 deletions, with one theme: correcting macOS-sensitive test assumptions exposed by restoring the Mac unit package list. |

## Test evidence

`test_cmd_scope`: `full-suite`

`test_cmd`:

```text
DOCKER_HOST=unix:///run/user/1000/podman/podman.sock TESTCONTAINERS_RYUK_DISABLED=true make test-local-full-parallel
```

`test_counts`: **23/40 jobs PASS, 17/40 jobs FAIL**. The full-suite runner's
job summary does not expose an exhaustive test-event PASS/SKIP count, so none is
inferred here. Full log: `/var/tmp/ga-vx0kno-full.log`; per-job logs:
`/var/tmp/gc-local-tests.sRvHTQ`.

Focused Linux diagnostic, run at the exact reviewed SHA:

```text
go test -json -count=1 ./internal/pidutil/... ./internal/runtime/herdr/... -run <three changed tests>
```

`diff_tests_executed`:

| Changed test | Linux result | Darwin result at exact SHA |
|---|---|---|
| `TestCmdline_FailsClosedWhenUnreadable` | **SKIP** — PID 1 argv is readable | PASS |
| `TestSocketPathHonorsXDGConfigHomeOverHome` | PASS | **SKIP** — helper excludes non-Linux |
| `TestSocketPathFallsBackToHomeConfigWhenXDGUnset` | PASS | **SKIP** — helper excludes non-Linux |

The Darwin results come from the real `Mac / make test` job `99029090669` in
workflow-dispatch run [33225744772](https://github.com/gastownhall/gascity/actions/runs/33225744772),
which checked out the exact reviewed SHA. The two herdr tests are individually
logged as skipped even though their package passed; the earlier review's package-
level summary misclassified them as individual PASS results.

`waiver_ref`: **none**. Neither the deploy bead nor review bead contains a
mayor/operator waiver for these diff-owned skips. Under the release-gate
criterion-3 convention, a justification for a platform skip is not itself a
waiver, so this is a hard failure.

## Raw full-suite failures

The following failures have predating trackers and do not overlap the two-file
candidate diff:

- beads dirty-table/bootstrap signatures: `ga-lpfjhc`, with focused trackers
  `ga-ukteq1`, `ga-ylwqm3`, and `ga-81syu4`
- Dolt server/readiness and connection failures: `ga-piz22t`, `ga-561tqj`,
  `ga-tr4hod`, `ga-gajll3`, `ga-thuouz`, and `ga-gm5f4e`
- session reconciler async-start timeout: `ga-hgjlhi`
- Dolt compact/reaper/sweep process failures: `ga-ok3q3c`, `ga-4q1evf`,
  `ga-vbyn8v`, `ga-fawr0t`, and `ga-pfi425`
- stale bd flag manifest: `ga-f0uceo`
- `TestSessionEventsLive`: `ga-idsv6m` / `ga-at7jv0`
- `DisableAndPurge` concurrent-state family: `ga-s759zk`

Four signatures were first seen in this gate run. Trackers were created for
follow-up, but because they did not predate the run they cannot be used as
retroactive criterion-3 attribution:

- `ga-s0rwap` — ACP handshake timeout loses captured stderr
- `ga-mjv35r` — Docker session failed-start subtests receive `SIGKILL`
- `ga-jdm83k` — personal-work formula loses its Dolt database probe connection
- `ga-x48epi` — adopt-PR transient-retry test times out probing Dolt identity

## Disposition

No deploy clearance. Existing PR #5746 is left open and unchanged; no new PR is
opened and nothing is merged. Route `ga-vx0kno` back to the builder to obtain an
operator waiver or replace the platform skips with execution evidence that
satisfies criterion 3, then re-run the complete deploy gate.
