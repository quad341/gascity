# Release gate: PostgresAuthCheck warmup test coverage

- Deploy bead: `ga-xawilr`
- Source bead: `ga-uslskt.1.1`
- Reviewed source: `0888561accc88d720a127aee40677e8c28edd716`
- Provenance branch: `fork/validator/ga-uslskt-1-postgres-warmup-tests`
- Reviewed patch-id: `bff0fd8ca2d0b57873a15490c2a89c44d504d52a`
- Gate base: `origin/main@7cf9c8b3fb0bb03dac7cc89683a5f1883a641c6c`
- Superseding change: PR #5144, merge commit `a475cd9a5a9a2989b9b50702221300dbd7f83612`
- Evaluated: 2026-08-30
- Disposition: **NO RELEASE — CLOSE AS OBSOLETE**

## Gate checklist

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | The source handoff records reviewer PASS for exact commit `0888561accc88d720a127aee40677e8c28edd716`; the fork provenance branch still resolves to that SHA. |
| 2 | Acceptance criteria met on current main | **FAIL — OBSOLETE** | The candidate tests `PostgresAuthCheck.WarmupEligible`, its custom sole-failure mail, and secret exclusion. PR #5144 intentionally removed the PostgreSQL resolver, `PostgresAuthCheck`, `internal/doctor/checks`, and `--explain-postgres-auth` after moving non-Dolt backends behind the opaque store binding. Current main contains none of the production behavior these tests target. |
| 3 | Tests pass | **NOT RUN** | Criterion 6 failed first, as the deploy formula requires. Running or porting these tests would recreate coverage for intentionally deleted production behavior. |
| 3b | Policy/lint lane | **NOT RUN** | No release candidate can be formed without reversing PR #5144's settled ownership boundary. |
| 4 | No high-severity review findings open | **PASS FOR THE HISTORICAL SHA** | The prior review found no blocker in the test-only patch. That does not make the removed behavior releasable against current main. |
| 5 | Feature branch clean | **PASS** | The fork provenance branch remains at the exact reviewed SHA; the applicability probe used a separate disposable worktree and left it untouched. |
| 6 | Branch diverges cleanly from main | **FAIL** | Applying the reviewed commit to current main produces a modify/delete conflict on `internal/doctor/checks/postgres_auth_test.go`; it also adds `internal/doctor/checks/testenv_import_test.go` and `internal/warmup/runner_postgres_test.go`, which import the deleted package. This is semantic removal, not rebase drift, so review carryover does not apply. |
| 7 | Single feature theme | **PASS, BUT SUPERSEDED** | The historical patch is one test-coverage theme. PR #5144 deliberately removed that theme's production surface. |

## Applicability evidence

The prerequisite implementation did land in PR #4468 on 2026-07-21. The
candidate was then overtaken by PR #5144, merged on 2026-08-09:

```text
PR #5144: refactor(beads): make the opaque storage binding the only non-dolt shape
merge: a475cd9a5a9a2989b9b50702221300dbd7f83612

Deleted:
  internal/doctor/checks/postgres_auth.go
  internal/doctor/checks/postgres_auth_test.go
  internal/doctor/checks/testenv_import_test.go
  internal/pgauth/**
```

An isolated `cherry-pick --no-commit` of the reviewed SHA onto current main
failed before tests:

```text
CONFLICT (modify/delete):
  internal/doctor/checks/postgres_auth_test.go
  deleted in current main and modified by 0888561acc

Added by the stale patch:
  internal/doctor/checks/testenv_import_test.go
  internal/warmup/runner_postgres_test.go
```

The probe was aborted with a merge reset, its disposable worktree was removed,
and the provenance branch was not changed or pushed.

## Decision

Do not rebase, rebuild, open a PR, or publish deploy clearance. Close
`ga-xawilr` as obsolete after PR #5144; reviving these tests would require a
new product decision to restore PostgreSQL-specific behavior to the platform,
not mechanical maintenance of reviewed coverage.

