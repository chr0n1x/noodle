package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poteto/noodle/internal/ingest"
)

const externalReviewReasonPrefix = "external review"

func externalReviewPromptPath(runtimeDir, orderID string, stageIndex int) string {
	return filepath.Join(runtimeDir, "reviews", fmt.Sprintf("%s-%d.prompt.md", orderID, stageIndex))
}

// externalSkillWording points an outside reviewer at the skill file on disk,
// since it cannot load .agents/skills through a Skill tool.
func (l *Loop) externalSkillWording(prompt, skill string) string {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return prompt
	}
	for _, dir := range l.config.Skills.Paths {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(l.projectDir, dir)
		}
		path := filepath.Join(dir, skill, "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			return strings.Replace(prompt,
				fmt.Sprintf("Use Skill(%s) for methodology.", skill),
				fmt.Sprintf("Read and follow the %s skill at %s for methodology.", skill, path),
				1)
		}
	}
	return prompt
}

// holdExternalReview parks a stage configured in [review] external_stages as
// a pending review instead of dispatching it, leaving the built prompt on disk
// for an outside reviewer. Returns handled=false when the stage is not external.
func (l *Loop) holdExternalReview(cand dispatchCandidate, order Order, name, worktreePath, prompt string) (bool, error) {
	stage := cand.Stage
	if !l.config.Review.IsExternal(stage.TaskKey, stage.Skill) || isAutoQuality(stage) {
		return false, nil
	}
	promptPath := externalReviewPromptPath(l.runtimeDir, cand.OrderID, cand.StageIndex)
	reviewPath := filepath.Join(l.runtimeDir, "reviews", fmt.Sprintf("%s-%d.review.md", cand.OrderID, cand.StageIndex))
	body := l.externalSkillWording(prompt, stage.Skill) + fmt.Sprintf(`

## External review

Review the changes in the worktree at %s.
Write your review as markdown to a file, for example %s.
When done, run this from the project root (%s) to accept the stage:

    %s --file %s

Add --reject to send the review back to the scheduler as blocking feedback.
Use this command instead of `+"`noodle event emit`"+` (stage_message) if the skill mentions it.
`, worktreePath, reviewPath, l.projectDir, stageCompleteCLI(cand.OrderID, stage.TaskKey), reviewPath)
	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		return true, fmt.Errorf("create reviews dir: %w", err)
	}
	if err := os.WriteFile(promptPath, []byte(body), 0o644); err != nil {
		return true, fmt.Errorf("write review prompt: %w", err)
	}
	cook := &cookHandle{
		cookIdentity: cookIdentity{
			orderID:    cand.OrderID,
			stageIndex: cand.StageIndex,
			stage:      stage,
			plan:       order.Plan,
		},
		orderStatus:  order.Status,
		worktreeName: name,
		worktreePath: worktreePath,
		session:      &adoptedSession{id: fmt.Sprintf("external-%s-%d", cand.OrderID, cand.StageIndex), status: "completed"},
	}
	reason := fmt.Sprintf("%s: run /noodle-review or `noodle review prompt %s`", externalReviewReasonPrefix, cand.OrderID) +
		l.previousStageReport(cand)
	l.logger.Info("stage held for external review", "order", cand.OrderID, "stage", cand.StageIndex, "prompt", promptPath)
	return true, l.parkPendingReview(cook, reason)
}

// externalReviewMaxReportRunes caps the previous stage's message shown in a
// hold's reason.
const externalReviewMaxReportRunes = 4000

// previousStageReport returns the stage_yield message of the stage before cand,
// formatted to follow the hold's reason line, or "" when there is no such
// stage or message. A failed lookup never blocks the hold.
func (l *Loop) previousStageReport(cand dispatchCandidate) string {
	if cand.StageIndex < 1 {
		return ""
	}
	if err := l.ensureCanonicalOrderFromOrders(cand.OrderID); err != nil {
		l.logger.Warn("previous stage report unavailable", "order", cand.OrderID, "error", err)
		return ""
	}
	order, ok := l.canonical.Orders[cand.OrderID]
	if !ok || cand.StageIndex > len(order.Stages) {
		return ""
	}
	prev := order.Stages[cand.StageIndex-1]
	if len(prev.Attempts) == 0 {
		return ""
	}
	yield := l.readStageYield(prev.Attempts[len(prev.Attempts)-1].SessionID)
	if yield == nil {
		return ""
	}
	message := strings.TrimSpace(yield.Message)
	if message == "" {
		return ""
	}
	if runes := []rune(message); len(runes) > externalReviewMaxReportRunes {
		message = string(runes[:externalReviewMaxReportRunes]) + "..."
	}
	return fmt.Sprintf("\n\nPrevious stage (%s) reported:\n%s", prev.TaskKey, message)
}

// controlStageComplete resolves the stage an order is held at for external
// review. stage must equal the held stage's task key: names repeat after
// rework, so the name is checked against the hold rather than used to find a
// stage. Accept advances the stage without publishing and keeps the work;
// reject queues rework, or re-parks the stage at the rework limit.
func (l *Loop) controlStageComplete(orderID, stage, review string, reject bool) error {
	orderID = strings.TrimSpace(orderID)
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok || !strings.HasPrefix(pending.reason, externalReviewReasonPrefix) {
		return fmt.Errorf("order %s is not held for external review", orderID)
	}
	held := strings.TrimSpace(pending.stage.TaskKey)
	if held == "" {
		return fmt.Errorf("order %s is held at a stage with no task key; it cannot be completed by name", orderID)
	}
	if strings.TrimSpace(stage) != held {
		return fmt.Errorf("stage-complete refused: order %s is held at stage %s; use %s", orderID, held, stageCompleteCLI(orderID, held))
	}
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: pending.worktreePath,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}

	if reject {
		file := l.recordExternalReview(cook, review, "rejected")
		round, limit, err := l.startRework(cook, review)
		if err != nil {
			return err
		}
		if round > 0 {
			outcome := fmt.Sprintf("rework round %d of %d queued", round, limit)
			l.emitReviewDecision(cook, "rejected", outcome, review, file)
			l.cmds.ackMessage = "review rejected; " + outcome
			return nil
		}
		outcome := "stage blocked, feedback sent to the scheduler"
		if limit > 0 {
			outcome = fmt.Sprintf("rework limit (%d) reached; %s", limit, outcome)
		}
		l.emitReviewDecision(cook, "rejected", outcome, review, file)
		l.forwardToScheduler(cook, "stage_message_blocked", review, nil)
		l.cmds.ackMessage = "review rejected; " + outcome
		return l.parkPendingReview(cook, "blocked by stage message: "+review)
	}

	file := l.recordExternalReview(cook, review, "accepted")
	kept := l.keepOrDiscardAcceptedWork(orderID, pending.worktreeName)
	outcome := "not published"
	if kept != "" {
		outcome += "; work kept at " + kept
	}
	l.emitReviewDecision(cook, "accepted", outcome, review, file)
	if err := l.finishExternalAccept(cook, review, "external review accepted"); err != nil {
		return err
	}
	l.cmds.ackMessage = "review accepted"
	if kept != "" {
		l.cmds.ackMessage += "; unpublished work kept at " + kept
	}
	return nil
}

func (l *Loop) recordExternalReview(cook *cookHandle, review, verdict string) string {
	file := l.saveExternalReview(cook, review)
	l.writeExternalReviewVerdict(ExternalReviewVerdict{
		OrderID:    cook.orderID,
		StageIndex: cook.stageIndex,
		TaskKey:    cook.stage.TaskKey,
		Verdict:    verdict,
		ReviewFile: file,
	})
	return file
}

func (l *Loop) finishExternalAccept(cook *cookHandle, review, reason string) error {
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      reason,
	}); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
		return err
	}
	l.forwardToScheduler(cook, "stage_message", review, nil)
	if err := l.advanceAndPersist(context.Background(), cook, &review); err != nil {
		return err
	}
	return l.syncPendingReviewProjection()
}

func (l *Loop) saveExternalReview(cook *cookHandle, review string) string {
	dir := filepath.Join(l.projectDir, "brain", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		l.logger.Warn("save external review failed", "order", cook.orderID, "error", err)
		return ""
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", cook.orderID, cook.stage.TaskKey))
	if err := os.WriteFile(path, []byte(review), 0o644); err != nil {
		l.logger.Warn("save external review failed", "order", cook.orderID, "error", err)
		return ""
	}
	return path
}

// ExternalReviewVerdict is the latest external review outcome for an order,
// stored at <runtimeDir>/reviews/<order_id>.verdict.json.
type ExternalReviewVerdict struct {
	OrderID    string `json:"order_id"`
	StageIndex int    `json:"stage_index"`
	TaskKey    string `json:"task_key"`
	Verdict    string `json:"verdict"`
	ReviewFile string `json:"review_file"`
}

func externalReviewVerdictPath(runtimeDir, orderID string) string {
	return filepath.Join(runtimeDir, "reviews", orderID+".verdict.json")
}

func (l *Loop) writeExternalReviewVerdict(v ExternalReviewVerdict) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		path := externalReviewVerdictPath(l.runtimeDir, v.OrderID)
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			err = os.WriteFile(path, append(data, '\n'), 0o644)
		}
	}
	if err != nil {
		l.logger.Warn("write external review verdict failed", "order", v.OrderID, "error", err)
	}
}

// ReadExternalReviewVerdict returns the latest recorded verdict for an order.
func ReadExternalReviewVerdict(runtimeDir, orderID string) (ExternalReviewVerdict, error) {
	var v ExternalReviewVerdict
	data, err := os.ReadFile(externalReviewVerdictPath(runtimeDir, orderID))
	if err != nil {
		if os.IsNotExist(err) {
			return v, fmt.Errorf("no external review verdict recorded for order %q", orderID)
		}
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, fmt.Errorf("parse verdict for order %q: %w", orderID, err)
	}
	return v, nil
}

// AppendControlCommand appends one command line to <runtimeDir>/control.ndjson
// under the same control.lock the loop holds while draining commands.
func AppendControlCommand(runtimeDir string, cmd ControlCommand) error {
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return fmt.Errorf("create control directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(runtimeDir, "control.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open control lock: %w", err)
	}
	defer lock.Close()
	if err := acquireFileLock(lock.Fd()); err != nil {
		return fmt.Errorf("lock control file: %w", err)
	}
	defer func() { _ = releaseFileLock(lock.Fd()) }()

	line, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("encode control command: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(runtimeDir, "control.ndjson"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open control file: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append control command: %w", err)
	}
	return nil
}

func stageCompleteCLI(orderID, taskKey string) string {
	return fmt.Sprintf("noodle order %s complete %s", orderID, taskKey)
}
