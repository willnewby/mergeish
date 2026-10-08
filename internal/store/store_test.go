package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRelPath(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:willnewby/shinfra.git", "github.com/willnewby/shinfra.git"},
		{"git@github.com:spothero/terraform-monorepo", "github.com/spothero/terraform-monorepo.git"},
		{"https://github.com/spothero/ord.git", "github.com/spothero/ord.git"},
		{"ssh://git@github.com:22/spothero/mdw.git/", "github.com/spothero/mdw.git"},
		{"/srv/git/../repo.git", "local/srv/git/repo.git"},
	}

	for _, tt := range tests {
		if got := relPath(tt.url); got != filepath.FromSlash(tt.want) {
			t.Errorf("relPath(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := run(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWorktreeLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	t.Setenv(HomeEnv, filepath.Join(tmp, "home"))
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// Remote with a default branch and an existing feature branch
	src := filepath.Join(tmp, "src")
	remote := filepath.Join(tmp, "remote.git")
	git(t, tmp, "init", "--quiet", "-b", "main", src)
	git(t, src, "commit", "--quiet", "--allow-empty", "-m", "init")
	git(t, src, "branch", "existing")
	git(t, tmp, "clone", "--quiet", "--bare", src, remote)

	storePath, err := Ensure(remote)
	if err != nil {
		t.Fatal(err)
	}
	if heads := git(t, storePath, "for-each-ref", "refs/heads"); heads != "" {
		t.Errorf("store has local branches: %s", heads)
	}
	if got := DefaultBranch(storePath, "fallback"); got != "main" {
		t.Errorf("DefaultBranch = %q, want main", got)
	}

	// New branch starts from the default branch without an upstream
	newDir := filepath.Join(tmp, "ws", "new")
	if err := AddWorktree(storePath, newDir, "feature/new", "main"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, newDir, "branch", "--show-current"); got != "feature/new" {
		t.Errorf("branch = %q, want feature/new", got)
	}
	if StoreFor(newDir) == "" {
		t.Error("StoreFor did not recognise worktree")
	}
	if StoreFor(src) != "" {
		t.Error("StoreFor matched a repo outside the store")
	}

	// Existing remote branch is tracked
	existingDir := filepath.Join(tmp, "ws", "existing")
	if err := AddWorktree(storePath, existingDir, "existing", "main"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, existingDir, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/existing" {
		t.Errorf("upstream = %q, want origin/existing", got)
	}

	// The same branch cannot be checked out twice
	if err := AddWorktree(storePath, filepath.Join(tmp, "ws", "dup"), "existing", "main"); err == nil {
		t.Error("expected error checking out a branch already in a worktree")
	}

	// Unpushed commits are detected
	if HasUnpushedCommits(storePath, "feature/new") {
		t.Error("fresh branch reported as unpushed")
	}
	git(t, newDir, "commit", "--quiet", "--allow-empty", "-m", "wip")
	if !HasUnpushedCommits(storePath, "feature/new") {
		t.Error("branch with local commit not reported as unpushed")
	}

	// Dirty worktrees are not removed without force
	if err := os.WriteFile(filepath.Join(newDir, "file"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if dirty, _ := IsDirty(newDir); !dirty {
		t.Error("IsDirty = false for worktree with untracked file")
	}
	if err := RemoveWorktree(storePath, newDir, false); err == nil {
		t.Error("expected error removing dirty worktree")
	}
	if err := RemoveWorktree(storePath, newDir, true); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(storePath, "feature/new"); err != nil {
		t.Fatal(err)
	}
}
