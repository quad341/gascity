# Release Gate — ga-gdc6 (rollup-ship)

**Bead:** ga-gdc6 — `[ga-d5y] Ship ADR 0002 — dolt store maintenance (rollup)`
**Evaluated:** 2026-04-23
**Result:** **FAIL** — second-pass (gemini) reviews incomplete on every child review bead.

## Source branch
`gc-builder-1-01561d4fb9ea`

## Children + cherry-picks (declared in CHERRY_PICKS)

| Child | Commit | Review bead | Reviewer-1 | Gemini |
|-------|--------|-------------|------------|--------|
| ga-zol | 67cdfb34 | ga-i24 | PASS | **PENDING** (`needs-gemini-review`) |
| ga-8km | 04b929c4 | ga-5tb | PASS | **PENDING** (`needs-gemini-review`) |
| ga-8cq | 51e6581a | ga-0awq | PASS | **PENDING** (`needs-gemini-review`) |
| ga-zoj | 5fd4dbf2 | ga-yhbi | PASS | **PENDING** (`needs-gemini-review`) |
| ga-p5n | 86bc6259 | ga-4xqa | PASS | **PENDING** (`needs-gemini-review`) |
| ga-74d | 4a847942 | ga-0ydz | PASS | **PENDING** (`needs-gemini-review`) |
| ga-zn8 | 81a39f48 | ga-7mah | PASS | **PENDING** (`needs-gemini-review`) |
| ga-sec | f67ed540 | *(no review bead found)* | — | — |
| ga-e3s | cf4d9ba1 | ga-4nh2 | PASS | **PENDING** (`needs-gemini-review`) |

## Criteria results

| # | Criterion | Result |
|---|-----------|--------|
| 1 | Two-pass review complete (each child) | **FAIL** — 8/8 review beads still labeled `needs-gemini-review`; gemini-reviewer has not run on any child |
| 2 | Acceptance criteria met (each child) | not evaluated (gated on #1) |
| 3 | Tests pass on assembled branch | not evaluated (no branch cut) |
| 4 | No high-severity review findings open | not evaluated (gated on #1) |
| 5 | Final branch is clean | not evaluated (no branch cut) |
| 6 | Branch diverges cleanly from main | not evaluated (no branch cut) |

## Observations

- All 8 located review beads (`ga-i24`, `ga-5tb`, `ga-0awq`, `ga-yhbi`, `ga-4xqa`, `ga-0ydz`, `ga-7mah`, `ga-4nh2`) have first-pass `Reviewer verdict: PASS` recorded by `gascity/reviewer`, then end with handoff text along the lines of "Routing to gascity/gemini-reviewer for second-pass review." Each retains the `needs-gemini-review` label and `gc.routed_to=gascity/gemini-reviewer`.
- ga-sec (docs runbook, commit `f67ed540`) does not appear to have a review bead. Builder's close-reason on ga-sec self-asserts the doc was verified against the implementing files, but the two-reviewer gate has not been applied. This needs adjudication: either gate the runbook through review like the rest, or PM should explicitly carve docs-only beads out of the rollup gate.
- The CHERRY_PICKS block in the rollup description was checked structurally but commits were NOT cherry-picked because criterion #1 already FAILed.
- ADR file `docs/adr/0002-dolt-store-maintenance-runbook.md` is noted in the description as "untracked — review needed before merge." That is also unresolved at gate time.

## Action

- **Do NOT push, do NOT open PR.**
- Remove `needs-deploy` from ga-gdc6 so the deployer formula does not keep firing on the same bead.
- Append these findings to ga-gdc6 notes.
- Mail the mayor: rollup arrived prematurely; gemini-reviewer queue must drain first, and ga-sec needs a coverage decision.
- After gemini-reviewer issues PASS verdicts on all 8 review beads (and ga-sec is resolved), re-label `needs-deploy` and re-route to deployer.

## Routing rationale

The rollup is not blocked on the *builder* (commits exist and reviewer-1 PASSed each one); it is blocked on the *gemini-reviewer* completing its independent second-pass and on a docs-coverage decision. Routing back to `ready-to-build` would be misleading, so the gate FAIL clears `needs-deploy` and notifies the mayor instead.
