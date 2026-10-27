package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// linkPaths symlinks each LinkPaths entry from the root into the worktree.
func (a *App) linkPaths(wtPath string) {
	for _, p := range a.LinkPaths {
		src := filepath.Join(a.Root, p)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(wtPath, p)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := os.Symlink(src, dst); err == nil {
			a.info("Symlinked " + p)
			a.excludeLinkedPath(wtPath, p)
		}
	}
}

// excludeLinkedPath adds "/<p>" to the shared info/exclude when git would
// report the symlink as untracked. Directory patterns like "brain/" do not
// match a symlink, which git treats as a file.
func (a *App) excludeLinkedPath(wtPath, p string) {
	if exec.Command("git", "-C", wtPath, "check-ignore", "-q", "--no-index", p).Run() == nil {
		return
	}
	out, err := exec.Command("git", "-C", wtPath, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return
	}
	exclude := filepath.Join(strings.TrimSpace(string(out)), "info", "exclude")
	_ = os.MkdirAll(filepath.Dir(exclude), 0o755)
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString("/" + filepath.ToSlash(p) + "\n")
}
