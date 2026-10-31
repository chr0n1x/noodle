package loop

import (
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/dispatcher"
	"github.com/poteto/noodle/internal/stringx"
	"github.com/poteto/noodle/mise"
	"github.com/poteto/noodle/monitor"
	loopruntime "github.com/poteto/noodle/runtime"
	"github.com/poteto/noodle/skill"
	"github.com/poteto/noodle/worktree"
)

type noOpWorktree struct{}

func (noOpWorktree) Create(string, ...worktree.CreateOpts) error   { return nil }
func (noOpWorktree) Merge(string, ...worktree.MergeOpts) error     { return nil }
func (noOpWorktree) MergeRemoteBranch(string) error                { return nil }
func (noOpWorktree) Cleanup(string, ...worktree.CleanupOpts) error { return nil }
func (noOpWorktree) HasUnmergedCommits(string) (bool, error)       { return false, nil }

func defaultDependencies(projectDir, runtimeDir, noodleBin string, cfg config.Config, logger *slog.Logger, sink dispatcher.SessionEventSink) Dependencies {
	resolver := skill.Resolver{SearchPaths: cfg.Skills.Paths}
	local := dispatcher.NewProcessDispatcher(dispatcher.ProcessDispatcherConfig{
		ProjectDir:      projectDir,
		RuntimeDir:      runtimeDir,
		NoodleBin:       noodleBin,
		SkillResolver:   resolver,
		RuntimeKind:     "process",
		Sink:            sink,
		ProviderConfigs: ProviderConfigsFrom(cfg.Agents),
	})
	runtimes := map[string]loopruntime.Runtime{
		"process": loopruntime.NewProcessRuntime(local, runtimeDir, cfg.Runtime.Process.MaxConcurrent),
	}
	if runtimeEnabled(cfg.AvailableRuntimes(), "sprites") {
		spriteName := strings.TrimSpace(cfg.Runtime.Sprites.SpriteName)
		if spriteName == "" {
			logger.Warn("sprites runtime unavailable: sprite_name not set")
		} else {
			sd := dispatcher.NewSpritesDispatcher(dispatcher.SpritesDispatcherConfig{
				ProjectDir:    projectDir,
				RuntimeDir:    runtimeDir,
				NoodleBin:     noodleBin,
				SkillResolver: resolver,
				SpriteName:    spriteName,
				Token:         cfg.Runtime.Sprites.Token(),
				GitToken:      cfg.Runtime.Sprites.GitToken(),
				Sink:          sink,
			})
			r := loopruntime.NewDispatcherRuntime("sprites", sd, runtimeDir)
			r.SetMaxConcurrent(cfg.Runtime.Sprites.MaxConcurrent)
			runtimes["sprites"] = r
		}
	}

	wtApp, _ := worktree.NewApp()
	var wt WorktreeManager
	if wtApp != nil {
		wtApp.CmdPrefix = "noodle worktree"
		wtApp.Quiet = true
		wtApp.LinkPaths = cfg.Worktree.Link
		wt = wtApp
	} else {
		wt = noOpWorktree{}
	}
	var draftPR DraftPRPublisher
	if cfg.Delivery.Enabled() {
		draftPR = newGitDraftPRPublisher(projectDir, runtimeDir, cfg.Delivery)
	}
	return Dependencies{
		DraftPR:        draftPR,
		Runtimes:       runtimes,
		Worktree:       wt,
		Adapter:        adapter.NewRunner(projectDir, cfg),
		Mise:           mise.NewBuilder(projectDir, cfg),
		Monitor:        monitor.NewMonitor(runtimeDir),
		Now:            time.Now,
		OrdersFile:     filepath.Join(runtimeDir, "orders.json"),
		OrdersNextFile: filepath.Join(runtimeDir, "orders-next.json"),
		StatusFile:     filepath.Join(runtimeDir, "status.json"),
	}
}

func runtimeEnabled(available []string, kind string) bool {
	kind = stringx.Normalize(kind)
	for _, runtime := range available {
		if stringx.Normalize(runtime) == kind {
			return true
		}
	}
	return false
}

// ProviderConfigsFrom maps every [agents.<provider>] table to dispatcher config.
func ProviderConfigsFrom(agents config.AgentsConfig) dispatcher.ProviderConfigs {
	out := make(dispatcher.ProviderConfigs, len(config.Providers))
	for _, name := range config.Providers {
		p := agents.For(name)
		inherit, _ := agents.InheritEnvFor(name)
		out[name] = dispatcher.ProviderConfig{Path: p.Path, Args: p.Args, ExitAfterTurn: p.ExitAfterTurn, InheritEnv: inherit}
	}
	return out
}
