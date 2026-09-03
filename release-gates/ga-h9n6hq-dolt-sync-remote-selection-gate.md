# Release gate: deterministic Dolt sync remote selection

- Deploy bead: `ga-h9n6hq`
- Review of record: `ga-d9wgal`
- Reviewed commit: `ce6f114a94d79d0ccd7deba99aa133328366f143`
- Base: `origin/main@fcbd34178b1c86ec14f5b88ebc40dbe805f224ed`
- Deploy mode: remote
- Push remote: `fork`
- Evaluated: 2026-09-03
- Verdict: **FAIL** — nothing was pushed and no pull request was opened

## Gate checklist

The target-already-merged pre-flight found no pull request carrying the reviewed
commit. Criterion 6 was evaluated first after fetching current `origin/main`.
The reviewed branch did not contain the current base, so the required bounded
self-rebase was attempted from the internally authored source branch.

The helper rebased the local branch from
`ce6f114a94d79d0ccd7deba99aa133328366f143` to
`47c54d1d784deb228f1e684b01b79c4d27969bdc`, but its lease-protected push to
`fork` did not complete. Under `attempt_bounded_self_rebase`'s contract this is
`rc=13`, so criterion 6 remains failed and all later criteria are skipped.
The source remote remains unchanged at the reviewed commit, and the fork has no
`builder/ga-h9n6hq-conflict-fix` ref.

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **SKIPPED** | Fail-fast after criterion 6. Review bead `ga-d9wgal` records `verdict: pass` for the reviewed commit. |
| 2 | Acceptance criteria met | **SKIPPED** | Fail-fast after criterion 6. |
| 3 | Tests pass | **SKIPPED** | The full-suite command was not run because criterion 6 failed first. `test_cmd: not run`; `test_cmd_scope: n/a`; `test_counts: n/a`; `diff_tests_executed: not evaluated`; `waiver_ref: none`. |
| 3a | Pre-existing failures attributed | **SKIPPED** | No criterion-3 run was performed. |
| 3b | Policy and static lanes | **SKIPPED** | Required lanes were not run because criterion 6 failed first. `policy_lane: not run (fail-fast)`. |
| 3c | CI-config lane execution | **SKIPPED** | Required lanes were not evaluated because criterion 6 failed first. |
| 4 | No high-severity review findings open | **SKIPPED** | Fail-fast after criterion 6. |
| 5 | Final branch is clean | **SKIPPED** | The source branch was clean before the bounded self-rebase and remained clean after the rejected push, but this criterion was not scored after criterion 6 failed. |
| 6 | Branch diverges cleanly from main | **FAIL** | After fetching `origin/main`, `git merge-base --is-ancestor origin/main ce6f114a94d79d0ccd7deba99aa133328366f143` exited 1. The absolute deployer helper ran with `PUSH_REMOTE=fork`; it rebased locally to `47c54d1d784deb228f1e684b01b79c4d27969bdc`, but the lease-protected push failed (`rc=13`). `origin/builder/ga-h9n6hq-conflict-fix` remains at `ce6f114a94d79d0ccd7deba99aa133328366f143`; the fork branch is absent. |
| 7 | Single feature theme | **SKIPPED** | Fail-fast after criterion 6. |

## Failed-gate disposition

Route `ga-h9n6hq` back to the builder. The builder must reconcile the rebased
local state with an owned remote branch, then route the final exact commit
through review before another deploy evaluation. No deploy branch, remote push,
pull request, or deploy-clearance status was created.
