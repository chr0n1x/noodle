package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/mise"
)

func TestPromotionKeepsOrderURL(t *testing.T) {
	logger, _ := newTestLogger()
	tc := newTestLoop(t, logger, func(o *testLoopOpts) {
		brief := mise.Brief{Backlog: []adapter.BacklogItem{{ID: "1", Title: "test", Status: "open"}}}
		o.brief = &brief
	})
	ordersNextPath := filepath.Join(tc.runtimeDir, "orders-next.json")
	compactJSON := []byte(`{"orders":[{"id":"o1","title":"PROJ-1 fix","url":"https://jira.example.com/browse/PROJ-1","stages":[{"do":"execute","with":"claude","model":"m"}]}]}`)
	if err := os.WriteFile(ordersNextPath, compactJSON, 0o644); err != nil {
		t.Fatalf("write orders-next: %v", err)
	}
	tc.loop.deps.OrdersNextFile = ordersNextPath

	if err := tc.loop.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	orders, err := tc.loop.currentOrders()
	if err != nil {
		t.Fatalf("orders: %v", err)
	}
	if len(orders.Orders) != 1 {
		t.Fatalf("orders = %d", len(orders.Orders))
	}
	if got := orders.Orders[0].URL; got != "https://jira.example.com/browse/PROJ-1" {
		t.Fatalf("URL after promotion = %q", got)
	}
	node, ok := tc.loop.canonical.Orders["o1"]
	if !ok {
		t.Fatalf("canonical order o1 missing")
	}
	if got := node.Metadata["url"]; got != "https://jira.example.com/browse/PROJ-1" {
		t.Fatalf("canonical url metadata = %q", got)
	}
}
