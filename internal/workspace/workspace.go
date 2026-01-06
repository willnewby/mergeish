package workspace

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/willnewby/mergeish/internal/config"
	"github.com/willnewby/mergeish/internal/git"
	"github.com/willnewby/mergeish/internal/repo"
)

// Result represents the result of an operation on a single repo
type Result struct {
	Repo  *repo.Repo
	Error error
}

// StatusResult represents status information for a repo
type StatusResult struct {
	Repo   *repo.Repo
	Status *git.Status
	Error  error
}

// Workspace manages multiple repositories
type Workspace struct {
	Root     string
	Config   *config.Config
	Repos    []*repo.Repo
	Parallel bool
}

// New creates a new workspace from config
func New(cfg *config.Config, root string) *Workspace {
	repos := make([]*repo.Repo, len(cfg.Repos))
	for i, rc := range cfg.Repos {
		repos[i] = repo.New(rc, root)
	}

	return &Workspace{
		Root:     root,
		Config:   cfg,
		Repos:    repos,
		Parallel: cfg.Settings.Parallel,
	}
}

// Load loads a workspace from the config file
func Load(configPath string) (*Workspace, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}

	root := filepath.Dir(configPath)
	return New(cfg, root), nil
}

// Clone clones all repositories
func (w *Workspace) Clone() []Result {
	return w.forEach(func(r *repo.Repo) error {
		if r.IsCloned() {
			return nil // Already cloned
		}
		return r.Clone()
	})
}

// Pull pulls all repositories
func (w *Workspace) Pull(rebase bool) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		return r.Pull(rebase)
	})
}

// Push pushes all repositories
func (w *Workspace) Push(force bool) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		return r.Push(force)
	})
}

// Status returns status for all repositories
func (w *Workspace) Status() []StatusResult {
	results := make([]StatusResult, len(w.Repos))

	if w.Parallel {
		var wg sync.WaitGroup
		for i, r := range w.Repos {
			wg.Add(1)
			go func(i int, r *repo.Repo) {
				defer wg.Done()
				status, err := r.Status()
				results[i] = StatusResult{Repo: r, Status: status, Error: err}
			}(i, r)
		}
		wg.Wait()
	} else {
		for i, r := range w.Repos {
			status, err := r.Status()
			results[i] = StatusResult{Repo: r, Status: status, Error: err}
		}
	}

	return results
}

// CreateBranch creates a branch on all repos
func (w *Workspace) CreateBranch(name string) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		if r.BranchExists(name) {
			return fmt.Errorf("branch %q already exists", name)
		}
		return r.CheckoutNewBranch(name)
	})
}

// DeleteBranch deletes a branch on all repos
func (w *Workspace) DeleteBranch(name string) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		// Can't delete current branch
		current, err := r.CurrentBranch()
		if err != nil {
			return err
		}
		if current == name {
			return fmt.Errorf("cannot delete current branch")
		}
		return r.DeleteBranch(name)
	})
}

// Checkout switches all repos to a branch, creating it if it doesn't exist
func (w *Workspace) Checkout(name string) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		if r.BranchExists(name) {
			return r.Checkout(name)
		}
		// Branch doesn't exist, create it
		return r.CheckoutNewBranch(name)
	})
}

// Commit commits staged changes on all repos
func (w *Workspace) Commit(message string, addAll bool) []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}

		if addAll {
			if err := r.AddAll(); err != nil {
				return err
			}
		}

		hasChanges, err := r.HasStagedChanges()
		if err != nil {
			return err
		}
		if !hasChanges {
			return nil // No changes to commit
		}

		return r.Commit(message)
	})
}

// CheckBranchConsistency checks if all repos are on the same branch
func (w *Workspace) CheckBranchConsistency() (string, bool, error) {
	var firstBranch string
	consistent := true

	for _, r := range w.Repos {
		if !r.IsCloned() {
			continue
		}

		branch, err := r.CurrentBranch()
		if err != nil {
			return "", false, err
		}

		if firstBranch == "" {
			firstBranch = branch
		} else if branch != firstBranch {
			consistent = false
		}
	}

	return firstBranch, consistent, nil
}

// forEach runs an operation on all repos
func (w *Workspace) forEach(fn func(*repo.Repo) error) []Result {
	results := make([]Result, len(w.Repos))

	if w.Parallel {
		var wg sync.WaitGroup
		for i, r := range w.Repos {
			wg.Add(1)
			go func(i int, r *repo.Repo) {
				defer wg.Done()
				results[i] = Result{Repo: r, Error: fn(r)}
			}(i, r)
		}
		wg.Wait()
	} else {
		for i, r := range w.Repos {
			results[i] = Result{Repo: r, Error: fn(r)}
		}
	}

	return results
}

// HasErrors checks if any results have errors
func HasErrors(results []Result) bool {
	for _, r := range results {
		if r.Error != nil {
			return true
		}
	}
	return false
}

// GitResult represents the result of a raw git command on a single repo
type GitResult struct {
	Repo   *repo.Repo
	Stdout string
	Stderr string
	Error  error
}

// RunGit executes an arbitrary git command on all repos
func (w *Workspace) RunGit(args []string) []GitResult {
	results := make([]GitResult, len(w.Repos))

	if w.Parallel {
		var wg sync.WaitGroup
		for i, r := range w.Repos {
			wg.Add(1)
			go func(i int, r *repo.Repo) {
				defer wg.Done()
				if !r.IsCloned() {
					results[i] = GitResult{Repo: r, Error: fmt.Errorf("not cloned")}
					return
				}
				stdout, stderr, err := r.RunGit(args...)
				results[i] = GitResult{Repo: r, Stdout: stdout, Stderr: stderr, Error: err}
			}(i, r)
		}
		wg.Wait()
	} else {
		for i, r := range w.Repos {
			if !r.IsCloned() {
				results[i] = GitResult{Repo: r, Error: fmt.Errorf("not cloned")}
				continue
			}
			stdout, stderr, err := r.RunGit(args...)
			results[i] = GitResult{Repo: r, Stdout: stdout, Stderr: stderr, Error: err}
		}
	}

	return results
}

// PRResult represents the result of a PR operation on a single repo
type PRResult struct {
	Repo     *repo.Repo
	PR       *git.PRInfo
	Existed  bool // true if PR already existed (not newly created)
	Error    error
}

// GetPRs returns PR status for all repos
func (w *Workspace) GetPRs() []PRResult {
	results := make([]PRResult, len(w.Repos))

	if w.Parallel {
		var wg sync.WaitGroup
		for i, r := range w.Repos {
			wg.Add(1)
			go func(i int, r *repo.Repo) {
				defer wg.Done()
				if !r.IsCloned() {
					results[i] = PRResult{Repo: r, Error: fmt.Errorf("not cloned")}
					return
				}
				pr, err := r.GetPR()
				results[i] = PRResult{Repo: r, PR: pr, Error: err}
			}(i, r)
		}
		wg.Wait()
	} else {
		for i, r := range w.Repos {
			if !r.IsCloned() {
				results[i] = PRResult{Repo: r, Error: fmt.Errorf("not cloned")}
				continue
			}
			pr, err := r.GetPR()
			results[i] = PRResult{Repo: r, PR: pr, Error: err}
		}
	}

	return results
}

// CreatePRs creates PRs for all repos on the current branch, skipping repos that already have a PR
func (w *Workspace) CreatePRs(title, body, base string) []PRResult {
	results := make([]PRResult, len(w.Repos))

	createPR := func(i int, r *repo.Repo) {
		if !r.IsCloned() {
			results[i] = PRResult{Repo: r, Error: fmt.Errorf("not cloned")}
			return
		}

		// Check if PR already exists
		existingPR, err := r.GetPR()
		if err != nil {
			results[i] = PRResult{Repo: r, Error: fmt.Errorf("checking existing PR: %w", err)}
			return
		}
		if existingPR != nil {
			// PR already exists, return it without error
			results[i] = PRResult{Repo: r, PR: existingPR, Existed: true, Error: nil}
			return
		}

		// Create new PR
		pr, err := r.CreatePR(title, body, base)
		results[i] = PRResult{Repo: r, PR: pr, Error: err}
	}

	if w.Parallel {
		var wg sync.WaitGroup
		for i, r := range w.Repos {
			wg.Add(1)
			go func(i int, r *repo.Repo) {
				defer wg.Done()
				createPR(i, r)
			}(i, r)
		}
		wg.Wait()
	} else {
		for i, r := range w.Repos {
			createPR(i, r)
		}
	}

	return results
}

// ClosePRs closes PRs for all repos on the current branch
func (w *Workspace) ClosePRs() []Result {
	return w.forEach(func(r *repo.Repo) error {
		if !r.IsCloned() {
			return fmt.Errorf("not cloned")
		}
		return r.ClosePR()
	})
}
