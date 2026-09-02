# Release Gate: ga-5pgqkm supervisor unit ownership

Date: 2026-09-02
Deployer: gascity/deployer
Status: FAIL
Source bead: ga-9pjtoy
Review bead: ga-2xa37z
Reviewed commit: 60ffdcba4981a8f4f70754ccf4e4a975e884b251
Base checked: origin/main at ed146d8d9f2fdf142b4b23540ff0412fd2eec33c
Source branch: builder/ga-9pjtoy
Push remote selected: fork

`docs/PROJECT_MANIFEST.md` is not present in this checkout, so this gate uses
the deployer release criteria and the repository testing policy in `TESTING.md`.

## Gate summary

| # | Criterion | Result | Evidence |
|---|-----------|--------|----------|
| 1 | Review PASS present | SKIPPED | Criterion 6 failed first; remaining criteria were not evaluated under the fail-fast rule. |
| 2 | Acceptance criteria met | SKIPPED | Criterion 6 failed first; remaining criteria were not evaluated under the fail-fast rule. |
| 3 | Tests pass | SKIPPED | The full-scope test command was deliberately not run after criterion 6 failed. |
| 4 | No high-severity review findings open | SKIPPED | Criterion 6 failed first; remaining criteria were not evaluated under the fail-fast rule. |
| 5 | Final branch is clean | SKIPPED | Criterion 6 failed first; remaining criteria were not evaluated under the fail-fast rule. |
| 6 | Branch diverges cleanly from main | FAIL | The reviewed commit did not contain current `origin/main` (`git merge-base --is-ancestor origin/main 60ffdcba4981a8f4f70754ccf4e4a975e884b251` exited 1). The mandated bounded self-rebase rewrote the two feature commits cleanly onto `origin/main`, producing local tip `4e0f70f9613f281eb373d198c138d8afcf81ed88`, but its guarded push did not complete and the helper returned `rc=13`. The selected fork has no `builder/ga-9pjtoy` ref; origin still holds the reviewed pre-rebase tip. Per the helper contract, the remote branch is not advanced and the bead must return to the builder. |
| 7 | Single feature theme | SKIPPED | Criterion 6 failed first; remaining criteria were not evaluated under the fail-fast rule. |

## Criterion 6 evidence

```text
reviewed_sha=60ffdcba4981a8f4f70754ccf4e4a975e884b251
base_sha=ed146d8d9f2fdf142b4b23540ff0412fd2eec33c
merge_base=6e441be0ae8b95a691f4c26fd1ba4b9eb5ed1780
self_rebase_before=60ffdcba4981a8f4f70754ccf4e4a975e884b251
self_rebase_after_local=4e0f70f9613f281eb373d198c138d8afcf81ed88
self_rebase_rc=13
fork_branch_ref=absent
origin_branch_ref=60ffdcba4981a8f4f70754ccf4e4a975e884b251
```

The origin push dry-run did not exit successfully because its pre-push gate was
waiting for occupied test slots; the deploy protocol therefore selected `fork`
by exit status before invoking the bounded self-rebase.

## Decision

FAIL. No deploy branch was cut, no PR was opened, and no merge action was taken.
The builder must publish a current reviewed-equivalent branch and return it for
release evaluation.
