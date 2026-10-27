package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/state"
)

func approveAndPublish(orderID string) ControlCommand {
	return ControlCommand{ID: "pub-" + orderID, Action: "approve-and-publish", OrderID: orderID}
}

// newAwaitingPublishLoop starts a loop whose order 42 already finished its
// stages and waits for publish, with kept holding its kept branches.
func newAwaitingPublishLoop(t *testing.T, publishAction string, kept ...string) gateTestLoop {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	ordersPath := writeTestOrders(t, runtimeDir, OrdersFile{Orders: []Order{{
		ID: "42", Title: "gate me", Rationale: "because", Status: OrderStatusAwaitingPublish,
		Stages: []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusCompleted},
			{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusCompleted},
		},
	}}})
	cfg := config.DefaultConfig()
	cfg.Mode = "supervised"
	cfg.Review.PublishAction = publishAction
	g := gateTestLoop{rt: newMockRuntime(), wt: &fakeWorktree{}, ar: &fakeAdapterRunner{}}
	g.l = New(projectDir, "noodle", cfg, g.deps(ordersPath))
	for _, b := range kept {
		if err := g.l.keepOrderWork("42", b); err != nil {
			t.Fatalf("keep work: %v", err)
		}
	}
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(g.rt.calls) != 0 {
		t.Fatalf("awaiting-publish order dispatched: %+v", g.rt.calls)
	}
	return g
}

// withPublishSkill adds name as a plain skill next to the order's task-type
// skills, since dispatching a skill outside the registry rediscovers it.
func withPublishSkill(t *testing.T, g gateTestLoop, name string) {
	t.Helper()
	dir := filepath.Join(g.l.projectDir, "skills")
	write := func(skillName, frontmatter string) {
		if err := os.MkdirAll(filepath.Join(dir, skillName), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + skillName + "\n" + frontmatter + "---\nbody\n"
		if err := os.WriteFile(filepath.Join(dir, skillName, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(name, "")
	for _, taskType := range []string{"execute", "review"} {
		write(taskType, "schedule: test\n")
	}
	g.l.config.Skills.Paths = []string{dir}
}

type publishGuardState struct {
	orders OrdersFile
	status state.OrderLifecycleStatus
	work   map[string]orderWorkRecord
}

func capturePublishState(t *testing.T, l *Loop) publishGuardState {
	t.Helper()
	orders, err := l.currentOrders()
	if err != nil {
		t.Fatalf("orders: %v", err)
	}
	return publishGuardState{orders: orders, status: l.canonical.Orders["42"].Status, work: l.loadOrderWork()}
}

func assertPublishRefused(t *testing.T, g gateTestLoop, orderID string, want ...string) {
	t.Helper()
	before := capturePublishState(t, g.l)
	ack := g.l.applyControlCommand(approveAndPublish(orderID))
	if ack.Status != "error" {
		t.Fatalf("ack = %+v, want error", ack)
	}
	for _, w := range want {
		if !strings.Contains(ack.Message, w) {
			t.Fatalf("ack message = %q, want %q", ack.Message, w)
		}
	}
	if after := capturePublishState(t, g.l); !reflect.DeepEqual(before, after) {
		t.Fatalf("state changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if len(g.wt.merged) != 0 || len(g.ar.doneCalls) != 0 {
		t.Fatalf("refused publish acted: merged=%v done=%v", g.wt.merged, g.ar.doneCalls)
	}
}

func assertPublishedCompleted(t *testing.T, g gateTestLoop, kept ...string) {
	t.Helper()
	orders, err := readOrders(g.l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 0 {
		t.Fatalf("published order should leave orders.json: %+v", orders.Orders)
	}
	if _, ok := g.l.canonical.Orders["42"]; ok && g.l.canonical.Orders["42"].Status != state.OrderCompleted {
		t.Fatalf("canonical status = %q", g.l.canonical.Orders["42"].Status)
	}
	if !strings.Contains(g.loopEvents(t), `"order.completed"`) {
		t.Fatalf("missing order.completed:\n%s", g.loopEvents(t))
	}
	if !slices.Equal(g.ar.doneCalls, []string{"42"}) {
		t.Fatalf("done calls = %v, want [42]", g.ar.doneCalls)
	}
	if _, ok := g.l.loadOrderWork()["42"]; ok {
		t.Fatal("kept work not released")
	}
	for _, b := range kept {
		if !slices.Contains(g.wt.cleaned, b) {
			t.Fatalf("kept branch %s not cleaned (cleaned=%v)", b, g.wt.cleaned)
		}
	}
	if msg, ok := ReadPublishErrors(g.l.runtimeDir)["42"]; ok {
		t.Fatalf("publish error left after success: %q", msg)
	}
}

func assertStillAwaitingPublish(t *testing.T, g gateTestLoop, wantErr string) {
	t.Helper()
	if got := g.l.canonical.Orders["42"].Status; got != state.OrderAwaitingPublish {
		t.Fatalf("canonical status = %q, want awaiting-publish", got)
	}
	orders, err := readOrders(g.l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusAwaitingPublish {
		t.Fatalf("orders.json = %+v, want order 42 awaiting-publish", orders.Orders)
	}
	if strings.Contains(g.loopEvents(t), `"order.completed"`) || len(g.ar.doneCalls) != 0 {
		t.Fatalf("failed publish completed the order: done=%v", g.ar.doneCalls)
	}
	if msg := ReadPublishErrors(g.l.runtimeDir)["42"]; !strings.Contains(msg, wantErr) {
		t.Fatalf("publish error = %q, want %q", msg, wantErr)
	}
}

func TestApproveAndPublishRefusedWhileStagesIncomplete(t *testing.T) {
	g := newGateTestLoop(t, "supervised", nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
	)
	assertPublishRefused(t, g, "42", "not awaiting publish", "1:quality:pending")
}

func TestApproveAndPublishRefusedWithExternalHold(t *testing.T) {
	g := newGateTestLoop(t, "auto", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	holdExternal(t, g.l)
	assertPublishRefused(t, g, "42", "noodle order 42 complete review", "1:review:")
}

func TestApproveAndPublishRefusedForUnknownOrder(t *testing.T) {
	g := newAwaitingPublishLoop(t, "", "42-0-execute")
	assertPublishRefused(t, g, "99", "order 99 not found")
}

func TestApproveAndPublishMergesNewestKeptBranch(t *testing.T) {
	g := newGateTestLoop(t, "supervised", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	if err := g.l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	holdExternal(t, g.l)
	if err := g.l.dispatchControlCommand(stageComplete("review", "accept", "looks good")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	g.assertAwaitingPublish(t, "42-0-execute", "42-1-review")

	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if !slices.Equal(g.wt.merged, []string{"42-1-review"}) {
		t.Fatalf("merged = %v, want [42-1-review]", g.wt.merged)
	}
	assertPublishedCompleted(t, g, "42-0-execute", "42-1-review")
}

func TestApproveAndPublishWithoutKeptWorkCompletes(t *testing.T) {
	g := newAwaitingPublishLoop(t, "")
	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("merged = %v, want none", g.wt.merged)
	}
	assertPublishedCompleted(t, g)
}

func TestApproveAndPublishMergeFailureStaysAwaiting(t *testing.T) {
	g := newAwaitingPublishLoop(t, "", "42-0-execute")
	g.wt.mergeErr = errors.New("rebase conflict")
	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "error" || !strings.Contains(ack.Message, "rebase conflict") {
		t.Fatalf("ack = %+v", ack)
	}
	assertStillAwaitingPublish(t, g, "rebase conflict")
	if _, ok := g.l.loadOrderWork()["42"]; !ok {
		t.Fatal("failed merge released kept work")
	}

	g.wt.mergeErr = nil
	if ack := g.l.applyControlCommand(ControlCommand{ID: "retry", Action: "approve-and-publish", OrderID: "42"}); ack.Status != "ok" {
		t.Fatalf("retry ack = %+v", ack)
	}
	assertPublishedCompleted(t, g, "42-0-execute")
}

func TestApproveAndPublishDraftPR(t *testing.T) {
	g := newAwaitingPublishLoop(t, "draft-pr", "42-0-execute", "42-1-review")
	pr := &fakeDraftPR{}
	g.l.deps.DraftPR = pr
	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "ok" || !strings.Contains(ack.Message, "https://github.com/acme/widgets/pull/7") {
		t.Fatalf("ack = %+v", ack)
	}
	if len(pr.requests) != 1 {
		t.Fatalf("publish requests = %+v", pr.requests)
	}
	if req := pr.requests[0]; req.OrderID != "42" || req.Branch != "42-1-review" || req.Title != "gate me" || req.Body != "because" {
		t.Fatalf("request = %+v", req)
	}
	if link := ReadDraftPRs(g.l.runtimeDir)["42"]; link.URL != "https://github.com/acme/widgets/pull/7" || link.Branch != "noodle/42" {
		t.Fatalf("draft PR record = %+v", link)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("draft-pr publish merged locally: %v", g.wt.merged)
	}
	assertPublishedCompleted(t, g, "42-0-execute", "42-1-review")
}

func TestApproveAndPublishDraftPRErrorStaysAwaiting(t *testing.T) {
	g := newAwaitingPublishLoop(t, "draft-pr", "42-0-execute")
	g.l.deps.DraftPR = &fakeDraftPR{err: errors.New("push rejected")}
	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "error" || !strings.Contains(ack.Message, "push rejected") {
		t.Fatalf("ack = %+v", ack)
	}
	assertStillAwaitingPublish(t, g, "push rejected")
}

func TestApproveAndPublishDraftPRWithoutPublisherStaysAwaiting(t *testing.T) {
	g := newAwaitingPublishLoop(t, "draft-pr", "42-0-execute")
	g.l.deps.DraftPR = nil
	ack := g.l.applyControlCommand(approveAndPublish("42"))
	if ack.Status != "error" || !strings.Contains(ack.Message, "[delivery]") {
		t.Fatalf("ack = %+v", ack)
	}
	assertStillAwaitingPublish(t, g, "[delivery]")
}

func TestApproveAndPublishUnknownSkillRefused(t *testing.T) {
	g := newAwaitingPublishLoop(t, "ship-it", "42-0-execute")
	g.l.config.Skills.Paths = []string{filepath.Join(g.l.projectDir, "skills")}
	assertPublishRefused(t, g, "42", "publish_action skill ship-it is not a registered skill")
}

func publishStages(t *testing.T, l *Loop) []Stage {
	t.Helper()
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	var out []Stage
	for _, o := range orders.Orders {
		for _, s := range o.Stages {
			if isPublishStage(s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// startSkillPublish approves order 42 for skill publish and dispatches the
// publish stage, returning its session index.
func startSkillPublish(t *testing.T, g gateTestLoop) int {
	t.Helper()
	ack := g.l.applyControlCommand(ControlCommand{ID: "pub-" + string(rune('a'+len(g.rt.sessions))), Action: "approve-and-publish", OrderID: "42"})
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if got := g.l.canonical.Orders["42"].Status; got != state.OrderActive {
		t.Fatalf("canonical status = %q, want active", got)
	}
	stages := publishStages(t, g.l)
	if len(stages) != 1 || stages[0].Skill != "ship-it" || stages[0].Status != StageStatusPending {
		t.Fatalf("publish stages = %+v", stages)
	}
	if !strings.Contains(stages[0].Prompt, "42-1-review") || !strings.Contains(stages[0].Prompt, "gate me") {
		t.Fatalf("publish prompt = %q", stages[0].Prompt)
	}
	before := len(g.rt.sessions)
	createdBefore := len(g.wt.created)
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("dispatch cycle: %v", err)
	}
	if len(g.rt.sessions) != before+1 || g.rt.calls[len(g.rt.calls)-1].Skill != "ship-it" {
		t.Fatalf("publish stage not dispatched: %+v", g.rt.calls)
	}
	if len(g.wt.created) != createdBefore+1 || g.wt.createOpts[createdBefore].From != "42-1-review" {
		t.Fatalf("publish worktree created=%v opts=%+v, want from 42-1-review", g.wt.created, g.wt.createOpts)
	}
	return before
}

func TestApproveAndPublishSkillCompletes(t *testing.T) {
	g := newAwaitingPublishLoop(t, "ship-it", "42-0-execute", "42-1-review")
	withPublishSkill(t, g, "ship-it")
	idx := startSkillPublish(t, g)
	assertPublishRefused(t, g, "42", "publish already running")

	g.rt.sessions[idx].complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("publish stage merged: %v", g.wt.merged)
	}
	if strings.Contains(g.loopEvents(t), `"order.awaiting_publish"`) {
		t.Fatal("publish stage was gated into awaiting-publish again")
	}
	assertPublishedCompleted(t, g, "42-0-execute", "42-1-review")
}

func TestApproveAndPublishSkillFailureReturnsToAwaiting(t *testing.T) {
	g := newAwaitingPublishLoop(t, "ship-it", "42-0-execute", "42-1-review")
	withPublishSkill(t, g, "ship-it")
	idx := startSkillPublish(t, g)

	g.rt.sessions[idx].complete("failed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
	assertStillAwaitingPublish(t, g, "failed")
	if !strings.Contains(g.loopEvents(t), `"order.publish_failed"`) {
		t.Fatalf("missing order.publish_failed:\n%s", g.loopEvents(t))
	}
	if stages := publishStages(t, g.l); len(stages) != 0 {
		t.Fatalf("failed publish stage left on order: %+v", stages)
	}
	if _, ok := g.l.loadOrderWork()["42"]; !ok {
		t.Fatal("failed publish released kept work")
	}

	startSkillPublish(t, g)
	if msg, ok := ReadPublishErrors(g.l.runtimeDir)["42"]; ok {
		t.Fatalf("retry left publish error %q", msg)
	}
}

func TestApproveAndPublishSkillDispatchFailureReturnsToAwaiting(t *testing.T) {
	g := newAwaitingPublishLoop(t, "ship-it", "42-0-execute", "42-1-review")
	withPublishSkill(t, g, "ship-it")
	ack := g.l.applyControlCommand(ControlCommand{ID: "pub-x", Action: "approve-and-publish", OrderID: "42"})
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	g.rt.dispatchErr = errors.New("spawn failed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("dispatch cycle: %v", err)
	}
	assertStillAwaitingPublish(t, g, "spawn failed")
	if stages := publishStages(t, g.l); len(stages) != 0 {
		t.Fatalf("failed publish stage left on order: %+v", stages)
	}
	if _, ok := g.l.loadOrderWork()["42"]; !ok {
		t.Fatal("failed publish released kept work")
	}
}
