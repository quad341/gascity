# Release gate: macOS mechanical test fixes (`ga-vx0kno`)

Gate result: **FAIL**

- Evaluated: 2026-08-30
- Deploy mode: `remote`; push target resolves to `fork`
- Base: `origin/main@c7a92b25ebb100ccfd0f3a31cf2e865a5d7bfb1c`
- Reviewed source: `builder/ga-bxlbi6@4441c9e404df7fff2bb28e1b41b50e34171a63b3`
- Review bead: `ga-ll4xto` (round-2 verdict: PASS)
- Existing pull request: [#5746](https://github.com/gastownhall/gascity/pull/5746), open at the exact reviewed SHA
- Gate evidence branch: `deploy/ga-vx0kno-gate` (evidence only; no replacement PR)

## Checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Reviewer PASS present | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. The bead still records the round-2 PASS, but this re-run does not re-score later criteria. |
| 2 | Acceptance criteria met | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |
| 3 | Tests pass | **SKIPPED** | The documented full-suite command was intentionally not run after the cheap mandatory provenance check failed. `test_cmd_scope: not-run (fail-fast)`; no test PASS is claimed. |
| 3a | Pre-existing failures may be attributed | **SKIPPED** | No full-suite result was produced in this re-run, so no failure attribution was attempted. |
| 3b | Policy/lint lane | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |
| 3c | CI-config diff lane | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |
| 4 | No high-severity review findings open | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |
| 5 | Final branch is clean | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |
| 6 | Branch diverges cleanly from main | **FAIL** | The content merge is clean: `git merge-tree --write-tree origin/main 4441c9e4...` returned 0 with tree `bfd1210aa70e00d7dea8311f1a13bad237c07a2d`, merge base `655713ff4559c335ff17ec09c4a65f5cca2a5f27`. The mandatory deploy-source ancestry guard nevertheless refused with rc 21 because commit `47ad03e007c46e9e02157b209e6bf9212ef47a9e` cites none of the confirmed bead IDs `ga-vx0kno`, `ga-bxlbi6`, or `ga-ll4xto`. Its full commit message contains no bead ID, so the accepted-ID set cannot be widened legitimately. |
| 7 | Single feature theme | **SKIPPED** | Fail-fast after criterion 6/source-ancestry refusal. |

## Pre-flight and CI evidence observed

- PR #5746 is `OPEN`, authored by team member `quad341`, and its head is exactly
  `4441c9e404df7fff2bb28e1b41b50e34171a63b3`.
- The PR has only member-authored issue comments, no reviews, and no inline
  review comments. No external contributor interaction was found.
- Exact-SHA Mac Regression run
  [33263203066](https://github.com/gastownhall/gascity/actions/runs/33263203066)
  completed with overall result `failure`. Its `Mac / make test` job ran
  `TestCmdline_FailsClosedWhenUnreadable`, and the `internal/pidutil` package
  passed. The two herdr XDG tests were logged as skipped on Darwin, consistent
  with their Linux-only guard. This evidence was inspected to check the mayor's
  conditional exact-SHA waiver, but it was not used to score criterion 3 because
  the ancestry guard had already failed and the release gate must stop there.

## Provenance failure

The reviewed range contains two commits after merge base:

1. `47ad03e007c46e9e02157b209e6bf9212ef47a9e` — herdr XDG test skip guard;
   no bead ID appears in its subject or body.
2. `4441c9e404df7fff2bb28e1b41b50e34171a63b3` — pidutil test fix;
   cites `ga-bxlbi6` and says it pairs with `47ad03e007`.

The content is one feature theme, but the guard's provenance contract is
mechanical: every introduced commit must cite an accepted bead ID. Pairing prose
in a later commit cannot repair the missing provenance on the earlier commit.

## Disposition

No replacement PR was opened, PR #5746 was not mutated, and no deploy-clearance
status was published. Builder action is required: recreate/amend the
`47ad03e007` commit so its message cites `ga-bxlbi6` (or another confirmed source
bead), preserve the exact content, and return the resulting SHA for fresh review
and deploy evaluation. The mayor waiver `gm-wisp-r5i1tn` was scoped to
`4441c9e404df7fff2bb28e1b41b50e34171a63b3` and does not carry to the new SHA.
