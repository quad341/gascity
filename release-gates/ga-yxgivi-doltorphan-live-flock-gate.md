# Release gate: Dolt-orphan live-flock protection

- Bead: `ga-yxgivi`
- Reviewed commit: `a2d50d633971c31ddaea7bf619f7e0c3c4be20c5`
- Source branch (provenance only): `builder/ga-63rfxj`
- Deploy mode: `remote`
- Base: `origin/main@8556a801c380ba9e43c04daa58c969f988021324`
- Evaluated: 2026-08-26
- Overall verdict: **FAIL**

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Review PASS present | **PASS** | Reviewer bead `ga-dkh58q` records `verdict: pass` for the exact reviewed commit. |
| 2 | Acceptance criteria met | **PASS** | `go test -count=1 -v ./internal/doltorphan/...` PASSed all 19 top-level tests, including every diff-owned test. `go test -count=1 -v -tags integration ./examples/gastown/... -run '^TestSweep_ReapsRealDoltDataDirAfterSIGKILL$'` PASSed the real-Dolt SIGKILL acceptance path. The same seven tests also PASSed in the full-suite run. |
| 3 | Tests pass | **FAIL** | `DOCKER_HOST=unix:///run/user/1000/podman/podman.sock TESTCONTAINERS_RYUK_DISABLED=true GOFLAGS=-v GO_TEST_TIMEOUT=30m make test-local-full-parallel`; `test_cmd_scope: full-suite`; `test_counts: 78,077 PASS / 33 FAIL / 270 SKIP`; 30 of 40 shard jobs PASSed. All diff-owned tests PASSed with zero diff-owned SKIPs. Three non-diff-owned failure signatures had no exact tracker predating this run: `TestAdoptPRFormulaRetriesTransientReviewerStep` and `TestHumaBinary_SessionMessageAsync` both encountered a dirty `issues` table during schema migration, while `TestLegacyCombinedStaticSnapshotHoldsConcreteWriterFence` timed out waiting for its SQLite child protocol. Trackers `ga-ago7nm`, `ga-i4tsvn`, and `ga-3czldl` were filed and read-back verified after the run; they make the regressions visible but cannot retroactively satisfy criterion 3a(ii). The 270 SKIPs are conditional or infrastructure skips outside the diff; criterion 3 independently fails. |
| 3b | Policy/lint lane | **PASS** | `make test-ci-policy` PASSed. `make lint` still reported two `govet` and one `revive` diagnostic in the ignored, generated `internal/api/dashboardspa/web/node_modules/flatted/.../flatted.go`. Tracker `ga-di310j` predates the audited lint rerun; `git check-ignore` confirms `node_modules/` is ignored, `go vet ./...` PASSed, and the candidate changes only `go.mod` and `internal/doltorphan`, so the lint failure is attributed as pre-existing and non-diff-owned. |
| 4 | No high-severity review findings open | **PASS** | The reviewer recorded no blocker or major security finding; the residual lock-probe TOCTOU was explicitly minor and non-blocking. |
| 5 | Final branch is clean | **PASS** | The candidate worktree was clean before this gate artifact was added. `go build ./...`, `go vet ./...`, `make fmt-check`, `make check-docs`, and `git diff --check` PASSed. |
| 6 | Branch diverges cleanly from main | **PASS** | GitHub commit-to-PR lookup returned no PR. `git merge-tree --write-tree origin/main a2d50d633971c31ddaea7bf619f7e0c3c4be20c5` exited 0 with tree `cabd60638978d71ee63ea0ce9e36b87817706c1a`; no self-rebase was needed. |
| 7 | Single feature theme | **PASS** | The four-commit range is one Dolt-orphan safety theme: two-scan lsof confirmation plus a real NBS flock check before deleting a candidate data directory. |

## Diff-owned tests

Each result below was resolved by name in both the focused run and the
canonical full-suite output:

- `TestSweep_ConfirmsUnheldWithSecondLsofScanBeforeRemoving` — PASS
- `TestSweep_SkipsConfirmScanWhenFirstScanAlreadyHeld` — PASS
- `TestSweep_RemovesWhenBothScansAgreeUnheld` — PASS
- `TestSweep_ConfirmScanErrorFailsClosed` — PASS
- `TestSweep_RespectsRealDoltLockEvenWhenLsofMissesIt` — PASS
- `TestSweep_RemovesWhenDoltLockFileExistsButIsUnheld` — PASS
- `TestSweep_ReapsRealDoltDataDirAfterSIGKILL` — PASS (real-Dolt integration acceptance)

## Other failing-test tracking

The other failure signatures have tracker records that predate the canonical
run, including `ga-umtnqe`, `ga-sxtkmu`, `ga-f0uceo`, `ga-ho7b9l`,
`ga-9vz14c`, `ga-afqddr`, `ga-gajll3`, `ga-81syu4`, and `ga-982j01`.
They are recorded for audit but are not used to rescue criterion 3: the three
post-run trackers above already make the criterion a hard FAIL.

## Logs and disposition

- Focused unit log: `/var/tmp/ga-yxgivi-gate.xko3l5Ni/doltorphan-unit.log`
- Focused acceptance log: `/var/tmp/ga-yxgivi-gate.xko3l5Ni/doltorphan-acceptance.log`
- Canonical runner log: `/var/tmp/ga-yxgivi-gate-r2.g4NbxCHX/full-suite.log`
- Canonical per-job logs: `/var/tmp/gc-local-tests.ReuI4v`
- Audited lint rerun: `/var/tmp/ga-yxgivi-gate-r2.g4NbxCHX/lint-r2.log`

No pull request was created or changed, and no deploy clearance was published.
Route the bead to the builder; a future gate run may consider `ga-ago7nm`,
`ga-i4tsvn`, and `ga-3czldl` only because they will then predate that new run.
