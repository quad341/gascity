# Release gate: macOS unit and Linux coverage package discovery

- Deploy bead: `ga-77ibqz`
- Source build bead: `ga-1qp5qo`
- Source review bead: `ga-iny2e5`
- Reviewed source: `97a25d19a581870c24dcc7c1646efff6587d78e2`
- Provenance branch: `builder/ga-1qp5qo`
- Local evidence branch: `deploy/ga-77ibqz-gate`
- Gate base: `origin/main@a35acaaec9df59cf656fc9cefec6133b9481837c`
- Merge base: `655713ff4559c335ff17ec09c4a65f5cca2a5f27`
- Evaluation date: 2026-08-28
- Disposition: **FAIL — RETURN TO BUILDER, NO RELEASE PR**

## Gate checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | Review bead `ga-iny2e5` is closed PASS for the exact reviewed SHA. It records no candidate-owned blocking finding and explicitly carries live macOS and Linux coverage checks into release evaluation. |
| 2 | Acceptance criteria met | **FAIL** | Source inspection and dry-run policy tests verify `go-version-file: go.mod`, non-empty Make package selection, loud `go list` failure, and the XDG-platform test guard. The two live-CI criteria—real package execution in Mac Regression and a non-empty Linux coverage profile—require a PR and were not run because the local release gate did not pass. |
| 3 | Tests pass | **FAIL** | Build, smoke, policy, formatting, vet, isolated-cache lint, and 27 of the 40 documented full-suite jobs passed. Five full-suite jobs were red and eight were interrupted or had not started when root filesystem headroom fell below 1 GiB. One exact failure had no predating tracker, so it cannot be waived retroactively; it is now `ga-8qllf1`. Host disk recurrence is `ga-vtthm5`. |
| 4 | No high-severity review findings open | **PASS** | The review bead records PASS and no unresolved high-severity candidate finding. The release blockers are test-infrastructure/evidence blockers, not a new review finding. |
| 5 | Final branch is clean | **PASS** | The exact candidate was evaluated in a disposable worktree. Generated `bin/gc` and the isolated lint cache were removed, leaving only this release-gate record for the evidence commit. |
| 6 | Branch diverges cleanly from main | **PASS** | `git merge-tree --write-tree origin/main 97a25d19a581870c24dcc7c1646efff6587d78e2` exited 0 and produced tree `65882f79525a39cdaaf69a691117565e73aedd1a`. `git diff --check` and the deploy ancestry-scope guard passed. No self-rebase was performed. |
| 7 | Single feature theme | **PASS** | The eight-file effective diff is one CI package-discovery hardening theme: guarded Make package lists, policy tests, the macOS XDG skip guard, documentation, and the required resource-census update. |

## Test evidence

The candidate passed:

```text
make build
./bin/gc version --long
./bin/gc --help
make test-ci-policy
make fmt-check
go vet ./...
GOLANGCI_LINT_CACHE=<isolated-disk-path> make lint
```

The first `make lint` invocation reported 212 files from deleted or stale
sibling worktrees through the shared golangci-lint cache. The exact predating
tracker is `ga-u8z8j6`. Re-running with an isolated on-disk lint cache produced
`0 issues`.

`make test-local-full-parallel` selected the documented 40-job union. At the
safe-stop boundary its census was:

```text
27 PASS
 5 FAIL
 8 interrupted or not started
```

Red jobs and dispositions:

- `integration-packages-core-1-of-4`: `TestCompactScriptRealDoltRemotePush`
  timed out during remote push (predating `ga-ok3q3c` / `ga-4q1evf`), and
  `TestCompactScriptIsolatesBackupPushFailureFromPrimaryPush` failed its
  `beads` table-value-hash preflight. The latter had no exact predating
  tracker and is now `ga-8qllf1`; this occurrence remains unwaived.
- `integration-packages-core-2-of-4`: the known
  `TestSessionEventsLive` failure has exact merge-base reproduction in review
  evidence (`ga-at7jv0`), and `TestProviderLiveClaudeKindPath` matched the
  standing pane-busy disposition in `ga-fh1flg` / `ga-cqq3hs.1`.
- `integration-packages-core-4-of-4`: `TestBdFlagManifestCurrent` matched the
  installed-`bd` manifest drift tracked by `ga-f0uceo`.
- `integration-review-formulas-basic-2-of-2` and
  `integration-rest-smoke-2-of-2`: both matched the predating beads pending
  dirty-table failure tracked by `ga-lpfjhc`; the candidate cannot alter that
  store/bootstrap mechanism.

The run was stopped when `/` and `/var/tmp` returned to 100% usage with less
than 1 GiB free. `lsof +L1` showed hundreds of older `gc-drift` processes
retaining deleted test executables; none was created by this gate. The
ownership-safe remediation and rerun requirement are tracked by `ga-vtthm5`.
No unrelated process was killed and the shared Go cache was not cleaned.

## Decision

Gate FAIL on criteria 2 and 3. Push this branch to the fork only as a durable
failed-gate record; do not open a release PR. Although no candidate-owned
regression was established, the candidate adds declared test load and the
newly observed compact-script failure had no predating tracker, so the release
policy does not permit attribution. Route the bead back to builder with
`ga-8qllf1` and `ga-vtthm5` as the recorded failure context. A subsequent
reviewed candidate must rerun the full documented test union, then create a
fresh deploy branch and PR only if every release criterion passes.
