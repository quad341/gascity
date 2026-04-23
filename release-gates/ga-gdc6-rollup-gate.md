# Release Gate — ga-gdc6 (rollup-ship)

**Bead:** ga-gdc6 — `[ga-d5y] Ship ADR 0002 — dolt store maintenance (rollup)`
**Evaluated:** 2026-04-23 (third attempt, EXCLUDES=issues.jsonl)
**Result:** **FAIL** — genuine content conflict on ga-zn8 (`internal/api/client.go`, `internal/api/types_read.go`). Source branch has diverged from `origin/main`.

## Source branch
`gc-builder-1-01561d4fb9ea` (visible as `fork/gc-builder-1-01561d4fb9ea`)

## What passed

- All 9 child beads (`ga-zol`, `ga-8km`, `ga-8cq`, `ga-zoj`, `ga-p5n`, `ga-74d`,
  `ga-zn8`, `ga-sec`, `ga-e3s`) are CLOSED.
- All 9 declared SHAs resolve in the object graph (`git cat-file -e`).
- Review beads under single-pass reviewer gate:

  | Child | Commit | Review bead | Reviewer verdict |
  |-------|--------|-------------|------------------|
  | ga-zol | `67cdfb34` | `ga-i24` | PASS |
  | ga-8km | `04b929c4` | `ga-5tb` | PASS |
  | ga-8cq | `51e6581a` | `ga-0awq` | PASS |
  | ga-zoj | `5fd4dbf2` | `ga-yhbi` | PASS |
  | ga-p5n | `86bc6259` | `ga-4xqa` | PASS |
  | ga-74d | `4a847942` | `ga-0ydz` | PASS |
  | ga-zn8 | `81a39f48` | `ga-7mah` | PASS |
  | ga-sec | `f67ed540` | *(DOCS_ONLY carve-out)* | n/a (docs/runbooks/) |
  | ga-e3s | `cf4d9ba1` | `ga-4nh2` | PASS |

- Cherry-picks 1-6 (ga-zol → ga-74d) applied cleanly with `EXCLUDES: issues.jsonl`.
  The EXCLUDES recipe from the prompt works — second-attempt blocker is resolved.

## What failed

**Criterion 6 (Branch diverges cleanly from main) — FAIL on ga-zn8.**

Cherry-picking `81a39f48` (ga-zn8: `gc maintenance CLI + API handlers`)
onto the release branch produced two non-excluded conflicts:

```
CONFLICT (content): Merge conflict in internal/api/client.go
CONFLICT (modify/delete): internal/api/types_read.go deleted in HEAD and
    modified in 81a39f48
```

Root cause — the source branch `gc-builder-1-01561d4fb9ea` predates changes
already landed on `origin/main`:

1. **`internal/api/types_read.go` was DELETED on `origin/main`.**
   `git cat-file -e origin/main:internal/api/types_read.go` → `path does not
   exist`. The file is no longer on main; ga-zn8 still edits it.

2. **`internal/api/client.go` was modified on `origin/main`** in a region
   that ga-zn8 also modifies. `git diff origin/main...81a39f48 --
   internal/api/client.go` shows ga-zn8 adds new imports + a
   `cacheNotLiveError` type; origin/main has diverged in the same file.

Deployer aborted per directive: *"If a genuine conflict (outside excluded
paths) occurs, FAIL the gate — route back to the builder for rebase. Never
resolve genuine content conflicts from the deployer seat."* The release
branch was reset to origin/main (6 staged picks discarded).

## Criteria results

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Review PASS present (single-pass) | PASS (8 review beads PASS, ga-sec DOCS_ONLY) |
| 2 | Acceptance criteria met (per child) | not re-evaluated — gated on #6 |
| 3 | Tests pass on assembled branch | not evaluated — branch not fully assembled |
| 4 | No high-severity review findings open | PASS (findings in review beads are LOW/INFO) |
| 5 | Final branch is clean | n/a — no final branch assembled |
| 6 | Branch diverges cleanly from main | **FAIL** — `internal/api/client.go` + `internal/api/types_read.go` conflicts on ga-zn8 (81a39f48) |

## Additional known gap (not the blocker this run)

Rollup description flags `docs/adr/0002-dolt-store-maintenance-runbook.md`,
`docs/architecture/gc-read-path.md`, and `docs/rules/dolt-store-maintenance.md`
as UNTRACKED on the source branch. The declared CHERRY_PICKS do not include
commits creating them. This is a separate remediation the mayor/builder must
resolve before a human can merge.

## Action taken

- Aborted the cherry-pick and reset the release branch to `origin/main`.
- Did NOT push, did NOT open PR.
- Did NOT touch the source branch or any child bead status.
- Left `rollup-ship` label on ga-gdc6. Removed `needs-deploy` so the deployer
  formula does not re-fire on unchanged state.
- Appended findings to ga-gdc6 notes.
- Mailed the mayor.

## Routing rationale

Root cause: the source branch `gc-builder-1-01561d4fb9ea` predates changes
that have since landed on `origin/main` — specifically the deletion of
`internal/api/types_read.go` and modifications to `internal/api/client.go`.
This is a stale-source-branch problem, not a reviewer miss.

Options for remediation (mayor's call):

1. **Builder rebases `gc-builder-1-01561d4fb9ea` onto latest `origin/main`**
   and updates ga-zn8's changes to not reference the deleted
   `internal/api/types_read.go`. This may require the reviewer to re-check
   ga-zn8 since API-layer code is touched. Then PM amends the rollup with
   the new ga-zn8 SHA and re-routes `needs-deploy`.

2. **Builder creates a new ga-zn8-redux commit** that replays the intent of
   `81a39f48` against current main, and replaces it in CHERRY_PICKS.

Per deployer prompt this is routed back via mail to the mayor (not
`ready-to-build`) because the failure is pipeline hygiene (stale source
branch) the mayor should triage.
