package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/poteto/noodle/loop"
	"github.com/spf13/cobra"
)

var controlAckTimeout = 30 * time.Second

// controlAckHook runs after a stage-complete command is queued (tests only).
var controlAckHook func(id string)

// newOrderCmd builds `noodle order <order_id> complete <stage> --file <review.md>`.
// The stage name must match the stage the order is held at; the loop refuses
// any other name.
func newOrderCmd(app *App) *cobra.Command {
	var file string
	var reject, noWait bool
	cmd := &cobra.Command{
		Use:   "order <order_id> complete <stage> --file <review.md>",
		Short: "Complete the stage an order is held at for external review",
		Long: "Submit an external review for the stage an order is held at.\n" +
			"Accept advances the stage without publishing; --reject sends the review back as rework.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 3 || args[1] != "complete" {
				return fmt.Errorf("usage: noodle order <order_id> complete <stage> --file <review.md> [--reject] [--no-wait]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStageComplete(app, args[0], args[2], file, reject, !noWait)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Markdown file with the review")
	cmd.Flags().BoolVar(&reject, "reject", false, "Send the review back as rework")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Do not wait for the loop to acknowledge")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func runStageComplete(app *App, orderID, stage, file string, reject, wait bool) error {
	runtimeDir, err := app.RuntimeDir()
	if err != nil {
		return err
	}
	review, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read review file: %w", err)
	}
	value := "accept"
	if reject {
		value = "reject"
	}
	now := time.Now().UTC()
	cmd := loop.ControlCommand{
		ID:      "review-" + strconv.FormatInt(now.UnixNano(), 10),
		Action:  "stage-complete",
		OrderID: strings.TrimSpace(orderID),
		TaskKey: strings.TrimSpace(stage),
		Prompt:  string(review),
		Value:   value,
		At:      now,
	}
	if err := loop.AppendControlCommand(runtimeDir, cmd); err != nil {
		return err
	}
	if controlAckHook != nil {
		controlAckHook(cmd.ID)
	}
	if !wait {
		fmt.Fprintf(os.Stdout, "review %s queued (%s)\n", cmd.ID, value)
		return nil
	}
	deadline := time.Now().Add(controlAckTimeout)
	for {
		if ack, ok := findControlAck(filepath.Join(runtimeDir, "control-ack.ndjson"), cmd.ID); ok {
			if ack.Status != "ok" {
				return fmt.Errorf("%s failed: %s", cmd.Action, ack.Message)
			}
			fmt.Fprintf(os.Stdout, "review %s: %s %s\n", value, ack.Status, ack.Message)
			return nil
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stdout, "review %s queued (%s); loop has not acknowledged it, it will apply when noodle is running\n", cmd.ID, value)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}
