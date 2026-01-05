package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Status represents the status of a git repository
type Status struct {
	Branch        string
	HasChanges    bool
	StagedChanges bool
	Ahead         int
	Behind        int
	Files         []FileStatus
}

// FileStatus represents the status of a single file
type FileStatus struct {
	Path   string
	Status string // "M", "A", "D", "??" etc.
}

// Git provides git operations for a specific directory
type Git struct {
	dir string
}

// New creates a new Git instance for the given directory
func New(dir string) *Git {
	return &Git{dir: dir}
}

// run executes a git command and returns stdout
func (g *Git) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Clone clones a repository to the target directory
func Clone(url, targetDir string) error {
	cmd := exec.Command("git", "clone", url, targetDir)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone: %w: %s", err, stderr.String())
	}

	return nil
}

// CurrentBranch returns the current branch name
func (g *Git) CurrentBranch() (string, error) {
	return g.run("rev-parse", "--abbrev-ref", "HEAD")
}

// Status returns the repository status
func (g *Git) Status() (*Status, error) {
	branch, err := g.CurrentBranch()
	if err != nil {
		return nil, err
	}

	// Get porcelain status
	output, err := g.run("status", "--porcelain")
	if err != nil {
		return nil, err
	}

	status := &Status{
		Branch: branch,
	}

	// Parse file status
	if output != "" {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if len(line) < 3 {
				continue
			}
			fs := FileStatus{
				Status: strings.TrimSpace(line[:2]),
				Path:   line[3:],
			}
			status.Files = append(status.Files, fs)

			// Check if staged
			if line[0] != ' ' && line[0] != '?' {
				status.StagedChanges = true
			}
		}
		status.HasChanges = len(status.Files) > 0
	}

	// Get ahead/behind
	ahead, behind, _ := g.getAheadBehind()
	status.Ahead = ahead
	status.Behind = behind

	return status, nil
}

// getAheadBehind returns how many commits ahead/behind the current branch is
func (g *Git) getAheadBehind() (ahead, behind int, err error) {
	output, err := g.run("rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	if err != nil {
		// No upstream configured
		return 0, 0, nil
	}

	parts := strings.Fields(output)
	if len(parts) != 2 {
		return 0, 0, nil
	}

	behind, _ = strconv.Atoi(parts[0])
	ahead, _ = strconv.Atoi(parts[1])
	return ahead, behind, nil
}

// Pull pulls changes from remote
func (g *Git) Pull(rebase bool) error {
	args := []string{"pull"}
	if rebase {
		args = append(args, "--rebase")
	}
	_, err := g.run(args...)
	return err
}

// Push pushes changes to remote
func (g *Git) Push(force bool) error {
	args := []string{"push"}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(args...)
	return err
}

// PushSetUpstream pushes and sets upstream for the current branch
func (g *Git) PushSetUpstream() error {
	branch, err := g.CurrentBranch()
	if err != nil {
		return err
	}
	_, err = g.run("push", "-u", "origin", branch)
	return err
}

// CreateBranch creates a new branch
func (g *Git) CreateBranch(name string) error {
	_, err := g.run("branch", name)
	return err
}

// DeleteBranch deletes a branch
func (g *Git) DeleteBranch(name string) error {
	_, err := g.run("branch", "-d", name)
	return err
}

// Checkout switches to a branch
func (g *Git) Checkout(branch string) error {
	_, err := g.run("checkout", branch)
	return err
}

// CheckoutNewBranch creates and switches to a new branch
func (g *Git) CheckoutNewBranch(name string) error {
	_, err := g.run("checkout", "-b", name)
	return err
}

// BranchExists checks if a branch exists
func (g *Git) BranchExists(name string) bool {
	_, err := g.run("rev-parse", "--verify", name)
	return err == nil
}

// ListBranches returns all local branches
func (g *Git) ListBranches() ([]string, error) {
	output, err := g.run("branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	return strings.Split(output, "\n"), nil
}

// Add stages files for commit
func (g *Git) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := g.run(args...)
	return err
}

// AddAll stages all changes
func (g *Git) AddAll() error {
	_, err := g.run("add", "-A")
	return err
}

// Commit creates a commit with the given message
func (g *Git) Commit(message string) error {
	_, err := g.run("commit", "-m", message)
	return err
}

// HasStagedChanges returns true if there are staged changes
func (g *Git) HasStagedChanges() (bool, error) {
	output, err := g.run("diff", "--cached", "--name-only")
	if err != nil {
		return false, err
	}
	return output != "", nil
}

// Fetch fetches from remote
func (g *Git) Fetch() error {
	_, err := g.run("fetch")
	return err
}

// IsRepo checks if the directory is a git repository
func (g *Git) IsRepo() bool {
	_, err := g.run("rev-parse", "--git-dir")
	return err == nil
}
