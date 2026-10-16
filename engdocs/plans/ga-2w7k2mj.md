# Verification: custom work queries and named-session demand

Root: `ga-2w7k2mj` · Source ruling: `ga-kkaehzk` · Measured: 2026-10-16

The investigator's configured work query cannot serve the row restored by the
detached-orphan sweep. The historical restarts are real, but their specific
demand reason is not recorded in the retained trace. Assigned work existed at
the same time, so these observations do not establish that the restored route
caused the restarts.

The next scope is `ga-j8b1coz`, labelled `needs-architecture` and routed to
`gascity/architect`. It carries the evidence and asks for an isolated reproduction
and a contract ruling before any implementation decision.

## Query verification

The current investigator pack configures an `on_demand` named session with
`wake_mode = "fresh"`, a ten-minute idle timeout, one active session and no custom
`scale_check`. Its work query, with the rig substituted, is:

```sh
gc bd --rig gascity ready \
  --label-any needs-pr-response,needs-investigation \
  --exclude-label hold:mayor,hold:external \
  --exclude-type=step,molecule,convoy --json 2>/dev/null |
  if command -v quiet-window >/dev/null 2>&1; then
    quiet-window claim-filter
  else
    cat
  fi
```

This command exited zero and returned `[]`. The historical command in the
persisted `template_config_snapshot` is the same `gc bd ready` command without
the `quiet-window` suffix. Executed separately, it also exited zero and returned
`[]`; its unsuppressed stderr was retained.

Event sequence 68393286 records `ga-yi96afx` open, unassigned and unrouted at
2026-10-16T21:27:05Z. Sequence 68393416 records the restored
`gc.routed_to=gascity/investigator` at 21:34:42Z. Both snapshots have only the
`source:mayor` label: neither intake label, and no hold. The query selects by
intake label rather than route, so subsequent containment of the route cannot
explain this exclusion. PM did not reroute, claim or otherwise modify the
incident bead to reproduce it.

## Wake and diagnostic evidence

| Check | Observed output | Limit |
|---|---|---|
| Configured and historical queries | Exit 0, `[]` for each | Confirms exclusion; does not execute a scheduler reproduction |
| `gc trace reasons --template gascity/investigator --since 72h` | `retained: 1281`, `start: 3`, `wake: 2` | Detailed start/wake records concern October 14; October 16 has baseline summaries |
| Supervisor log and session events | Idle-kill at 22:11:43Z, woke at 22:12:09Z; idle-kill at 22:28:34Z, woke at 22:28:54Z | No retained `named-demand`, `routed-demand` or `work-query` reason for these starts |
| Actual divergence events, October 14 through collection at October 16 23:53Z | 25 events; none for `gascity/investigator` or session `gm-wisp-vcj7zd` | An absent diagnostic does not establish demand/serve agreement |

The trace query returned 3,849 records, including 1,281 template summaries. The
supervisor log also records `namedWorkReady` matching assigned, in-progress
`ga-ttfs3ie` during the restart interval. This is a competing direct-demand
source. The exact reason for each historical start is **NOT_ENOUGH_INFO**; the
observed interval must not be described as an isolated ten-minute routed-demand
loop.

The controller recorded in these traces runs commit `8e920748be`. The repository
source inspected for this verification is
`30b8ac715e84c80759bbc09b7f733eaf5f4e2aed`; these are different versions. In the
inspected source, the default routed probe is gated on custom `scale_check`,
while `PoolDemandServeRulesForQuery` describes generated Tier-3 rules. Neither
fact alone proves a scheduler defect for an arbitrary custom work query.

The divergence emitter is gated by `GC_SPAWN_ORIGIN=demand` and hook claim-drain
reporting. An idle stop of a named session need not traverse that reporting
path. Architecture must account for that observation boundary when assessing
the missing report.

## Handoff and dependency graph

`ga-2w7k2mj` verification → `ga-j8b1coz` architecture evaluation

The follow-up has a `discovered-from` edge to this verification, rather than a
blocking dependency on an implementation that does not exist. There is one
follow-up scope, no test/build pair and no rollup. No new external tracker issue
was imported. Existing custom-scale-check work (`ga-k3jb5n.1`, `ga-huwqp6` and
triage `ga-xl98dr` for issue #6351) is related context; the investigator has no
custom scale check, so those scopes are not assumed to be equivalent.

The architecture exit is an actual wake reason and served-ID result for an
isolated named singleton with the custom query and a ready routed row lacking
intake labels, with no direct-assignee work, explicit wake or other demand. It
then records whether the core contract is violated and chooses the lowest
appropriate rung. Arbitrary shell query semantics must not be guessed in Go.
If isolated reproduction refutes the defect, close with the evidence and name
any pack/config responsibility. Implementation work follows a confirmed ruling.

The detached-orphan sweep, pack parking recipes and incident containment remain
with their existing scopes under `ga-kkaehzk`.

## Retained evidence

Evidence directory: `/var/tmp/ga-2w7k2mj-pm.UT0Y1i`.

The directory contains both query outputs, exit codes and stderr; `trace.json`;
`raw-investigator-trace.jsonl`; `incident-bead-events.json`;
`investigator-session-events.json`; `supervisor-investigator.txt`;
`supervisor-context.txt`; and `divergence-window-summary.json`. The latter names
the three compressed event archives and live event stream, including their
coverage boundaries. The routed architecture bead carries a self-contained
copy of the findings so it does not depend solely on temporary files.
