package ingest

// EventStageRemainingSkipped marks every non-terminal stage of an order from
// stage_index on as skipped.
const EventStageRemainingSkipped EventType = "stage_remaining_skipped"

func init() {
	knownEventTypeSet[EventStageRemainingSkipped] = struct{}{}
}
