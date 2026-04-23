# Release Gate — ga-gdc6 (rollup-ship)

**Bead:** ga-gdc6 — `[ga-h6w + ga-d5y] Ship ADR 0001 + ADR 0002 (mega rollup)`
**Evaluated:** 2026-04-23 (fourth attempt, new SHAs from ga-tazy rebase)
**Result:** **FAIL** — cherry-picks apply cleanly but the assembled branch introduces new test failures (stale generated artifacts + broken markdown links).

## Source

`feat/h6w-d5y-rebased` @ `fork/feat/h6w-d5y-rebased` (produced by ga-tazy).
18 commits cherry-picked onto a fresh branch off `origin/main`
(`release/ga-gdc6`) with `EXCLUDES: issues.jsonl`.

## What passed

- All 18 declared SHAs resolve in the object graph (`git cat-file -e`).
- All 18 child beads CLOSED.
- All 17 non-docs children have a reviewer PASS verdict; ga-sec is covered
  by the `DOCS_ONLY` carve-out (commit `193e94c6` touches only
  `docs/runbooks/dolt-maintenance.md`).
- Cherry-pick: **all 18 applied cleanly** with the `EXCLUDES: issues.jsonl`
  recipe. No conflicts. Branch is clean.
- `go vet ./...` — clean.
- `go build ./...` — clean.

### Reviewer verdicts

| Child | Commit | Review bead | Reviewer verdict |
|-------|--------|-------------|------------------|
| ga-zol | `a257a692` | `ga-i24` | PASS |
| ga-8km | `35f2f4c9` | `ga-5tb` | PASS |
| ga-8cq | `ec71b17e` | `ga-0awq` | PASS |
| ga-zoj | `72e4bc18` | `ga-yhbi` | PASS |
| ga-p5n | `19818d49` | `ga-4xqa` | PASS |
| ga-71l | `2452afab` | `ga-yaqp` | PASS |
| ga-e3s | `da60f000` | `ga-4nh2` | PASS |
| ga-2o9 | `540460eb` | `ga-sooy` | PASS |
| ga-6q1 | `76dc58c2` | `ga-0ly6` | PASS |
| ga-idc | `e0546140` | `ga-gys0` | PASS |
| ga-06g | `e97d86e7` | `ga-sk12` | PASS |
| ga-6s5 | `a7a5ea30` | `ga-sxx5` | PASS |
| ga-74d | `bc393010` | `ga-0ydz` | PASS |
| ga-zn8 | `0b09a6e3` | `ga-7mah` | PASS |
| ga-sec | `193e94c6` | *(DOCS_ONLY)* | n/a |
| ga-gti | `1dc12385` | `ga-ut5z` | PASS |
| ga-69s | `e1b3392c` | `ga-dbrk` | PASS |
| ga-2fr | `910407ca` | `ga-djbd` | PASS |

## What failed — Criterion 3 (tests pass)

`go test ./...` on the assembled branch produces **new** failures that do
not reproduce on `origin/main` (verified in a disposable worktree at
`99742e36`).

### Regressions introduced by this rollup

1. **`test/docsync.TestSchemaFreshness/city-schema.json`** — FAIL
2. **`test/docsync.TestSchemaFreshness/config.md`** — FAIL
   Both fail with `... is stale. Run: go run ./cmd/genschema`. Root cause:
   `ga-zol` (`a257a692`) adds a new `[maintenance.dolt]` section to the
   config surface. The generated schema (`city-schema.json`) and docs
   (`config.md`) were not regenerated before the rollup was routed.

3. **`test/docsync.TestLocalMarkdownLinks`** — FAIL
   ```
   docs/runbooks/dolt-maintenance.md -> ../adr/0002-dolt-store-maintenance-runbook.md
   docs/runbooks/dolt-maintenance.md -> ../architecture/gc-read-path.md
   docs/runbooks/dolt-maintenance.md -> ../rules/dolt-store-maintenance.md
   ```
   The runbook (ga-sec, `193e94c6`) links to ADR/architecture/rule docs
   that the rollup description explicitly flags as UNTRACKED. The rollup
   expects a human to commit those docs to the PR before merge, but the
   test suite breaks in the interim — a human cannot merge a red build.

### Pre-existing failures (unrelated; also fail on `origin/main`)

These were verified against a disposable `origin/main` worktree and fail
there as well. They are **not** caused by this rollup and are not this
gate's remediation.

- `internal/runtime/k8s.TestControllerScriptDeployUsesResolvedConfigPrefixesForBootstrap`
- `internal/runtime/k8s.TestControllerScriptDeployBootstrapsAfterStartSignalAndLogProbe`
- `internal/runtime/k8s.TestControllerScriptDeployBootstrapsWhenLogsNeverMatch`
- `internal/runtime/k8s.TestControllerScriptDeployFailsWhenBootstrapFails`

Common signal: `controller bootstrap requires both GC_DOLT_HOST and
GC_DOLT_PORT when either is set` (k8s bootstrap env guard).

## Criteria table

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Review PASS present (single-pass) | PASS (17 review beads PASS, ga-sec DOCS_ONLY) |
| 2 | Acceptance criteria met (per child) | PASS (checked in review beads) |
| 3 | Tests pass on assembled branch | **FAIL** — 3 new failures in `test/docsync` |
| 4 | No high-severity review findings open | PASS |
| 5 | Final branch is clean | PASS (`git status` clean after cherry-picks) |
| 6 | Branch diverges cleanly from main | PASS (no conflicts; 18/18 applied) |

## Action taken

- Cut a fresh `release/ga-gdc6` branch off `origin/main` and cherry-picked
  all 18 SHAs with `EXCLUDES: issues.jsonl`.
- Ran `go vet ./...` (clean), `go build ./...` (clean), `go test ./...` (3
  new failures, above).
- Verified the 4 k8s failures reproduce on `origin/main` — not regressions.
- Did NOT push `release/ga-gdc6`, did NOT open a PR.
- Did NOT touch source branch or child bead status.
- Removed `needs-deploy` label on ga-gdc6 so the deployer formula stops
  firing.
- Appending findings to ga-gdc6 notes and mailing the mayor.

## Routing rationale & remediation (mayor's call)

Root cause is builder/source-branch hygiene, not reviewer miss. Two
distinct artifacts need committing before the rollup is green:

1. **Regenerated schema/docs.** `go run ./cmd/genschema` needs to run on
   the source branch and the diff committed. This would have been caught
   by CI on the source branch too; route to builder (or PM if a
   follow-up bead is preferred). Suggested path: a tiny child bead
   (`ga-h6w`/`ga-d5y` scope) committing the regen output, then prepend
   its SHA to `CHERRY_PICKS`.

2. **Missing ADR / architecture / rule docs.** ga-sec's runbook links to
   three untracked docs. Either the mayor pre-commits them to the source
   branch before re-routing, or ga-sec's links are softened (e.g.,
   removed or replaced with a TODO note) until the docs land. The
   rollup description already states the mayor owns this step.

Re-route with `needs-deploy` once both are resolved. Cherry-picks apply
cleanly — everything upstream of Criterion 3 is green this run.

## Evidence

- Assembled branch HEAD: `0666a3fa` (local `release/ga-gdc6`; not pushed).
- `go test ./...` output: 3 new `test/docsync` failures + 4 pre-existing
  `internal/runtime/k8s` failures (identical on `origin/main`).
- Baseline comparison worktree: `/tmp/main-test-ga-gdc6` @ `origin/main`
  `99742e36` (cleaned up).
