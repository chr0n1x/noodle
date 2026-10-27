package loop

import (
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	loopruntime "github.com/poteto/noodle/runtime"
)

func newConcurrencyTestLoop(t *testing.T, excludeScheduler bool) *Loop {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Concurrency.MaxConcurrency = 1
	cfg.Concurrency.ExcludeScheduler = excludeScheduler
	l := New(t.TempDir(), "noodle", cfg, Dependencies{
		Runtimes: map[string]loopruntime.Runtime{"process": newMockRuntime()},
		Worktree: &fakeWorktree{},
		Adapter:  &fakeAdapterRunner{},
		Mise:     &fakeMise{},
		Monitor:  fakeMonitor{},
		Registry: testLoopRegistry(),
		Now:      time.Now,
	})
	l.cooks.activeCooksByOrder[scheduleOrderID] = &cookHandle{
		cookIdentity: cookIdentity{orderID: scheduleOrderID},
	}
	return l
}

func TestAtMaxConcurrencyCountsSchedulerByDefault(t *testing.T) {
	l := newConcurrencyTestLoop(t, false)
	if !l.atMaxConcurrency() {
		t.Fatal("by default the scheduler counts toward max_concurrency")
	}
}

func TestAtMaxConcurrencyExcludeScheduler(t *testing.T) {
	l := newConcurrencyTestLoop(t, true)
	if l.atMaxConcurrency() {
		t.Fatal("with exclude_scheduler a running scheduler used up the only cook slot")
	}

	l.cooks.activeCooksByOrder["1"] = &cookHandle{cookIdentity: cookIdentity{orderID: "1"}}
	if !l.atMaxConcurrency() {
		t.Fatal("one work cook with max_concurrency = 1 should be at max")
	}
}
