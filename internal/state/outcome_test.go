package state

import "testing"

func TestUserState(t *testing.T) {
	reject := []StageOutcome{{State: "accept", Action: "retry"}, {State: "reject", Action: "promote"}}
	cases := []struct {
		status   string
		outcomes []StageOutcome
		want     string
	}{
		{"pending", nil, "pending"},
		{"dispatching", nil, "in-progress"},
		{"running", nil, "in-progress"},
		{"active", nil, "in-progress"},
		{"merging", nil, "in-progress"},
		{"review", reject, "in-progress"},
		{"completed", nil, "accept"},
		{"completed", reject, "reject"},
		{"failed", nil, "reject"},
		{"skipped", nil, "skipped"},
		{"cancelled", nil, "skipped"},
	}
	for _, c := range cases {
		if got := UserState(c.status, c.outcomes); got != c.want {
			t.Errorf("UserState(%q, %d outcomes) = %q, want %q", c.status, len(c.outcomes), got, c.want)
		}
	}
}
