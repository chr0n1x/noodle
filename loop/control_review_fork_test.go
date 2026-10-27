package loop

import (
	"strings"
	"testing"
)

func TestControlRequestChangesAtMaxConcurrencyReturnsError(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	l.config.Concurrency.MaxConcurrency = 1
	l.cooks.activeCooksByOrder["7"] = &cookHandle{cookIdentity: cookIdentity{orderID: "7"}}

	err := l.controlRequestChanges("42", "Add missing tests")
	if err == nil || !strings.Contains(err.Error(), "max concurrency") {
		t.Fatalf("err = %v, want a max concurrency error", err)
	}
	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("pending review must stay parked when request-changes cannot run")
	}
}
