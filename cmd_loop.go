package main

import (
	"strconv"
	"time"

	"github.com/poteto/noodle/loop"
	"github.com/spf13/cobra"
)

var loopControlShort = map[string]string{
	"pause":  "Stop scheduling and dispatching new work; running cooks continue",
	"resume": "Resume scheduling and dispatching after pause or drain",
	"drain":  "Finish running work, then stop taking new work",
}

// newLoopControlCmd builds `noodle pause`, `noodle resume` or `noodle drain`,
// each queueing the loop control action of the same name.
func newLoopControlCmd(app *App, action string) *cobra.Command {
	var noWait bool
	cmd := &cobra.Command{
		Use:   action,
		Short: loopControlShort[action],
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now().UTC()
			return sendControl(app, loop.ControlCommand{
				ID:     action + "-" + strconv.FormatInt(now.UnixNano(), 10),
				Action: action,
				At:     now,
			}, "loop", action, !noWait)
		},
	}
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Do not wait for the loop to acknowledge")
	return cmd
}
