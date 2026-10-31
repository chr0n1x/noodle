package dispatcher

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/filex"
	"github.com/poteto/noodle/internal/stringx"
	"github.com/poteto/noodle/skill"
	wt "github.com/poteto/noodle/worktree"
)

// ProcessDispatcherConfig configures a process dispatcher.
type ProcessDispatcherConfig struct {
	ProjectDir      string
	RuntimeDir      string
	NoodleBin       string
	SkillResolver   skill.Resolver
	ProviderConfigs ProviderConfigs
	RuntimeDefault  string // command template from config, empty = built-in
	RuntimeKind     string // runtime kind this dispatcher instance services
	Sink            SessionEventSink
}

// ProcessDispatcher dispatches provider sessions as direct child processes
// with bidirectional pipes. Replaces the tmux-based dispatcher.
type ProcessDispatcher struct {
	projectDir      string
	runtimeDir      string
	noodleBin       string
	skillResolver   skill.Resolver
	providerConfigs ProviderConfigs
	runtimeDefault  string
	runtimeKind     string
	sink            SessionEventSink
}

func NewProcessDispatcher(config ProcessDispatcherConfig) *ProcessDispatcher {
	return &ProcessDispatcher{
		projectDir:      strings.TrimSpace(config.ProjectDir),
		runtimeDir:      strings.TrimSpace(config.RuntimeDir),
		noodleBin:       strings.TrimSpace(config.NoodleBin),
		skillResolver:   config.SkillResolver,
		providerConfigs: config.ProviderConfigs,
		runtimeDefault:  strings.TrimSpace(config.RuntimeDefault),
		runtimeKind:     NormalizeRuntime(config.RuntimeKind),
		sink:            config.Sink,
	}
}

func (d *ProcessDispatcher) Dispatch(ctx context.Context, req DispatchRequest) (Session, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	reqRuntime := strings.TrimSpace(req.Runtime)
	if reqRuntime == "" {
		reqRuntime = d.runtimeKind
	} else {
		reqRuntime = NormalizeRuntime(reqRuntime)
	}
	if reqRuntime != d.runtimeKind {
		return nil, fmt.Errorf("runtime %q not configured", reqRuntime)
	}
	req.Runtime = reqRuntime

	worktree, err := d.resolveWorktreeForDispatch(req)
	if err != nil {
		return nil, err
	}
	req.WorktreePath = worktree

	if d.runtimeDir == "" {
		return nil, fmt.Errorf("runtime directory not set")
	}

	sessionID, err := generateSessionID(req.Name)
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	sessionDir, promptPath, stampedPath, canonicalPath, stderrPath := sessionPaths(d.runtimeDir, sessionID)
	eventWriter, skillBundle, systemPrompt, composedPrompt, err := d.prepareSessionDir(sessionID, sessionDir, promptPath, req)
	if err != nil {
		return nil, err
	}

	controller, process, err := d.startSessionProcess(ctx, req, systemPrompt, composedPrompt, sessionID, sessionDir, stderrPath)
	if err != nil {
		return nil, err
	}

	session := newProcessSession(processSessionConfig{
		id:            sessionID,
		provider:      providerKey(req.Provider),
		process:       process,
		eventWriter:   eventWriter,
		canonicalPath: canonicalPath,
		stampedPath:   stampedPath,
		prompt:        req.Prompt,
		warnings:      skillBundle.Warnings,
		controller:    controller,
		sink:          d.sink,
	})
	session.start(ctx)
	return session, nil
}

// resolveWorktreeForDispatch validates or resolves the worktree path for a
// dispatch request. Primary checkout is allowed when explicitly opted in;
// otherwise we require a linked worktree.
func (d *ProcessDispatcher) resolveWorktreeForDispatch(req DispatchRequest) (string, error) {
	if req.AllowPrimaryCheckout {
		path := strings.TrimSpace(req.WorktreePath)
		if path == "" {
			path = d.projectDir
		}
		if strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("project directory not set")
		}
		return path, nil
	}
	validWorktree, err := wt.ValidateLinkedCheckout(req.WorktreePath)
	if err != nil {
		return "", fmt.Errorf("worktree enforcement: %w", err)
	}
	return validWorktree, nil
}

// prepareSessionDir creates the session directory, resolves skills, composes
// prompts, and writes prompt files. Returns all artifacts needed for process
// startup.
func (d *ProcessDispatcher) prepareSessionDir(
	sessionID, sessionDir, promptPath string, req DispatchRequest,
) (*event.EventWriter, loadedSkill, string, string, error) {
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, loadedSkill{}, "", "", fmt.Errorf("create session directory: %w", err)
	}
	eventWriter, err := event.NewEventWriter(d.runtimeDir, sessionID)
	if err != nil {
		return nil, loadedSkill{}, "", "", fmt.Errorf("create event writer: %w", err)
	}

	skillBundle, err := resolveSkillBundle(d.skillResolver, req)
	if err != nil {
		return nil, loadedSkill{}, "", "", err
	}

	preamble := buildSessionPreamble()
	systemPrompt, composedPrompt := composePrompts(req.Provider, req.Prompt, preamble, skillBundle.SystemPrompt, req.StageSystemPrompt)

	if _, err := writePromptFiles(sessionDir, promptPath, req.Prompt, composedPrompt); err != nil {
		return nil, loadedSkill{}, "", "", err
	}
	return eventWriter, skillBundle, systemPrompt, composedPrompt, nil
}

// startSessionProcess builds the command, starts the child process, configures
// the controller for stdin steering, drains stderr, and writes metadata.
func (d *ProcessDispatcher) startSessionProcess(
	ctx context.Context, req DispatchRequest,
	systemPrompt, composedPrompt, sessionID, sessionDir, stderrPath string,
) (*claudeController, *ProcessHandle, error) {
	cmd, err := d.buildCmd(req, systemPrompt)
	if err != nil {
		return nil, nil, fmt.Errorf("build command: %w", err)
	}
	cmd.Dir = req.WorktreePath
	cmd.Env = d.sessionEnv(req, sessionID)

	process, err := StartProcess(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("start process: %w", err)
	}

	controller, err := d.configureStdin(ctx, process, req.Provider, composedPrompt, d.closeStdinAfterTurn(req))
	if err != nil {
		_ = process.ForceKill()
		return nil, nil, err
	}

	go drainToFile(process.Stderr(), stderrPath)

	if err := WriteProcessMetadata(sessionDir, sessionID, process.PID(), nowUTC()); err != nil {
		_ = process.ForceKill()
		return nil, nil, fmt.Errorf("write process metadata: %w", err)
	}
	if err := writeDispatchMetadata(d.runtimeDir, sessionID, req, nowUTC()); err != nil {
		_ = process.ForceKill()
		return nil, nil, fmt.Errorf("write spawn metadata: %w", err)
	}
	return controller, process, nil
}

// closeStdinAfterTurn reports whether a steerable session's stdin is closed
// when its turn finishes: only with [agents.<provider>] exit_after_turn, and
// never for keep-alive sessions such as the scheduler.
func (d *ProcessDispatcher) closeStdinAfterTurn(req DispatchRequest) bool {
	return d.providerConfigs[providerKey(req.Provider)].ExitAfterTurn && !req.KeepAlive
}

// steerable reports whether a provider reads stream-json user messages on
// stdin, so a running session can be steered.
func steerable(provider string) bool {
	switch providerKey(provider) {
	case "claude", "maki":
		return true
	}
	return false
}

// providerKey normalizes a provider name, defaulting to claude.
func providerKey(provider string) string {
	if p := stringx.Normalize(provider); p != "" {
		return p
	}
	return "claude"
}

// configureStdin sets up stdin handling for the started process. For
// stream-json providers (claude, maki), creates a controller that owns stdin
// for live steering and sends the prompt as the first user message. For the
// rest, writes the prompt to stdin and closes it.
func (d *ProcessDispatcher) configureStdin(
	ctx context.Context, process *ProcessHandle, provider, composedPrompt string, closeOnIdle bool,
) (*claudeController, error) {
	if !steerable(provider) {
		go func() {
			_, _ = io.WriteString(process.Stdin(), composedPrompt)
			_ = process.Stdin().Close()
		}()
		return nil, nil
	}
	controller := newClaudeController(process.Stdin(), closeOnIdle)
	if err := controller.SendMessage(ctx, composedPrompt); err != nil {
		return nil, fmt.Errorf("send initial prompt: %w", err)
	}
	return controller, nil
}

// buildCmd constructs an exec.Cmd for the provider. The stamp processor runs
// in-process via processSession.processStream, so no shell pipeline is needed.
func (d *ProcessDispatcher) buildCmd(req DispatchRequest, systemPrompt string) (*exec.Cmd, error) {
	runtimeCmd := strings.TrimSpace(d.runtimeDefault)
	if runtimeCmd != "" {
		return d.buildCustomRuntimeCmd(req, runtimeCmd)
	}

	provider := stringx.Normalize(req.Provider)
	binary := d.resolveAgentBinary(provider)
	extraArgs := d.resolveExtraArgs(provider)

	switch provider {
	case "codex":
		args := codexBaseArgs(req)
		args = append(args, extraArgs...)
		return exec.Command(binary, args...), nil

	case "maki":
		args := append(makiBaseArgs(req, systemPrompt), extraArgs...)
		return exec.Command(binary, args...), nil

	case "pi", "pig":
		args := append(piBaseArgs(req, systemPrompt), extraArgs...)
		return exec.Command(binary, args...), nil

	case "copilot":
		args := append(copilotBaseArgs(req), extraArgs...)
		return exec.Command(binary, args...), nil

	default: // claude
		args := claudeBaseArgs(req, systemPrompt)
		// Replace -p (pipe mode) with --input-format stream-json for
		// bidirectional streaming. The prompt is sent as a user message
		// through the claudeController rather than piped to stdin.
		args = replaceFlag(args, "-p", "--input-format", "stream-json")
		args = append(args, extraArgs...)
		return exec.Command(binary, args...), nil
	}
}

// buildCustomRuntimeCmd handles user-provided runtime command templates.
func (d *ProcessDispatcher) buildCustomRuntimeCmd(req DispatchRequest, runtimeCmd string) (*exec.Cmd, error) {
	vars := map[string]string{
		"repo":   req.WorktreePath,
		"prompt": filepath.Join(d.runtimeDir, "sessions", req.Name, "input.txt"),
		"skill":  req.Skill,
		"brief":  filepath.Join(d.runtimeDir, "mise.json"),
	}
	resolved := resolveTemplateVars(runtimeCmd, vars)
	return exec.Command("sh", "-c", resolved), nil
}

func (d *ProcessDispatcher) resolveAgentBinary(provider string) string {
	name := providerKey(provider)
	if path := strings.TrimSpace(d.providerConfigs[name].Path); path != "" {
		candidate := filepath.Join(filex.ExpandHome(path), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return name
}

func (d *ProcessDispatcher) resolveExtraArgs(provider string) []string {
	return d.providerConfigs[providerKey(provider)].Args
}

// drainToFile reads from r and writes to the named file.
func drainToFile(r io.ReadCloser, path string) {
	if r == nil || strings.TrimSpace(path) == "" {
		return
	}
	defer r.Close()
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = io.Copy(f, r)
}

// replaceFlag removes old from args and appends the replacement key-value pair.
func replaceFlag(args []string, old string, newKey, newVal string) []string {
	out := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a != old {
			out = append(out, a)
		}
	}
	return append(out, newKey, newVal)
}

var _ Dispatcher = (*ProcessDispatcher)(nil)

// sessionEnv is the cook's environment. The cook runs in its worktree, so
// NOODLE_PROJECT_DIR points `noodle event emit` and other commands at the
// main checkout's .noodle, where the loop reads session events.
func (d *ProcessDispatcher) sessionEnv(req DispatchRequest, sessionID string) []string {
	inherit := d.providerConfigs[providerKey(req.Provider)].InheritEnv
	env := append(buildDispatchEnv(req, inherit), "NOODLE_SESSION_ID="+sessionID)
	if d.projectDir != "" {
		env = append(env, "NOODLE_PROJECT_DIR="+d.projectDir)
	}
	return env
}
