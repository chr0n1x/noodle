package ingest

// EventStageRemainingSkipped marks every non-terminal stage of an order from
// stage_index on as skipped.
const EventStageRemainingSkipped EventType = "stage_remaining_skipped"

// EventStageSkipped marks the stage at stage_index as skipped and leaves
// later stages to run.
const EventStageSkipped EventType = "stage_skipped"

func init() {
	knownEventTypeSet[EventStageRemainingSkipped] = struct{}{}
	knownEventTypeSet[EventStageSkipped] = struct{}{}
}
