package store

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// HomeEnv overrides the mergeish home directory (default: ~/.mergeish)
const HomeEnv = "MERGEISH_HOME"

// Home returns the mergeish home directory
func Home() (string, error) {
	if h := os.Getenv(HomeEnv); h != "" {
		return filepath.Abs(h)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mergeish"), nil
}

// ReposDir returns the directory holding the bare store clones
func ReposDir() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "repos"), nil
}

// WorkspacesDir returns the directory holding worktree workspaces
func WorkspacesDir() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "workspaces"), nil
}

// WorkspaceDir returns the workspace directory for a branch.
// Slashes in branch names are replaced so each workspace is a single directory.
func WorkspaceDir(branch string) (string, error) {
	dir, err := WorkspacesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, strings.ReplaceAll(branch, "/", "-")), nil
}

var (
	scpURL    = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)
	unsafeDir = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// relPath maps a git URL to a store-relative path like github.com/owner/repo.git
func relPath(url string) string {
	var host, path string

	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			host, path = rest[:j], rest[j+1:]
		} else {
			host = rest
		}
		// Strip userinfo and port
		if k := strings.LastIndex(host, "@"); k >= 0 {
			host = host[k+1:]
		}
		if k := strings.Index(host, ":"); k >= 0 {
			host = host[:k]
		}
	} else if m := scpURL.FindStringSubmatch(url); m != nil && !filepath.IsAbs(url) {
		host, path = m[1], m[2]
	} else {
		// Local path
		host, path = "local", url
	}

	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")

	parts := []string{unsafeDir.ReplaceAllString(host, "_")}
	for _, p := range strings.Split(path, "/") {
		if p == "" || p == "." || p == ".." {
			continue
		}
		parts = append(parts, unsafeDir.ReplaceAllString(p, "_"))
	}
	parts[len(parts)-1] += ".git"
	return filepath.Join(parts...)
}

// Path returns the store path for a git URL
func Path(url string) (string, error) {
	dir, err := ReposDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, relPath(url)), nil
}

// run executes a git command in dir and returns trimmed stdout
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Ensure creates the bare store for url if missing, then fetches it.
// The store has no local branches of its own: every branch lives in a worktree,
// and new branches start from the freshly fetched remote-tracking refs.
func Ensure(url string) (string, error) {
	path, err := Path(url)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", fmt.Errorf("creating store directory: %w", err)
		}
		if _, err := run(filepath.Dir(path), "init", "--bare", "--quiet", path); err != nil {
			return "", err
		}
		if _, err := run(path, "remote", "add", "origin", url); err != nil {
			os.RemoveAll(path)
			return "", err
		}
	}

	if _, err := run(path, "fetch", "--prune", "--quiet", "origin"); err != nil {
		return "", err
	}
	if _, err := run(path, "remote", "set-head", "origin", "--auto"); err != nil {
		return "", err
	}

	return path, nil
}

// refExists checks whether a ref exists in the repo at dir
func refExists(dir, ref string) bool {
	_, err := run(dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// DefaultBranch returns the remote's default branch, falling back to fallback
func DefaultBranch(storePath, fallback string) string {
	ref, err := run(storePath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil && ref != "" {
		return strings.TrimPrefix(ref, "origin/")
	}
	return fallback
}

// AddWorktree checks out branch from the store into dir.
// An existing local branch is reused, a remote branch is tracked,
// and otherwise a new branch is created from the remote default branch.
func AddWorktree(storePath, dir, branch, fallbackBase string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	var args []string
	switch {
	case refExists(storePath, "refs/heads/"+branch):
		args = []string{"worktree", "add", "--quiet", dir, branch}
	case refExists(storePath, "refs/remotes/origin/"+branch):
		args = []string{"worktree", "add", "--quiet", "--track", "-b", branch, dir, "origin/" + branch}
	default:
		base := "origin/" + DefaultBranch(storePath, fallbackBase)
		args = []string{"worktree", "add", "--quiet", "--no-track", "-b", branch, dir, base}
	}

	_, err := run(storePath, args...)
	return err
}

// StoreFor returns the store a worktree belongs to, or "" if dir is not
// a worktree of a mergeish store
func StoreFor(dir string) string {
	common, err := run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}

	reposDir, err := ReposDir()
	if err != nil {
		return ""
	}

	common = resolve(common)
	if rel, err := filepath.Rel(resolve(reposDir), common); err == nil && !strings.HasPrefix(rel, "..") {
		return common
	}
	return ""
}

// resolve returns path with symlinks evaluated, or path unchanged if that fails
func resolve(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	return path
}

// IsDirty reports whether the worktree at dir has uncommitted or untracked changes
func IsDirty(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// RemoveWorktree removes the worktree at dir from its store
func RemoveWorktree(storePath, dir string, force bool) error {
	args := []string{"worktree", "remove", dir}
	if force {
		args = []string{"worktree", "remove", "--force", dir}
	}
	_, err := run(storePath, args...)
	return err
}

// DeleteBranch force-deletes a local branch from the store
func DeleteBranch(storePath, branch string) error {
	_, err := run(storePath, "branch", "-D", branch)
	return err
}

// HasUnpushedCommits reports whether branch has commits not on any remote branch
func HasUnpushedCommits(storePath, branch string) bool {
	out, err := run(storePath, "rev-list", "--max-count=1", branch, "--not", "--remotes=origin")
	return err != nil || out != ""
}
