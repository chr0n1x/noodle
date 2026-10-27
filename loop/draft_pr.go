package loop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/poteto/noodle/config"
)

// DraftPRRequest describes an order whose kept work, all on Branch, is
// delivered as a draft PR.
type DraftPRRequest struct {
	OrderID string
	Branch  string
	Title   string
	Body    string
}

type DraftPRResult struct {
	Branch string
	URL    string
}

// DraftPRPublisher pushes an order's work to a remote branch and opens one
// draft PR per order.
type DraftPRPublisher interface {
	Publish(req DraftPRRequest) (DraftPRResult, error)
}

type draftPRRecord = DraftPRLink

// DraftPRLink is the branch and PR URL published for one order.
type DraftPRLink struct {
	Branch string `json:"branch"`
	URL    string `json:"url"`
}

// ReadDraftPRs returns the published draft PRs keyed by order ID.
func ReadDraftPRs(runtimeDir string) map[string]DraftPRLink {
	links := map[string]DraftPRLink{}
	data, err := os.ReadFile(filepath.Join(runtimeDir, "draft-prs.json"))
	if err != nil {
		return links
	}
	_ = json.Unmarshal(data, &links)
	return links
}

type gitDraftPRPublisher struct {
	projectDir string
	recordPath string
	cfg        config.DeliveryConfig
}

func newGitDraftPRPublisher(projectDir, runtimeDir string, cfg config.DeliveryConfig) *gitDraftPRPublisher {
	return &gitDraftPRPublisher{
		projectDir: projectDir,
		recordPath: filepath.Join(runtimeDir, "draft-prs.json"),
		cfg:        cfg,
	}
}

func (p *gitDraftPRPublisher) Publish(req DraftPRRequest) (DraftPRResult, error) {
	records, err := p.load()
	if err != nil {
		return DraftPRResult{}, err
	}
	rec, exists := records[req.OrderID]
	if rec.Branch == "" {
		rec.Branch = strings.ReplaceAll(p.cfg.BranchTemplate(), "{order_id}", req.OrderID)
	}
	if _, err := p.run("git", "-C", p.projectDir, "push", p.cfg.PushRemote,
		"refs/heads/"+req.Branch+":refs/heads/"+rec.Branch); err != nil {
		return DraftPRResult{}, fmt.Errorf("push %s to %s/%s: %w", req.Branch, p.cfg.PushRemote, rec.Branch, err)
	}
	if !exists || rec.URL == "" {
		owner, err := p.remoteOwner()
		if err != nil {
			return DraftPRResult{}, err
		}
		out, err := p.run("gh", "pr", "create", "--draft",
			"--repo", p.cfg.PRRepo,
			"--base", p.cfg.Base,
			"--head", owner+":"+rec.Branch,
			"--title", req.Title,
			"--body", req.Body)
		if err != nil {
			return DraftPRResult{}, fmt.Errorf("open draft PR: %w", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		rec.URL = strings.TrimSpace(lines[len(lines)-1])
	}
	records[req.OrderID] = rec
	if err := p.save(records); err != nil {
		return DraftPRResult{}, err
	}
	return DraftPRResult{Branch: rec.Branch, URL: rec.URL}, nil
}

var githubOwnerPattern = regexp.MustCompile(`github\.com[:/]([^/]+)/`)

func (p *gitDraftPRPublisher) remoteOwner() (string, error) {
	url, err := p.run("git", "-C", p.projectDir, "remote", "get-url", p.cfg.PushRemote)
	if err != nil {
		return "", fmt.Errorf("read remote %s: %w", p.cfg.PushRemote, err)
	}
	m := githubOwnerPattern.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("remote %s is not a GitHub URL: %s", p.cfg.PushRemote, strings.TrimSpace(url))
	}
	return m[1], nil
}

func (p *gitDraftPRPublisher) run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = p.projectDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (p *gitDraftPRPublisher) load() (map[string]draftPRRecord, error) {
	records := map[string]draftPRRecord{}
	data, err := os.ReadFile(p.recordPath)
	if os.IsNotExist(err) {
		return records, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read draft PR records: %w", err)
	}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse draft PR records: %w", err)
	}
	return records, nil
}

func (p *gitDraftPRPublisher) save(records map[string]draftPRRecord) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.recordPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p.recordPath, append(data, '\n'), 0o644)
}
