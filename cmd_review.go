package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/poteto/noodle/loop"
	"github.com/spf13/cobra"
)

const externalReviewPrefix = "external review"

type reviewListItem struct {
	OrderID      string `json:"order_id"`
	StageIndex   int    `json:"stage_index"`
	TaskKey      string `json:"task_key"`
	Title        string `json:"title"`
	WorktreePath string `json:"worktree_path"`
	PromptFile   string `json:"prompt_file"`
}

func newReviewCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "review",
		Short: "Work with stages held for external review",
	}

	var asJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List stages waiting for external review",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReviewList(app, asJSON)
		},
	}
	list.Flags().BoolVar(&asJSON, "json", false, "Print JSON")

	prompt := &cobra.Command{
		Use:   "prompt <order_id>",
		Short: "Print the review prompt for an order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewPrompt(app, args[0])
		},
	}

	var showJSON bool
	show := &cobra.Command{
		Use:   "show <order_id>",
		Short: "Show the latest external review verdict for an order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewShow(app, args[0], showJSON)
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, "Print JSON")

	root.AddCommand(list, prompt, show)
	return root
}

func externalReviews(runtimeDir string) ([]reviewListItem, error) {
	pending, err := loop.ReadPendingReview(runtimeDir)
	if err != nil {
		return nil, err
	}
	items := []reviewListItem{}
	for _, p := range pending {
		if !strings.HasPrefix(p.Reason, externalReviewPrefix) {
			continue
		}
		items = append(items, reviewListItem{
			OrderID:      p.OrderID,
			StageIndex:   p.StageIndex,
			TaskKey:      p.TaskKey,
			Title:        p.Title,
			WorktreePath: p.WorktreePath,
			PromptFile:   filepath.Join(runtimeDir, "reviews", fmt.Sprintf("%s-%d.prompt.md", p.OrderID, p.StageIndex)),
		})
	}
	return items, nil
}

func runReviewList(app *App, asJSON bool) error {
	runtimeDir, err := app.RuntimeDir()
	if err != nil {
		return err
	}
	items, err := externalReviews(runtimeDir)
	if err != nil {
		return err
	}
	if asJSON {
		data, err := json.Marshal(items)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "{\"items\": %s}\n", data)
		return nil
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stdout, "no external reviews pending")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ORDER\tSTAGE\tTASK\tTITLE\tWORKTREE")
	for _, it := range items {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", it.OrderID, it.StageIndex, it.TaskKey, it.Title, it.WorktreePath)
	}
	return w.Flush()
}

func runReviewPrompt(app *App, orderID string) error {
	runtimeDir, err := app.RuntimeDir()
	if err != nil {
		return err
	}
	items, err := externalReviews(runtimeDir)
	if err != nil {
		return err
	}
	for _, it := range items {
		if it.OrderID != strings.TrimSpace(orderID) {
			continue
		}
		data, err := os.ReadFile(it.PromptFile)
		if err != nil {
			return fmt.Errorf("read review prompt: %w", err)
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	return fmt.Errorf("no external review pending for order %q", orderID)
}

func findControlAck(path, id string) (loop.ControlAck, bool) {
	f, err := os.Open(path)
	if err != nil {
		return loop.ControlAck{}, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ack loop.ControlAck
		if json.Unmarshal(scanner.Bytes(), &ack) == nil && ack.ID == id {
			return ack, true
		}
	}
	return loop.ControlAck{}, false
}

func runReviewShow(app *App, orderID string, asJSON bool) error {
	runtimeDir, err := app.RuntimeDir()
	if err != nil {
		return err
	}
	v, err := loop.ReadExternalReviewVerdict(runtimeDir, strings.TrimSpace(orderID))
	if err != nil {
		return err
	}
	if asJSON {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	fmt.Printf("order %s stage %d (%s): %s\nreview: %s\n", v.OrderID, v.StageIndex, v.TaskKey, v.Verdict, v.ReviewFile)
	return nil
}
