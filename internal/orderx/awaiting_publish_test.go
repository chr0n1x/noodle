package orderx

import "testing"

func TestAwaitingPublishStatusIsValidAndKept(t *testing.T) {
	if err := ValidateOrderStatus(OrderStatusAwaitingPublish); err != nil {
		t.Fatalf("validate: %v", err)
	}
	of := OrdersFile{Orders: []Order{{
		ID: "42", Status: OrderStatusAwaitingPublish,
		Stages: []Stage{{TaskKey: "execute", Status: StageStatusCompleted}},
	}}}
	got, _ := ApplyLifecycleDefaults(of)
	if got.Orders[0].Status != OrderStatusAwaitingPublish {
		t.Fatalf("status = %q, want %q", got.Orders[0].Status, OrderStatusAwaitingPublish)
	}
}
