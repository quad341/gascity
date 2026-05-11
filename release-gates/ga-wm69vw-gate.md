# Release gate — ga-wm69vw (ga-9h05hk slice 2/5) — **FAIL: needs rebase**

**Bead:** `ga-wm69vw` — review bead for `ga-9h05hk` (live-session SHOW PROCESSLIST probe + fail-closed; slice 2/5 of `ga-nw4z6`)
**Builder branch:** `quad341:builder/ga-9h05hk-1` @ `9ba6a298`
**Source bead:** `ga-9h05hk` (closed)
**Design:** `ga-lyv6d4` (closed)
**Merge base with `origin/main`:** `28c7416c` (v1.1.0 tag)
**`origin/main` HEAD:** `96f9759d`
**Date:** 2026-05-11

## Verdict: FAIL — branch carries 12 integration-baseline commits that must be dropped

The slice-2 *work itself* is sound. Cherry-picking the three in-scope
commits onto current `origin/main` is **clean**, tests pass, vet/build
clean. The problem is purely packaging: the source branch is built on
top of `local/integration-2026-04-30`, which diverged from main on
`28c7416c` (the v1.1.0 release tag). Since then, main has shipped
**182 commits**. The branch carries 12 integration-baseline commits
that are not part of slice-2 scope, several of which have already
landed on main at different SHAs through their own PRs.

| # | Criterion | Evidence | Status |
|---|-----------|----------|--------|
| 1 | Review PASS present | `gascity/reviewer` PASS verdict in `ga-wm69vw` notes (full design checklist walked; race detector clean; 5/5 new tests + existing TestRunDoltCleanup/TestPlanDoltDrops green; OWASP review clean; 5 info-level findings, no request-changes) | PASS |
| 2 | Acceptance criteria met | All 16 acceptance checkboxes in design `ga-lyv6d4` §8 hit. Verbatim contract (SQL, 2s timeout, reason strings, function signatures, 5 test names) byte-identical to spec. Two documented deviations are minimal and justified. | PASS |
| 3 | Tests pass on the assembled branch | Verified locally: `go test ./cmd/gc -run 'TestProbeLiveSessions\|TestRunDoltCleanup\|TestPlanDoltDrops' -count=1` **PASS** on cherry-picked tip; `go vet ./...` clean; `go build ./...` clean. The actual slice-2 changes do not regress against current main. | PASS (after rebase) |
| 4 | No high-severity review findings open | Reviewer findings: 5 info-level only, no high/medium. Zero request-changes. | PASS |
| 5 | Final branch is clean | N/A — no deployer-side commits attempted on the feature branch. | N/A |
| 6 | Branch diverges cleanly from main | **FAIL.** Branch is 15 commits ahead, 182 commits behind. The 12 baseline commits are out of scope for slice-2. Opening the PR as-is would produce a diff of **533 files / 5051 ins / 45251 del** — most of those deletions are reverts of specs/architecture (555 lines), 13 release-gates files, internal/{sling,sourceworkflow,telemetry,worker,workspacesvc,...} tests, and other unrelated content that has accrued on main since `28c7416c`. The actual slice-2 scope is **10 files / 774 ins / 4 del**. | **FAIL** |

## Diagnosis

The 15 commits between merge base `28c7416c` and branch tip `9ba6a298`:

**Slice-2 scope (3 commits, KEEP):**

| SHA | Subject |
|-----|---------|
| `42831af2` | docs(plans): decompose ga-u0lx9p + ga-lyv6d4 into builder beads (ga-rq2e5a, ga-9h05hk) |
| `d3530a7f` | fix(lint): inline reflect.Ptr → reflect.Pointer (govet) |
| `9ba6a298` | feat(dolt-cleanup): live-session SHOW PROCESSLIST probe + fail-closed (ga-nw4z6 slice 2/5) |

**Integration-baseline commits (12, DROP):**

| SHA on branch | Already-on-main equivalent (different SHA — rebased through PR) |
|---------------|---------|
| `84fd9f96` fixup(rebase): restore main's versions of conflict files + test fixup | (rebase-fixup; subsumed by main state) |
| `71f21f6f` fix(reconciler): orphan stale pending-create beads with no active lease | (not directly searched; expected on main) |
| `d23f5dd8` fix(session): commit async start result when session has advanced to active | `4649e710` (#1531) on main |
| `2b395ff6` harden(hook): keep claim flow non-intrusive | `ff5d7eaf` (#1517) on main |
| `2a0cfa4c` fix(handoff): replace select{} with bounded poll loop in cmdHandoff | `ece15565` (#1481) on main |
| `be5a3dd0` fix(lint): cancelled → canceled (misspell) | (subsumed by main lint state) |
| `4d582c2b` docs(runtime): align request-restart help text with new poll-and-signal loop | (subsumed by main docs state) |
| `b2ecad89` fix: replace select{} with bounded poll loop in doRuntimeRequestRestart (gc-8jy) | `d776e06a` on main |
| `183f83b6` fix(dispatch): close orphaned workflow finalizers instead of crashing serve loop | `56fac6da` (#1470) on main |
| `7c7366a4` fix(spawn): prepend gc bin dir to agent PATH so bare 'gc' resolves correctly | `2852c765` (#1490) on main |
| `0b672027` fix(sling): set bead assignee in addition to gc.routed_to metadata | (not directly searched; expected on main) |
| `1fcf9c67` fix(session): treat instance_token as authoritative for stale async start | `4a74c6c7` (#1528) on main |

Most of these have already landed on main at different SHAs through their own PRs.
On rebase, `git rebase` will detect them as already-applied by patch-id
and skip them; the remaining handful that haven't landed should drop
cleanly because main's state already supersedes them.

## Cherry-pick verification

I cut a throwaway branch off current `origin/main` (`96f9759d`) and
cherry-picked the three slice-2 commits in order:

```
=== cherry-pick 42831af2 (docs(plans): decompose ga-u0lx9p + ga-lyv6d4) — CLEAN
=== cherry-pick d3530a7f (fix(lint): inline reflect.Ptr → reflect.Pointer) — CLEAN
=== cherry-pick 9ba6a298 (feat(dolt-cleanup): live-session probe + fail-closed) — CLEAN
```

Resulting diff vs `origin/main`: 10 files / 774 insertions / 4 deletions.

Verification on the cherry-picked tip:

- `go test ./cmd/gc -run 'TestProbeLiveSessions|TestRunDoltCleanup|TestPlanDoltDrops' -count=1` — **PASS** (1.589s)
- `go vet ./...` — clean
- `go build ./...` — clean

## Action: route back to `gascity/builder` for rebase

The fix is mechanical:

1. `git fetch origin main`
2. `git checkout builder/ga-9h05hk-1`
3. `git rebase origin/main` — patch-id matching will drop most of the
   12 baseline commits; the remainder (already on main at different
   SHAs but same content) should drop cleanly with no genuine
   conflicts. No expected conflicts on the slice-2 scope itself
   (verified above).
4. After rebase, verify with `git log --oneline origin/main..HEAD` —
   should show exactly 3 commits: `42831af2`-equivalent,
   `d3530a7f`-equivalent, `9ba6a298`-equivalent (new SHAs after
   rebase).
5. `make test` or at minimum `go test ./cmd/gc/... ./internal/api/...
   -count=1`; vet/build.
6. Force-push to `quad341:builder/ga-9h05hk-1`.
7. Re-route to deployer once CI clears.

Per deployer protocol, the deployer does not rebase from its own seat
("Never force-push. If the branch has diverged from main, route back
to the builder instead of rebasing from the deployer seat.").

## Parallel state

Sibling slice 1/4 (`ga-pnqg.1`) is in the same shape — gate-FAILed
on 2026-05-05 for the same reason (branch behind main, needs rebase).
The builder has not yet rebased that branch either. The PG-auth ADR
slices and this dolt-cleanup slice are unrelated changesets; they
should rebase independently.

## References

- Source bead: `ga-9h05hk` (closed)
- Design: `ga-lyv6d4` (closed; docs/plans/dolt-cleanup-live-session-probe.md)
- Parent ADR bead: `ga-nw4z6`
- Sibling slice gate FAIL: `release-gates/ga-pnqg.1-gate.md` (commit `7f67d052`)
- Pre-existing test failures on integration branch: `ga-h32gje`
