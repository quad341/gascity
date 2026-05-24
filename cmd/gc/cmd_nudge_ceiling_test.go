package main

import (
	"errors"
	"testing"
	"time"
)

// TestDefaultQueuedNudgeMaxAttempts_Is50 verifies that the queued nudge
// dead-letter ceiling is 50. Pool agents with asleep-target loops can easily
// exhaust the original limit of 5 over a short period; 50 provides adequate
// durability across typical wake cycles.
//
// This test FAILS on current code: defaultQueuedNudgeMaxAttempts = 5.
func TestDefaultQueuedNudgeMaxAttempts_Is50(t *testing.T) {
	if defaultQueuedNudgeMaxAttempts != 50 {
		t.Errorf("defaultQueuedNudgeMaxAttempts = %d, want 50 — ceiling too low for asleep targets", defaultQueuedNudgeMaxAttempts)
	}
}

// TestQueuedNudgeSurvivesFormerFiveAttemptCeiling verifies that a queued nudge
// is still alive (not dead-lettered) after 5 failed delivery attempts. Under
// the new ceiling of 50, the nudge must remain pending for 45 more attempts.
//
// This test FAILS on current code: with ceiling=5, item.Attempts reaches 5 on
// the fifth call and the dead-letter condition (Attempts >= 5) fires immediately.
func TestQueuedNudgeSurvivesFormerFiveAttemptCeiling(t *testing.T) {
	now := time.Now()
	item := newQueuedNudge("agent-a", "pending message", "sling", now)
	cause := errors.New("delivery failed")

	var dead bool
	for i := 0; i < 5; i++ {
		item, dead = failedQueuedNudge(item, cause, now)
	}

	if dead {
		t.Errorf("nudge dead-lettered at attempts=%d, want alive — 50-attempt ceiling means death only at attempt 50", item.Attempts)
	}
}
