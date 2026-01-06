package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/willnewby/mergeish/internal/config"
	"github.com/willnewby/mergeish/internal/git"
)

// Repo represents a managed git repository
type Repo struct {
	Config   config.RepoConfig
	FullPath string
	git      *git.Git
}

// New creates a new Repo from config and workspace root
func New(cfg config.RepoConfig, workspaceRoot string) *Repo {
	fullPath := filepath.Join(workspaceRoot, cfg.Path)
	return &Repo{
		Config:   cfg,
		FullPath: fullPath,
		git:      git.New(fullPath),
	}
}

// Name returns a display name for the repo (the path)
func (r *Repo) Name() string {
	return r.Config.Path
}

// Exists checks if the repo directory exists
func (r *Repo) Exists() bool {
	info, err := os.Stat(r.FullPath)
	return err == nil && info.IsDir()
}

// IsCloned checks if the repo has been cloned
func (r *Repo) IsCloned() bool {
	return r.Exists() && r.git.IsRepo()
}

// Clone clones the repository
func (r *Repo) Clone() error {
	// Ensure parent directory exists
	parent := filepath.Dir(r.FullPath)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	return git.Clone(r.Config.URL, r.FullPath)
}

// Status returns the repository status
func (r *Repo) Status() (*git.Status, error) {
	if !r.IsCloned() {
		return nil, fmt.Errorf("repository not cloned")
	}
	return r.git.Status()
}

// CurrentBranch returns the current branch
func (r *Repo) CurrentBranch() (string, error) {
	return r.git.CurrentBranch()
}

// Pull pulls changes from remote
func (r *Repo) Pull(rebase bool) error {
	return r.git.Pull(rebase)
}

// Push pushes changes to remote
func (r *Repo) Push(force bool) error {
	return r.git.Push(force)
}

// PushSetUpstream pushes and sets upstream
func (r *Repo) PushSetUpstream() error {
	return r.git.PushSetUpstream()
}

// CreateBranch creates a new branch
func (r *Repo) CreateBranch(name string) error {
	return r.git.CreateBranch(name)
}

// DeleteBranch deletes a branch
func (r *Repo) DeleteBranch(name string) error {
	return r.git.DeleteBranch(name)
}

// Checkout switches to a branch
func (r *Repo) Checkout(branch string) error {
	return r.git.Checkout(branch)
}

// CheckoutNewBranch creates and switches to a new branch
func (r *Repo) CheckoutNewBranch(name string) error {
	return r.git.CheckoutNewBranch(name)
}

// BranchExists checks if a branch exists
func (r *Repo) BranchExists(name string) bool {
	return r.git.BranchExists(name)
}

// ListBranches returns all local branches
func (r *Repo) ListBranches() ([]string, error) {
	return r.git.ListBranches()
}

// AddAll stages all changes
func (r *Repo) AddAll() error {
	return r.git.AddAll()
}

// Commit creates a commit
func (r *Repo) Commit(message string) error {
	return r.git.Commit(message)
}

// HasStagedChanges returns true if there are staged changes
func (r *Repo) HasStagedChanges() (bool, error) {
	return r.git.HasStagedChanges()
}

// Fetch fetches from remote
func (r *Repo) Fetch() error {
	return r.git.Fetch()
}

// RunGit executes an arbitrary git command and returns stdout, stderr, and error
func (r *Repo) RunGit(args ...string) (stdout, stderr string, err error) {
	return r.git.RunRaw(args...)
}

// GetPR returns PR info for the current branch
func (r *Repo) GetPR() (*git.PRInfo, error) {
	return r.git.GetPR()
}

// CreatePR creates a new pull request
func (r *Repo) CreatePR(title, body, base string) (*git.PRInfo, error) {
	return r.git.CreatePR(title, body, base)
}

// ClosePR closes the pull request for the current branch
func (r *Repo) ClosePR() error {
	return r.git.ClosePR()
}
