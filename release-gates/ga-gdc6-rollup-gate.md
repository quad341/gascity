# Release Gate — ga-gdc6 (rollup-ship)

**Bead:** ga-gdc6 — `[ga-d5y] Ship ADR 0002 — dolt store maintenance (rollup)`
**Evaluated:** 2026-04-23 (second attempt, gemini-reviewer now disabled)
**Result:** **FAIL** — every declared cherry-pick conflicts on `issues.jsonl`.

## Source branch
`gc-builder-1-01561d4fb9ea` (visible as `fork/gc-builder-1-01561d4fb9ea`)

## What passed

- All 9 child beads (`ga-zol`, `ga-8km`, `ga-8cq`, `ga-zoj`, `ga-p5n`, `ga-74d`,
  `ga-zn8`, `ga-sec`, `ga-e3s`) are CLOSED.
- All 9 declared SHAs resolve in the object graph (`git cat-file -e`) and are
  ancestors of `fork/gc-builder-1-01561d4fb9ea`.
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

## What failed

**Criterion 6 (Branch diverges cleanly from main) — FAIL.**

`issues.jsonl` is present on the source branch but does not exist on
`origin/main`; `git diff --name-status origin/main fork/gc-builder-1-01561d4fb9ea -- issues.jsonl`
reports `A issues.jsonl` (added). Every one of the 9 declared cherry-picks
modifies `issues.jsonl`:

```
67cdfb34: touches issues.jsonl
04b929c4: touches issues.jsonl
51e6581a: touches issues.jsonl
5fd4dbf2: touches issues.jsonl
86bc6259: touches issues.jsonl
4a847942: touches issues.jsonl
81a39f48: touches issues.jsonl
f67ed540: touches issues.jsonl
cf4d9ba1: touches issues.jsonl
```

Cherry-picking the first commit (`67cdfb34`) onto a branch cut from
`origin/main` produces `deleted by us: issues.jsonl` on every pick because
the file does not exist on main. Deployer aborted the cherry-pick per the
prompt directive "Conflict → FAIL, do not attempt to resolve." The assembled
release branch was deleted.

## Criteria results

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Review PASS present (single-pass) | PASS (8 review beads PASS, ga-sec DOCS_ONLY) |
| 2 | Acceptance criteria met (per child) | not re-evaluated — gated on #6 |
| 3 | Tests pass on assembled branch | not evaluated — no clean branch to test |
| 4 | No high-severity review findings open | PASS (findings in review beads are LOW/INFO) |
| 5 | Final branch is clean | n/a — no final branch assembled |
| 6 | Branch diverges cleanly from main | **FAIL** — `issues.jsonl` conflict on every commit |

## Additional known gap (not the blocker this run)

Rollup description flags `docs/adr/0002-dolt-store-maintenance-runbook.md`,
`docs/architecture/gc-read-path.md`, and `docs/rules/dolt-store-maintenance.md`
as UNTRACKED on the source branch. The declared CHERRY_PICKS do not include
commits creating them. This is a separate remediation the mayor/builder
must resolve before a human can merge.

## Action taken

- Aborted the cherry-pick and deleted the release branch.
- Did NOT push, did NOT open PR.
- Did NOT touch the source branch or any child bead status.
- Left `rollup-ship` label on ga-gdc6. Removed `needs-deploy` so the
  deployer formula does not re-fire on unchanged state.
- Appended findings to ga-gdc6 notes.
- Mailed the mayor.

## Routing rationale

Root cause is that the source branch carries `issues.jsonl` as an added
file relative to `origin/main`. Options for remediation (mayor's call):

1. **Land a separate PR that adds `issues.jsonl` to `main`** (or gitignore
   it globally) so subsequent rollups can cherry-pick cleanly; then
   re-route ga-gdc6 as `needs-deploy`.
2. **Builder rebases the source branch** to squash or drop the bd-sync
   commits, and/or to remove `issues.jsonl` from the picked commits so
   each cherry-pick's diff is purely code changes.
3. **PM amends the rollup bead's CHERRY_PICKS** to reference code-only
   SHAs (e.g., a pre-amended set the builder produces) — but this still
   requires the builder to rewrite history; the deployer cannot mint new
   commits from this seat without resolving conflicts.

Per deployer prompt this is routed back via mail to the mayor (not
`ready-to-build`) because the failure is not "reviewer missed a bug";
it is a pipeline hygiene issue the mayor should triage.
