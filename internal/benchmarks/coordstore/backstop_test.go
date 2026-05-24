package coordstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gastownhall/gascity/internal/benchmarks/coordstore"
)

func TestCheckBackstopReclaimPassesAtThreshold(t *testing.T) {
	reclaimer := &fakeBackstopReclaimer{
		reclaim: coordstore.BackstopReclaim{Main: 3, Ephemeral: 2},
	}
	policy := coordstore.BackstopPolicy{
		LeakThreshold:   5,
		MainCutoff:      24 * time.Hour,
		EphemeralCutoff: 10 * time.Minute,
	}

	check, err := coordstore.CheckBackstopReclaim(context.Background(), reclaimer, policy)
	if err != nil {
		t.Fatalf("CheckBackstopReclaim: %v", err)
	}
	if !check.Passed() {
		t.Fatalf("Passed() = false, want true: %s", check.FailureReason())
	}
	if reclaimer.mainCutoff != policy.MainCutoff || reclaimer.ephemeralCutoff != policy.EphemeralCutoff {
		t.Fatalf("cutoffs = main:%s ephemeral:%s, want main:%s ephemeral:%s",
			reclaimer.mainCutoff, reclaimer.ephemeralCutoff, policy.MainCutoff, policy.EphemeralCutoff)
	}
}

func TestBackstopCheckFailureReasonIncludesTierCounts(t *testing.T) {
	check := coordstore.BackstopCheck{
		Policy:  coordstore.BackstopPolicy{LeakThreshold: 5},
		Reclaim: coordstore.BackstopReclaim{Main: 6, Ephemeral: 2},
	}

	if check.Passed() {
		t.Fatalf("Passed() = true, want false")
	}
	reason := check.FailureReason()
	for _, want := range []string{"total=8", "threshold=5", "main=6", "ephemeral=2"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("FailureReason() = %q, want substring %q", reason, want)
		}
	}
}

func TestCheckBackstopReclaimReturnsReclaimerErrors(t *testing.T) {
	wantErr := errors.New("sweep unavailable")
	_, err := coordstore.CheckBackstopReclaim(context.Background(), &fakeBackstopReclaimer{err: wantErr}, coordstore.BackstopPolicy{
		LeakThreshold:   1,
		MainCutoff:      time.Hour,
		EphemeralCutoff: time.Hour,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CheckBackstopReclaim error = %v, want %v", err, wantErr)
	}
}

type fakeBackstopReclaimer struct {
	reclaim         coordstore.BackstopReclaim
	err             error
	mainCutoff      time.Duration
	ephemeralCutoff time.Duration
}

func (f *fakeBackstopReclaimer) PurgeBackstop(_ context.Context, mainCutoff, ephemeralCutoff time.Duration) (coordstore.BackstopReclaim, error) {
	f.mainCutoff = mainCutoff
	f.ephemeralCutoff = ephemeralCutoff
	return f.reclaim, f.err
}
