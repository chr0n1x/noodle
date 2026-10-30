package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/loop"
	"github.com/spf13/cobra"
)

var controlAckTimeout = 30 * time.Second

// controlAckHook runs after a stage-complete command is queued (tests only).
var controlAckHook func(id string)

// newOrderCmd builds `noodle order <order_id> complete <stage> --file <review.md>`
// and `noodle order <order_id> resolve <stage> <accept|reject> <action>`.
// The stage name must match the stage the order is held or parked at; the
// loop refuses any other name.
func newOrderCmd(app *App) *cobra.Command {
	var file, feedbackFile string
	var reject, noWait bool
	cmd := &cobra.Command{
		Use: "order <order_id> complete <stage> --file <review.md>\n" +
			"  noodle order <order_id> resolve <stage> <accept|reject> <promote|exit|park|retry|retry-previous> [--feedback-file <f>]",
		Short: "Complete or resolve the stage an order is held or parked at",
		Long: "complete submits an external review for the stage an order is held at.\n" +
			"Accept advances the stage without publishing; --reject sends the review back as rework.\n\n" +
			"resolve gives a parked or held stage a verdict and an action:\n" +
			"  promote         the next stage runs\n" +
			"  exit            the remaining work stages are skipped; after hooks still run;\n" +
			"                  a reject fails the order, keeping the worktree\n" +
			"  park            the stage stays parked with the verdict recorded\n" +
			"  retry           this stage runs again with the feedback\n" +
			"  retry-previous  the stage before it runs again with the feedback, then this one\n" +
			"Retries count against [review] max_rework; at the limit the order fails.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 3 && args[1] == "complete" {
				return nil
			}
			if len(args) == 5 && args[1] == "resolve" {
				if s := args[3]; s != string(config.StageStateAccept) && s != string(config.StageStateReject) {
					return fmt.Errorf("state %q is not accept or reject", s)
				}
				if _, ok := config.ParseStageAction(args[4]); !ok {
					return fmt.Errorf("unknown action %q (promote, exit, park, retry, retry-previous)", args[4])
				}
				return nil
			}
			return fmt.Errorf("usage: noodle order <order_id> complete <stage> --file <review.md> [--reject] [--no-wait]\n" +
				"       noodle order <order_id> resolve <stage> <accept|reject> <action> [--feedback-file <f>] [--no-wait]")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[1] == "resolve" {
				return runResolve(app, args[0], args[2], args[3], args[4], feedbackFile, !noWait)
			}
			if strings.TrimSpace(file) == "" {
				return fmt.Errorf("complete needs --file <review.md>")
			}
			return runStageComplete(app, args[0], args[2], file, reject, !noWait)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "complete: Markdown file with the review")
	cmd.Flags().BoolVar(&reject, "reject", false, "complete: send the review back as rework")
	cmd.Flags().StringVar(&feedbackFile, "feedback-file", "", "resolve: file with feedback recorded on the stage and shown to later stages")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Do not wait for the loop to acknowledge")
	return cmd
}

func runStageComplete(app *App, orderID, stage, file string, reject, wait bool) error {
	review, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read review file: %w", err)
	}
	value := "accept"
	if reject {
		value = "reject"
	}
	now := time.Now().UTC()
	return sendControl(app, loop.ControlCommand{
		ID:      "review-" + strconv.FormatInt(now.UnixNano(), 10),
		Action:  "stage-complete",
		OrderID: strings.TrimSpace(orderID),
		TaskKey: strings.TrimSpace(stage),
		Prompt:  string(review),
		Value:   value,
		At:      now,
	}, "review", value, wait)
}

func runResolve(app *App, orderID, stage, st, do, feedbackFile string, wait bool) error {
	feedback := ""
	if strings.TrimSpace(feedbackFile) != "" {
		data, err := os.ReadFile(feedbackFile)
		if err != nil {
			return fmt.Errorf("read feedback file: %w", err)
		}
		feedback = string(data)
	}
	now := time.Now().UTC()
	return sendControl(app, loop.ControlCommand{
		ID:      "resolve-" + strconv.FormatInt(now.UnixNano(), 10),
		Action:  "resolve",
		OrderID: strings.TrimSpace(orderID),
		TaskKey: strings.TrimSpace(stage),
		State:   st,
		Do:      do,
		Prompt:  feedback,
		At:      now,
	}, "resolve", st+" "+do, wait)
}

// sendControl queues cmd and, when wait is set, waits up to controlAckTimeout
// for the loop's ack. label and detail name the command in the output.
func sendControl(app *App, cmd loop.ControlCommand, label, detail string, wait bool) error {
	runtimeDir, err := app.RuntimeDir()
	if err != nil {
		return err
	}
	if err := loop.AppendControlCommand(runtimeDir, cmd); err != nil {
		return err
	}
	if controlAckHook != nil {
		controlAckHook(cmd.ID)
	}
	if !wait {
		fmt.Fprintf(os.Stdout, "%s %s queued (%s)\n", label, cmd.ID, detail)
		return nil
	}
	deadline := time.Now().Add(controlAckTimeout)
	for {
		if ack, ok := findControlAck(filepath.Join(runtimeDir, "control-ack.ndjson"), cmd.ID); ok {
			if ack.Status != "ok" {
				return fmt.Errorf("%s failed: %s", cmd.Action, ack.Message)
			}
			fmt.Fprintf(os.Stdout, "%s %s: %s %s\n", label, detail, ack.Status, ack.Message)
			return nil
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stdout, "%s %s queued (%s); loop has not acknowledged it, it will apply when noodle is running\n", label, cmd.ID, detail)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}
