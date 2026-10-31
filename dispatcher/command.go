package dispatcher

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/poteto/noodle/internal/shellx"
	"github.com/poteto/noodle/internal/stringx"
)

const codexSkillRefsLimitBytes = 50 * 1024

// claudeBaseArgs returns the canonical Claude CLI flags for a dispatch request.
// Callers prepend the binary path and append site-specific extras.
func claudeBaseArgs(req DispatchRequest, systemPrompt string) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
	}
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
	}
	if req.BudgetCap > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", req.BudgetCap))
	}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(req.ReasoningLevel) != "" {
		args = append(args, "--effort", req.ReasoningLevel)
	}
	if strings.TrimSpace(systemPrompt) != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}

// makiBaseArgs returns the maki CLI flags for a dispatch request. maki reads
// and writes Claude-format stream-json, so the session is steered through a
// claudeController like Claude's.
func makiBaseArgs(req DispatchRequest, systemPrompt string) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--yolo",
	}
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
	}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(systemPrompt) != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}

// piBaseArgs returns the pi CLI flags for a dispatch request; pig takes the
// same flags. The prompt is piped to stdin and the session runs one-shot.
func piBaseArgs(req DispatchRequest, systemPrompt string) []string {
	args := []string{"-p", "--mode", "json"}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(req.ReasoningLevel) != "" {
		args = append(args, "--thinking", req.ReasoningLevel)
	}
	if strings.TrimSpace(systemPrompt) != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	return args
}

// copilotBaseArgs returns the GitHub Copilot CLI flags for a dispatch
// request. The prompt is piped to stdin and the session runs one-shot.
func copilotBaseArgs(req DispatchRequest) []string {
	args := []string{"--output-format", "json", "--yolo", "--no-color"}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(req.ReasoningLevel) != "" {
		args = append(args, "--reasoning-effort", req.ReasoningLevel)
	}
	return args
}

// codexBaseArgs returns the canonical Codex CLI flags for a dispatch request.
// Callers prepend the binary path and append site-specific extras.
func codexBaseArgs(req DispatchRequest) []string {
	args := []string{
		"exec",
		"--dangerously-bypass-approvals-and-sandbox",
		"--skip-git-repo-check",
		"--json",
	}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if p := strings.TrimSpace(req.ModelProvider); p != "" {
		args = append(args, "-c", fmt.Sprintf("model_provider=%q", p))
	}
	return args
}

func generateSessionID(name string) (string, error) {
	randBytes := make([]byte, 3)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}
	base := shellx.SanitizeToken(name, "cook")
	timestamp := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%s-%x", base, timestamp, randBytes), nil
}

func sessionPaths(runtimeDir, sessionID string) (sessionDir, promptPath, stampedPath, canonicalPath, stderrPath string) {
	sessionDir = filepath.Join(runtimeDir, "sessions", sessionID)
	promptPath = filepath.Join(sessionDir, "prompt.txt")
	stampedPath = filepath.Join(sessionDir, "raw.ndjson")
	canonicalPath = filepath.Join(sessionDir, "canonical.ndjson")
	stderrPath = filepath.Join(sessionDir, "stderr.log")
	return
}

func inputPath(sessionDir string) string {
	return filepath.Join(sessionDir, "input.txt")
}

// resolveTemplateVars replaces {{key}} placeholders with verbatim values.
// No shell quoting — the template author controls quoting in their template.
func resolveTemplateVars(tmpl string, vars map[string]string) string {
	result := tmpl
	for key, value := range vars {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}

// baseEnvKeys are inherited even when an inherit_env list is set, so the
// agent can still find binaries, its home directory and a temp directory.
var baseEnvKeys = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TERM", "TMPDIR", "LANG", "LC_*",
	"SYSTEMROOT", "COMSPEC", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP",
}

// filterEnv keeps the entries whose key is in baseEnvKeys or allow. Keys
// match case-insensitively and a trailing "*" matches a prefix. A nil allow
// list keeps every entry.
func filterEnv(environ, allow []string) []string {
	if allow == nil {
		return environ
	}
	patterns := append(slices.Clone(baseEnvKeys), allow...)
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		for _, pattern := range patterns {
			if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
				if len(key) >= len(prefix) && strings.EqualFold(key[:len(prefix)], prefix) {
					out = append(out, entry)
					break
				}
			} else if strings.EqualFold(key, pattern) {
				out = append(out, entry)
				break
			}
		}
	}
	return out
}

func buildDispatchEnv(req DispatchRequest, inheritEnv []string) []string {
	environ := filterEnv(os.Environ(), inheritEnv)
	env := make([]string, 0, len(environ)+len(req.EnvVars)+4)
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "CLAUDECODE") {
			continue
		}
		env = append(env, entry)
	}

	env = append(env, "NOODLE_WORKTREE="+req.WorktreePath)
	env = append(env, "NOODLE_PROVIDER="+req.Provider)
	env = append(env, "NOODLE_MODEL="+req.Model)
	if req.ReasoningLevel != "" {
		env = append(env, "NOODLE_REASONING_LEVEL="+req.ReasoningLevel)
	}
	for key, value := range req.EnvVars {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

// InlinesSkill reports whether the dispatcher puts a provider's skill in the
// system prompt instead of leaving the agent to load it with a Skill tool.
func InlinesSkill(provider string) bool {
	switch stringx.Normalize(provider) {
	case "claude", "maki", "pi", "pig", "copilot":
		return true
	}
	return false
}

func composePrompts(provider, requestPrompt, preamble, skillSystemPrompt, stageSystemPrompt string) (systemPrompt, finalPrompt string) {
	finalPrompt = requestPrompt
	fullSystemPrompt := stringx.JoinNonEmpty("\n\n", preamble, skillSystemPrompt, stageSystemPrompt)
	if strings.EqualFold(provider, "copilot") {
		// copilot has no system prompt flag; the whole prompt goes on stdin.
		if fullSystemPrompt != "" {
			finalPrompt = requestPrompt + "\n\n---\n\n" + fullSystemPrompt
		}
		return "", finalPrompt
	}
	if InlinesSkill(provider) {
		systemPrompt = fullSystemPrompt
		return systemPrompt, finalPrompt
	}
	// Codex loads skills natively via Skill(name) — only inline the preamble
	// and the stage's system prompt.
	inline := stringx.JoinNonEmpty("\n\n", preamble, stageSystemPrompt)
	if strings.EqualFold(provider, "codex") && inline != "" {
		finalPrompt = requestPrompt + "\n\n---\n\n" + inline
	}
	return systemPrompt, finalPrompt
}

