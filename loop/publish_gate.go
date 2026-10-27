package loop

import (
	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/mode"
)

// LoopEventOrderAwaitingPublish reports a gated order whose stages all
// finished; its work stays on kept branches until a human publishes it.
const LoopEventOrderAwaitingPublish event.LoopEventType = "order.awaiting_publish"

type OrderAwaitingPublishPayload struct {
	OrderID  string   `json:"order_id"`
	Branches []string `json:"branches,omitempty"`
}

// orderGated reports whether orders use the gated publish model: stages keep
// their work instead of merging, and the order publishes once at the end.
// An order is gated iff len(cfg.Review.ExternalStages) > 0 OR
// strings.TrimSpace(cfg.Review.PublishAction) != "". It is config-level, so
// every order of a project is gated or none is; the run mode never gates.
func (l *Loop) orderGated() bool {
	return l.config.Review.Gated()
}

func (l *Loop) noteAwaitingPublish(orderID string) {
	_ = l.events.Emit(LoopEventOrderAwaitingPublish, OrderAwaitingPublishPayload{
		OrderID:  orderID,
		Branches: l.loadOrderWork()[orderID].Branches,
	})
}

// autoPublish publishes an order that just became awaiting-publish when the
// mode allows auto merge, through the same path as approve-and-publish. A
// failure leaves it awaiting-publish with the error recorded.
func (l *Loop) autoPublish(orderID string) {
	if !(mode.ModeGate{}).CanAutoMerge(l.canonical.Mode) {
		return
	}
	ack := l.cmds.ackMessage
	defer func() { l.cmds.ackMessage = ack }()
	if err := l.controlApproveAndPublish(orderID); err != nil {
		if _, recorded := ReadPublishErrors(l.runtimeDir)[orderID]; !recorded {
			_ = l.publishFailed(orderID, err)
		}
	}
}
