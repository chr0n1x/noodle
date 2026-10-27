package server

import "testing"

func TestParseControlRequestAcceptsApprove(t *testing.T) {
	cmd, err := parseControlRequest("approve", controlRequest{Action: "approve", OrderID: " 42 "})
	if err != nil {
		t.Fatalf("parseControlRequest: %v", err)
	}
	if cmd.Action != "approve" || cmd.OrderID != "42" {
		t.Fatalf("cmd = %+v", cmd)
	}
}

func TestParseControlRequestAcceptsStageComplete(t *testing.T) {
	cmd, err := parseControlRequest("stage-complete", controlRequest{Action: "stage-complete", OrderID: "42", TaskKey: " quality ", Prompt: "looks good", Value: "reject"})
	if err != nil {
		t.Fatalf("parseControlRequest: %v", err)
	}
	if cmd.Action != "stage-complete" || cmd.OrderID != "42" || cmd.TaskKey != "quality" || cmd.Prompt != "looks good" || cmd.Value != "reject" {
		t.Fatalf("cmd = %+v", cmd)
	}
}

func TestParseControlRequestRejectsRemovedReviewActions(t *testing.T) {
	for _, action := range []string{"review-complete", "quality-review-complete", "adversarial-review-complete", "draft-pr"} {
		if _, err := parseControlRequest(action, controlRequest{Action: action, OrderID: "42"}); err == nil || err.Error() != "unknown action: "+action {
			t.Fatalf("%s: err = %v, want unknown action", action, err)
		}
	}
}

func TestParseControlRequestAcceptsMerge(t *testing.T) {
	cmd, err := parseControlRequest("merge", controlRequest{Action: "merge", OrderID: " 42 "})
	if err != nil {
		t.Fatalf("parseControlRequest: %v", err)
	}
	if cmd.Action != "merge" || cmd.OrderID != "42" {
		t.Fatalf("cmd = %+v", cmd)
	}
}

func TestParseControlRequestAcceptsApproveAndPublish(t *testing.T) {
	cmd, err := parseControlRequest("approve-and-publish", controlRequest{Action: "approve-and-publish", OrderID: " 42 "})
	if err != nil {
		t.Fatalf("parseControlRequest: %v", err)
	}
	if cmd.Action != "approve-and-publish" || cmd.OrderID != "42" {
		t.Fatalf("cmd = %+v", cmd)
	}
}
