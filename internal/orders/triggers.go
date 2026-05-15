package orders

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/execenv"
)

// TriggerResult holds the outcome of a trigger check.
type TriggerResult struct {
	// Due is true if the trigger condition is satisfied and the order should run.
	Due bool
	// Reason explains why the trigger is or isn't due.
	Reason string
	// LastRun is the last execution time (zero if never run).
	LastRun time.Time
}

// LastRunFunc returns the last run time for a named order.
// Returns zero time and nil error if never run.
type LastRunFunc func(name string) (time.Time, error)

// CursorFunc returns the event cursor (highest seq) for a named order.
// Returns 0 if no cursor exists.
type CursorFunc func(orderName string) uint64

// TriggerOptions carries execution context for triggers that run subprocesses.
type TriggerOptions struct {
	ConditionDir     string
	ConditionEnv     []string
	ConditionTimeout time.Duration
}

// CheckTrigger evaluates an order's trigger condition and returns whether it's due.
// ep is an events Provider used by event triggers to query events; may be nil for
// non-event triggers.
// cursorFn returns the last-processed event seq for event triggers; may be nil for
// non-event triggers.
func CheckTrigger(a Order, now time.Time, lastRunFn LastRunFunc, ep events.Provider, cursorFn CursorFunc) TriggerResult {
	return CheckTriggerWithOptions(a, now, lastRunFn, ep, cursorFn, TriggerOptions{})
}

// CheckTriggerWithOptions evaluates an order trigger using explicit execution
// context for condition checks.
func CheckTriggerWithOptions(a Order, now time.Time, lastRunFn LastRunFunc, ep events.Provider, cursorFn CursorFunc, opts TriggerOptions) TriggerResult {
	switch a.Trigger {
	case "cooldown":
		return checkCooldown(a, now, lastRunFn)
	case "cron":
		return checkCron(a, now, lastRunFn)
	case "condition":
		return checkCondition(a, opts)
	case "event":
		return checkEvent(a, ep, cursorFn)
	case "manual":
		return TriggerResult{Due: false, Reason: "manual trigger — use gc order run"}
	default:
		return TriggerResult{Due: false, Reason: fmt.Sprintf("unknown trigger %q", a.Trigger)}
	}
}

// checkCooldown checks if enough time has elapsed since the last run.
func checkCooldown(a Order, now time.Time, lastRunFn LastRunFunc) TriggerResult {
	interval, err := time.ParseDuration(a.Interval)
	if err != nil {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("bad interval: %v", err)}
	}

	last, err := lastRunFn(a.ScopedName())
	if err != nil {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("error querying last run: %v", err)}
	}

	if last.IsZero() {
		return TriggerResult{Due: true, Reason: "never run", LastRun: last}
	}

	elapsed := now.Sub(last)
	if elapsed >= interval {
		return TriggerResult{
			Due:     true,
			Reason:  fmt.Sprintf("elapsed %s >= interval %s", elapsed.Round(time.Second), interval),
			LastRun: last,
		}
	}

	remaining := interval - elapsed
	return TriggerResult{
		Due:     false,
		Reason:  fmt.Sprintf("cooldown: %s remaining", remaining.Round(time.Second)),
		LastRun: last,
	}
}

// checkCron uses simple minute-granularity matching against the schedule.
// Schedule format: "minute hour day-of-month month day-of-week" (5 fields).
func checkCron(a Order, now time.Time, lastRunFn LastRunFunc) TriggerResult {
	fields := strings.Fields(a.Schedule)
	if len(fields) != 5 {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("invalid cron schedule: want 5 fields, got %d", len(fields))}
	}

	specs := []cronFieldSpec{
		{name: "minute", field: fields[0], value: now.Minute(), min: 0, max: 59},
		{name: "hour", field: fields[1], value: now.Hour(), min: 0, max: 23},
		{name: "day-of-month", field: fields[2], value: now.Day(), min: 1, max: 31},
		{name: "month", field: fields[3], value: int(now.Month()), min: 1, max: 12},
		{name: "day-of-week", field: fields[4], value: int(now.Weekday()), min: 0, max: 6},
	}
	allMatched := true
	var invalid cronFieldSpec
	for _, spec := range specs {
		matched, parseOK := cronFieldMatches(spec.field, spec.value, spec.min, spec.max)
		if !parseOK && invalid.name == "" {
			invalid = spec
		}
		if !matched {
			allMatched = false
		}
	}
	if invalid.name != "" {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("invalid cron schedule: cannot parse %s field %q", invalid.name, invalid.field)}
	}
	if !allMatched {
		return TriggerResult{Due: false, Reason: "cron: schedule not matched"}
	}

	// Schedule matches — check if already run this minute.
	last, err := lastRunFn(a.ScopedName())
	if err != nil {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("error querying last run: %v", err)}
	}
	if !last.IsZero() && last.Truncate(time.Minute).Equal(now.Truncate(time.Minute)) {
		return TriggerResult{Due: false, Reason: "cron: already run this minute", LastRun: last}
	}

	return TriggerResult{Due: true, Reason: "cron: schedule matched", LastRun: last}
}

type cronFieldSpec struct {
	name  string
	field string
	value int
	min   int
	max   int
}

// cronFieldMatches reports whether a single cron field matches a value.
// The second return is false when the field is syntactically invalid.
func cronFieldMatches(field string, value, lower, upper int) (matched, parseOK bool) {
	if value < lower || value > upper {
		return false, false
	}
	for _, part := range strings.Split(field, ",") {
		partMatched, ok := parseCronPart(strings.TrimSpace(part), value, lower, upper)
		if !ok {
			return false, false
		}
		if partMatched {
			matched = true
		}
	}
	return matched, true
}

func parseCronPart(part string, value, lower, upper int) (matched, parseOK bool) {
	if part == "" {
		return false, false
	}
	if part == "*" {
		return true, true
	}

	if base, stepText, hasStep := strings.Cut(part, "/"); hasStep {
		step, err := strconv.Atoi(stepText)
		if err != nil || step <= 0 {
			return false, false
		}
		start, end, ok := cronPartRange(base, lower, upper, true)
		if !ok {
			return false, false
		}
		return value >= start && value <= end && (value-start)%step == 0, true
	}

	if strings.Contains(part, "-") {
		start, end, ok := cronPartRange(part, lower, upper, false)
		if !ok {
			return false, false
		}
		return value >= start && value <= end, true
	}

	n, err := strconv.Atoi(part)
	if err != nil || n < lower || n > upper {
		return false, false
	}
	return n == value, true
}

func cronPartRange(part string, lower, upper int, allowWildcard bool) (start, end int, parseOK bool) {
	if part == "*" {
		if !allowWildcard {
			return 0, 0, false
		}
		return lower, upper, true
	}

	startText, endText, ok := strings.Cut(part, "-")
	if !ok {
		return 0, 0, false
	}
	start, err := strconv.Atoi(startText)
	if err != nil {
		return 0, 0, false
	}
	end, err = strconv.Atoi(endText)
	if err != nil {
		return 0, 0, false
	}
	if start < lower || start > upper || end < lower || end > upper || start > end {
		return 0, 0, false
	}
	return start, end, true
}

// checkCondition runs the check command and returns due if exit code is 0.
// Uses a timeout to prevent hanging check scripts from blocking trigger evaluation.
func checkCondition(a Order, opts TriggerOptions) TriggerResult {
	const triggerCheckTimeout = 10 * time.Second
	timeout := opts.ConditionTimeout
	if timeout <= 0 {
		timeout = triggerCheckTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", a.Check)
	if opts.ConditionDir != "" {
		cmd.Dir = opts.ConditionDir
	}
	cmd.Env = mergeConditionEnv(os.Environ(), opts.ConditionEnv)
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return TriggerResult{Due: false, Reason: fmt.Sprintf("check command timed out after %s", timeout)}
		}
		return TriggerResult{Due: false, Reason: fmt.Sprintf("check command failed: %v", err)}
	}
	return TriggerResult{Due: true, Reason: "condition: check passed (exit 0)"}
}

func mergeConditionEnv(environ, extra []string) []string {
	return execenv.MergeEntries(environ, extra)
}

// checkEvent checks if matching events exist after the last cursor position.
func checkEvent(a Order, ep events.Provider, cursorFn CursorFunc) TriggerResult {
	if ep == nil {
		return TriggerResult{Due: false, Reason: "event: no events provider"}
	}
	var cursor uint64
	if cursorFn != nil {
		cursor = cursorFn(a.ScopedName())
	}

	matched, err := ep.List(events.Filter{
		Type:     a.On,
		AfterSeq: cursor,
	})
	if err != nil {
		return TriggerResult{Due: false, Reason: fmt.Sprintf("event: read error: %v", err)}
	}
	if len(matched) == 0 {
		return TriggerResult{Due: false, Reason: "event: no matching events"}
	}
	return TriggerResult{Due: true, Reason: fmt.Sprintf("event: %d %s event(s)", len(matched), a.On)}
}

// MaxSeqFromLabels extracts the highest seq:<N> value from bead labels.
// Used by CLI callers to compute the event cursor from BdStore results.
func MaxSeqFromLabels(labelSets [][]string) uint64 {
	var maxSeq uint64
	for _, labels := range labelSets {
		for _, l := range labels {
			if strings.HasPrefix(l, "seq:") {
				if n, err := strconv.ParseUint(l[4:], 10, 64); err == nil && n > maxSeq {
					maxSeq = n
				}
			}
		}
	}
	return maxSeq
}
