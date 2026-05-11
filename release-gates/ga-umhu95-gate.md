# Release Gate: ga-umhu95 — drift detection phase 3 (ga-xbgq + ga-9x22p8 follow-up)

**Deploy bead:** ga-umhu95
**Originating beads:** ga-xbgq (phase 3 integration tests + builder fixes), ga-9x22p8 (test follow-ups)
**Source branch:** `gc-builder-1-01561d4fb9ea` (fork: `quad341/gascity`), HEAD `37dd803f`
**Commits intended for release:** 0e04c195, ee37578d, 1cca9c98, 6d6228b6, 37dd803f
**Verdict:** FAIL — routing back to builder; phase 3 cannot ship without phase 1+2 on main

## Failure summary

The reviewer's PASS on ga-umhu95 covers phase 3 (ga-xbgq) plus the ga-9x22p8 test follow-ups. The reviewer tested on the builder branch where phases 1+2 are already present, so production builds and tests all pass in-place. The deployer's gate, however, requires the work to ship as a clean diff against `origin/main` — and that path is blocked because **phase 1 and phase 2 of ga-a3ry.1 are not on `origin/main`**:

- **Phase 1 (`ga-fd01`)** — review PASS verdict, but bead is currently `gc.routed_to=gascity/builder` and labeled `ready-to-build`. A clean candidate branch `builder/ga-a3ry-1` exists on fork (one commit `01cbb6e2`) but it is based on `481ea61b` (origin/main as of ~2026-05-04), now 89 commits behind current main (`fde67b7c`). Phase 1 is not in a deploy-ready state.
- **Phase 2 (`ga-xxqx`)** — review PASS but the prior deployer FAILed its gate (release-gates/ga-a3ry-1-gate.md on `deploy-fail/ga-a3ry-1`). Phase 2 cherry-pick produced 13 conflict blocks across 9 API-surface files. ga-xxqx notes call for a builder REBUILD against current main, not a cherry-pick. That work has not happened.
- **Phase 3 (`ga-umhu95`)** — my bead. The new test files in `0e04c195` reference symbols that live only in phase 1/2 production code (`DetectBinaryDrift`, `DetectPackDrift`, `PackRootStatus`, `supervisorAliveHook`, `newHTTPSupervisorClient`). The builder fix in `ee37578d` modifies `cmd/gc/cmd_start_drift.go` and `cmd/gc/drift.go`, both of which were *created* in the phase 1/2 commits. Cherry-picking phase 3 onto current main produces:
    - `cmd/gc/cmd_start_drift.go` — modify/delete conflict (file does not exist on main)
    - `cmd/gc/drift.go` — modify/delete conflict (file does not exist on main)
    - `docs/reference/cli.md` — content conflict (CLI docs evolved on main)
    - `cmd/gc/cmd_start_drift_bench_test.go` — vet failure: `undefined: PackRootStatus` even after the validator's tests apply cleanly, because the phase 1 type is absent.

Per the deployer guardrail "Never resolve genuine content conflicts from the deployer seat," this is a FAIL.

## Reproduction

```bash
git checkout -B test/cherry-pick-ga-xbgq origin/main
git cherry-pick 0e04c195     # validator's TDD-RED tests — applies (test files are new)
go vet ./cmd/gc/...           # FAILS: undefined: PackRootStatus
git cherry-pick ee37578d     # FAILS: modify/delete on drift.go, cmd_start_drift.go;
                             #         content conflict on docs/reference/cli.md
```

## Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | PASS | gascity/reviewer PASS verdict in ga-umhu95 notes (2026-05-11); covered phase 3 + ga-9x22p8 follow-up on HEAD 37dd803f. |
| 2 | Acceptance criteria met | PASS (in-place) | All TestStartDrift_* tests pass on the builder branch where phase 1+2 production code is present. Reviewer reran 5 integration tests + drift unit suite; all green. |
| 3 | Tests pass | not run | Tests not run on a clean assembled branch — cherry-pick failed before a runnable branch existed. |
| 4 | No high-severity review findings open | PASS | Zero high-severity findings on the review. Five info-level observations, all non-blocking. |
| 5 | Final branch is clean | n/a | No final branch produced. |
| 6 | Branch diverges cleanly from main | **FAIL** | `ee37578d` produces modify/delete conflicts on `cmd/gc/cmd_start_drift.go` and `cmd/gc/drift.go` against current `origin/main` (`fde67b7c`); both files were created in phase 1/2 commits that are not yet on main. `0e04c195`'s new test files vet-fail (`undefined: PackRootStatus`) without phase 1's `cmd/gc/drift.go`. |

## Recommended path forward

The drift-detection work is split across four logical phases tied together by symbol dependencies:

1. Phase 1 — `c1b203c7` (drift detection foundation, defines `DetectBinaryDrift`, `DetectPackDrift`, `PackRootStatus`, `restartLoopGuard` types)
2. Phase 2 — `a1a8c094` + `62f3e26c` + `f7f59f54` (BuildID on /health, HTTP client, wire-up in `gc start`)
3. Phase 3 — `0e04c195` + `ee37578d` (validator's integration tests + builder's permission-denied / persistent loop guard / `(deleted)` strip / WaitExit fixes)
4. Follow-up — `1cca9c98` + `6d6228b6` + `37dd803f` (ga-9x22p8: systemd unit-name parity + drift_history edge-case unit tests + XDG_RUNTIME_DIR fix)

Phase 3 cannot ship before phases 1+2. There are two viable strategies:

**Option A — Stacked PR train (preferred per project convention).** Rebuild phase 2 against current main (the ga-xxqx ask from 2026-05-04 that is still open), then re-cherry-pick phase 1 + phase 2 + phase 3 + ga-9x22p8 follow-up onto current main as ONE clean branch. Re-route the topmost review bead (ga-umhu95) to deployer with a note that this branch ships everything together, and close ga-fd01 / ga-xxqx with pointers to the bundled PR. This collapses three PRs into one and matches the natural review unit (the ADR / feature is "supervisor drift detection in `gc start`," not three independent phases).

**Option B — Three sequential PRs.** Land phase 1 first (rebuild `builder/ga-a3ry-1` onto current main, ship via ga-fd01), then phase 2 (ga-xxqx rebuild), then phase 3 (ga-umhu95, rebased on top). Requires three rounds of PR review and three merge waits — more overhead, but smaller individual diffs. Only worth it if the API-surface changes in phase 2 warrant independent review.

Either way, the builder must rebuild phase 2 against current main. ga-xxqx is the unblocking bead.

## Builder ask

1. **Pick up ga-xxqx** (phase 2 rebuild). The cherry-pick failed on 13 conflict blocks across 9 files in `cmd/gc/` and `internal/api/` (huma-registered handlers, openapi.json regeneration, supervisor /health surface). Reauthor phase 2's BuildID-on-/health + restart infra + drift wire-up against the current API control-plane shape — keep `TestOpenAPISpecInSync` green.
2. Once phase 2 lands cleanly on a fresh branch off current `origin/main`, re-cherry-pick **phase 1** (`c1b203c7`) onto it (the existing `builder/ga-a3ry-1` is 89 commits stale; better to start fresh).
3. Then re-cherry-pick **phase 3** (`0e04c195` + `ee37578d`) and **ga-9x22p8** (`1cca9c98` + `6d6228b6` + `37dd803f`) on top.
4. Push the assembled branch to fork; update ga-umhu95 description with the new branch name and `gc.routed_to=gascity/deployer` for stacked deploy. Note Option A vs B explicitly so the deployer knows whether to open one PR or three.

The PASS verdict on the **content** of ga-umhu95 stands — phase 3 + ga-9x22p8 are good code. The blocker is purely staging: the prerequisites aren't on main yet.
