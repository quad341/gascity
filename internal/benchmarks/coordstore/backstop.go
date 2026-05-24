package coordstore

import (
	"context"
	"fmt"
	"time"
)

// BackstopReclaim records how many stale closed records a single backstop
// sweep reclaimed from each physical tier.
type BackstopReclaim struct {
	Main      int
	Ephemeral int
}

// Total returns the aggregate reclaim count across all tiers.
func (r BackstopReclaim) Total() int {
	return r.Main + r.Ephemeral
}

// BackstopPolicy configures a single benchmark backstop sweep assertion.
type BackstopPolicy struct {
	LeakThreshold   int
	MainCutoff      time.Duration
	EphemeralCutoff time.Duration
}

// Enabled reports whether the policy has enough data to run the assertion.
func (p BackstopPolicy) Enabled() bool {
	return p.LeakThreshold > 0 && p.MainCutoff > 0 && p.EphemeralCutoff > 0
}

// BackstopReclaimer is implemented by adapters that can run their closed-record
// backstop sweep and report per-tier reclaim counts.
type BackstopReclaimer interface {
	PurgeBackstop(ctx context.Context, mainCutoff, ephemeralCutoff time.Duration) (BackstopReclaim, error)
}

// BackstopCheck captures one post-warm-up backstop assertion result.
type BackstopCheck struct {
	Policy  BackstopPolicy
	Reclaim BackstopReclaim
}

// Passed reports whether a single backstop sweep stayed within threshold.
func (c BackstopCheck) Passed() bool {
	return c.Reclaim.Total() <= c.Policy.LeakThreshold
}

// FailureReason formats the threshold failure with per-tier counts.
func (c BackstopCheck) FailureReason() string {
	return fmt.Sprintf("backstop reclaim total=%d > threshold=%d per tick (main=%d ephemeral=%d)",
		c.Reclaim.Total(), c.Policy.LeakThreshold, c.Reclaim.Main, c.Reclaim.Ephemeral)
}

// CheckBackstopReclaim runs one adapter backstop sweep and returns its bounded
// reclaim assertion result.
func CheckBackstopReclaim(ctx context.Context, reclaimer BackstopReclaimer, policy BackstopPolicy) (BackstopCheck, error) {
	reclaim, err := reclaimer.PurgeBackstop(ctx, policy.MainCutoff, policy.EphemeralCutoff)
	if err != nil {
		return BackstopCheck{}, err
	}
	return BackstopCheck{Policy: policy, Reclaim: reclaim}, nil
}
