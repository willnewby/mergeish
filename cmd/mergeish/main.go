package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/willnewby/mergeish/internal/ai"
	"github.com/willnewby/mergeish/internal/config"
	"github.com/willnewby/mergeish/internal/store"
	"github.com/willnewby/mergeish/internal/tern"
	"github.com/willnewby/mergeish/internal/workspace"
)

var (
	// Set by goreleaser ldflags
	version = "dev"
	commit  = "none"
	date    = "unknown"

	configPath string
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "mergeish",
		Short:   "Manage multiple git repos as a single monorepo",
		Long:    "Mergeish allows multiple separate git repositories to act as a single monorepo by managing git state across repositories in sync.",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config file")

	rootCmd.AddCommand(
		initCmd(),
		newCmd(),
		rmCmd(),
		lsCmd(),
		cloneCmd(),
		pullCmd(),
		pushCmd(),
		branchCmd(),
		commitCmd(),
		statusCmd(),
		gitCmd(),
		prCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getConfigPath() (string, error) {
	if configPath != "" {
		return configPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	return config.FindConfigFile(cwd)
}

func loadWorkspace() (*workspace.Workspace, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	return workspace.Load(path)
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new mergeish workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.DefaultConfigFile
			if configPath != "" {
				path = configPath
			}

			if _, err := os.Stat(path); err == nil {
				fmt.Printf("Config file %s already exists\n", path)
				return nil
			}

			cfg := config.DefaultConfig()
			if err := cfg.Save(path); err != nil {
				return err
			}

			fmt.Printf("Created %s\n", path)
			fmt.Println("Add your repositories to the config file and run 'mergeish clone'")
			return nil
		},
	}
}

const (
	// defaultBaseRepo is the workspace repo checked out by 'mergeish new'
	defaultBaseRepo = "git@github.com:willnewby/shinfra.git"
	// baseRepoEnv overrides defaultBaseRepo
	baseRepoEnv = "MERGEISH_BASE_REPO"
)

func newCmd() *cobra.Command {
	var from string
	var noTern bool

	cmd := &cobra.Command{
		Use:   "new <branch>",
		Short: "Create a worktree workspace with matching branches across all repos",
		Long: `Create a new workspace at ~/.mergeish/workspaces/<branch>/ (or $MERGEISH_HOME/workspaces/).

The base repo (default: ` + defaultBaseRepo + `, override with --from or $` + baseRepoEnv + `)
is checked out there as a git worktree on <branch>, then every repo in its mergeish.yml is
checked out inside it as a worktree on the same branch. Worktrees share bare clones kept
under ~/.mergeish/repos/, so only the first workspace pays the full clone cost.

An existing local or remote <branch> is checked out; otherwise it is created from the
remote's default branch. Commands listed in workspace.new.commands then run from the
new workspace directory.

If tern is installed, a tern session named after the workspace is created with an agent
block (tern's agent_command) and a terminal block, both in the workspace directory.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]

			if from == "" {
				from = os.Getenv(baseRepoEnv)
			}
			if from == "" {
				from = defaultBaseRepo
			}

			destDir, err := store.WorkspaceDir(branch)
			if err != nil {
				return err
			}
			if _, err := os.Stat(destDir); err == nil {
				return fmt.Errorf("workspace %s already exists", destDir)
			}

			fmt.Printf("Fetching %s...\n", from)
			storePath, err := store.Ensure(from)
			if err != nil {
				return err
			}
			if err := store.AddWorktree(storePath, destDir, branch, config.DefaultConfig().Settings.DefaultBranch); err != nil {
				return err
			}
			fmt.Printf("  ✓ %s\n\n", destDir)

			ws, err := workspace.Load(filepath.Join(destDir, config.DefaultConfigFile))
			if err != nil {
				return err
			}
			if err := cloneWorkspace(ws); err != nil {
				return err
			}

			// Run post-creation commands
			for _, c := range ws.Config.Workspace.New.Commands {
				fmt.Printf("\nRunning: %s\n", c)
				parts := strings.Fields(c)
				run := exec.Command(parts[0], parts[1:]...)
				run.Dir = destDir
				run.Stdout = os.Stdout
				run.Stderr = os.Stderr
				if err := run.Run(); err != nil {
					return fmt.Errorf("command %q failed: %w", c, err)
				}
			}

			if !noTern {
				createTernSession(filepath.Base(destDir), destDir)
			}

			fmt.Printf("\nWorkspace ready: %s\n", destDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "base repo URL (default: $"+baseRepoEnv+" or "+defaultBaseRepo+")")
	cmd.Flags().BoolVar(&noTern, "no-tern", false, "don't create a tern session")
	return cmd
}

// createTernSession opens a tern session for a workspace. Failures are reported
// but don't fail the command, since the workspace itself is already created.
func createTernSession(name, dir string) {
	if !tern.Available() {
		return
	}

	fmt.Printf("\nCreating tern session %s...\n", name)
	exists, err := tern.SessionExists(name)
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return
	}
	if exists {
		fmt.Printf("  - session %s already exists\n", name)
		return
	}
	if err := tern.NewWorkspaceSession(name, dir); err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return
	}
	fmt.Printf("  ✓ agent (%s) and terminal blocks\n", tern.AgentCommand())
}

func rmCmd() *cobra.Command {
	var force bool
	var deleteBranch bool

	cmd := &cobra.Command{
		Use:   "rm <branch>",
		Short: "Remove a worktree workspace created by 'mergeish new'",
		Long: `Remove the workspace for <branch>: every repo worktree inside it, then the workspace itself.

Refuses if any repo has uncommitted changes, unless --force is given.
Also ends the workspace's tern session, if there is one.
With --delete-branch, also deletes the branch from each repo's store; branches with
commits that are not on the remote are kept unless --force is given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := store.WorkspaceDir(args[0])
			if err != nil {
				return err
			}
			if _, err := os.Stat(dir); err != nil {
				return fmt.Errorf("no workspace for %s at %s", args[0], dir)
			}

			rootStore := store.StoreFor(dir)
			if rootStore == "" {
				return fmt.Errorf("%s is not a mergeish worktree workspace", dir)
			}

			ws, err := workspace.Load(filepath.Join(dir, config.DefaultConfigFile))
			if err != nil {
				return err
			}

			type target struct {
				name, dir, store, branch string
			}

			// Collect worktrees, deepest (repos) first and the workspace root last,
			// since removing the root also deletes the git-ignored repo directories in it
			var targets []target
			for _, r := range ws.Repos {
				if !r.IsCloned() {
					continue
				}
				s := r.Store()
				if s == "" {
					return fmt.Errorf("%s is a full clone, not a worktree; remove it manually first", r.Name())
				}
				branch, err := r.CurrentBranch()
				if err != nil {
					return fmt.Errorf("%s: %w", r.Name(), err)
				}
				targets = append(targets, target{r.Name(), r.FullPath, s, branch})
			}
			rootBranch, err := ws.RootBranch()
			if err != nil {
				return err
			}
			targets = append(targets, target{filepath.Base(dir), dir, rootStore, rootBranch})

			// Check everything before removing anything
			if !force {
				var problems []string
				for _, t := range targets {
					dirty, err := store.IsDirty(t.dir)
					if err != nil {
						return fmt.Errorf("%s: %w", t.name, err)
					}
					if dirty {
						problems = append(problems, fmt.Sprintf("%s: uncommitted changes", t.name))
					}
					if deleteBranch && store.HasUnpushedCommits(t.store, t.branch) {
						problems = append(problems, fmt.Sprintf("%s: %s has unpushed commits", t.name, t.branch))
					}
				}
				if len(problems) > 0 {
					for _, p := range problems {
						fmt.Printf("  ✗ %s\n", p)
					}
					return fmt.Errorf("refusing to remove workspace (use --force to discard)")
				}
			}

			fmt.Printf("Removing workspace %s...\n", dir)
			for i, t := range targets {
				if err := store.RemoveWorktree(t.store, t.dir, force); err != nil {
					fmt.Printf("  ✗ %s: %v\n", t.name, err)
					if i < len(targets)-1 {
						return fmt.Errorf("failed to remove some repositories; workspace root kept")
					}
					return fmt.Errorf("failed to remove workspace root")
				}
				fmt.Printf("  ✓ %s\n", t.name)
			}

			if tern.Available() {
				name := filepath.Base(dir)
				if exists, _ := tern.SessionExists(name); exists {
					if err := tern.KillSession(name); err != nil {
						fmt.Printf("  ✗ tern session %s: %v\n", name, err)
					} else {
						fmt.Printf("  ✓ tern session %s\n", name)
					}
				}
			}

			if deleteBranch {
				fmt.Println("\nDeleting branches...")
				hasErrors := false
				for _, t := range targets {
					if err := store.DeleteBranch(t.store, t.branch); err != nil {
						fmt.Printf("  ✗ %s: %v\n", t.name, err)
						hasErrors = true
					} else {
						fmt.Printf("  ✓ %s: %s\n", t.name, t.branch)
					}
				}
				if hasErrors {
					return fmt.Errorf("failed to delete some branches")
				}
			}

			fmt.Println("Done!")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "remove even with uncommitted changes or unpushed commits")
	cmd.Flags().BoolVarP(&deleteBranch, "delete-branch", "D", false, "also delete the branch from each repo")
	return cmd
}

func lsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List worktree workspaces created by 'mergeish new'",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := store.WorkspacesDir()
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(dir)
			if err != nil && !os.IsNotExist(err) {
				return err
			}

			found := 0
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				path := filepath.Join(dir, e.Name())
				if store.StoreFor(path) == "" {
					fmt.Printf("  %s (not a worktree)\n", path)
					continue
				}
				ws, err := workspace.Load(filepath.Join(path, config.DefaultConfigFile))
				if err != nil {
					fmt.Printf("  %s: error: %v\n", path, err)
					continue
				}
				branch, err := ws.RootBranch()
				if err != nil {
					fmt.Printf("  %s: error: %v\n", path, err)
					continue
				}
				fmt.Printf("  %s  %s\n", branch, path)
				found++
			}

			if found == 0 {
				fmt.Println("No workspaces. Create one with 'mergeish new <branch>'")
			}
			return nil
		},
	}
}

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone",
		Short: "Clone all configured repositories",
		Long: `Clone all configured repositories into the workspace.

In a workspace created by 'mergeish new' (the workspace root is a git worktree),
repos are checked out as worktrees on the workspace's branch instead of cloned.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			return cloneWorkspace(ws)
		},
	}
}

// cloneWorkspace clones every repo, or checks each out as a worktree
// on the root's branch when the workspace is itself a worktree
func cloneWorkspace(ws *workspace.Workspace) error {
	var results []workspace.Result
	if ws.IsWorktree() {
		branch, err := ws.RootBranch()
		if err != nil {
			return err
		}
		fmt.Printf("Checking out %s in all repositories...\n", branch)
		results = ws.CloneWorktrees(branch)
	} else {
		fmt.Println("Cloning repositories...")
		results = ws.Clone()
	}

	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
			hasErrors = true
		} else if r.Repo.IsCloned() {
			fmt.Printf("  ✓ %s\n", r.Repo.Name())
		}
	}

	if hasErrors {
		return fmt.Errorf("some repositories failed to clone")
	}

	fmt.Println("Done!")
	return nil
}

func pullCmd() *cobra.Command {
	var rebase bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull changes for all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			branch, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				fmt.Println("Warning: repositories are on different branches")
			}

			fmt.Printf("Pulling %s...\n", branch)
			results := ws.Pull(rebase)

			hasErrors := false
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					hasErrors = true
				} else {
					fmt.Printf("  ✓ %s\n", r.Repo.Name())
				}
			}

			if hasErrors {
				return fmt.Errorf("some repositories failed to pull")
			}

			fmt.Println("Done!")
			return nil
		},
	}

	cmd.Flags().BoolVar(&rebase, "rebase", false, "use rebase instead of merge")
	return cmd
}

func pushCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push changes for all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			branch, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				return fmt.Errorf("repositories are on different branches, cannot push")
			}

			if force {
				fmt.Print("Force push? This may overwrite remote changes. [y/N]: ")
				var response string
				if _, err := fmt.Scanln(&response); err != nil || (response != "y" && response != "Y") {
					fmt.Println("Aborted")
					return nil
				}
			}

			fmt.Printf("Pushing %s...\n", branch)
			results := ws.Push(force)

			hasErrors := false
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					hasErrors = true
				} else {
					fmt.Printf("  ✓ %s\n", r.Repo.Name())
				}
			}

			if hasErrors {
				return fmt.Errorf("some repositories failed to push")
			}

			fmt.Println("Done!")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "force push")
	return cmd
}

func branchCmd() *cobra.Command {
	var deleteBranch bool
	var checkout bool

	cmd := &cobra.Command{
		Use:   "branch [name]",
		Short: "Manage branches across all repositories",
		Long: `Manage branches across all repositories.

Without arguments, lists current branch for each repo.
With a name argument, creates a new branch on all repos.
With -d flag, deletes the branch from all repos.
With --checkout flag, switches to the branch on all repos.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// No args: list branches
			if len(args) == 0 && !deleteBranch && !checkout {
				return listBranches(ws)
			}

			if len(args) == 0 {
				return fmt.Errorf("branch name required")
			}

			branchName := args[0]

			if deleteBranch {
				return deleteBranchOp(ws, branchName)
			}

			if checkout {
				return checkoutBranch(ws, branchName)
			}

			// Create new branch
			return createBranch(ws, branchName)
		},
	}

	cmd.Flags().BoolVarP(&deleteBranch, "delete", "d", false, "delete the branch")
	cmd.Flags().BoolVar(&checkout, "checkout", false, "switch to the branch")
	return cmd
}

func listBranches(ws *workspace.Workspace) error {
	results := ws.Status()

	fmt.Println("Current branches:")
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  %s: error: %v\n", r.Repo.Name(), r.Error)
		} else {
			fmt.Printf("  %s: %s\n", r.Repo.Name(), r.Status.Branch)
		}
	}

	return nil
}

func createBranch(ws *workspace.Workspace, name string) error {
	fmt.Printf("Creating branch %s...\n", name)
	results := ws.CreateBranch(name)

	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
			hasErrors = true
		} else {
			fmt.Printf("  ✓ %s\n", r.Repo.Name())
		}
	}

	if hasErrors {
		return fmt.Errorf("failed to create branch on some repositories")
	}

	fmt.Println("Done!")
	return nil
}

func deleteBranchOp(ws *workspace.Workspace, name string) error {
	fmt.Printf("Deleting branch %s...\n", name)
	results := ws.DeleteBranch(name)

	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
			hasErrors = true
		} else {
			fmt.Printf("  ✓ %s\n", r.Repo.Name())
		}
	}

	if hasErrors {
		return fmt.Errorf("failed to delete branch on some repositories")
	}

	fmt.Println("Done!")
	return nil
}

func checkoutBranch(ws *workspace.Workspace, name string) error {
	fmt.Printf("Switching to branch %s...\n", name)
	results := ws.Checkout(name)

	hasErrors := false
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
			hasErrors = true
		} else {
			fmt.Printf("  ✓ %s\n", r.Repo.Name())
		}
	}

	if hasErrors {
		return fmt.Errorf("failed to switch branch on some repositories")
	}

	fmt.Println("Done!")
	return nil
}

func commitCmd() *cobra.Command {
	var message string
	var addAll bool

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit changes across all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if message == "" {
				return fmt.Errorf("commit message required (-m)")
			}

			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			_, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				return fmt.Errorf("repositories are on different branches, cannot commit")
			}

			fmt.Println("Committing changes...")
			results := ws.Commit(message, addAll)

			committed := 0
			hasErrors := false
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					hasErrors = true
				} else {
					// Check if we actually committed something
					status, _ := r.Repo.Status()
					if status != nil && !status.HasChanges {
						committed++
						fmt.Printf("  ✓ %s (committed)\n", r.Repo.Name())
					} else {
						fmt.Printf("  - %s (no changes)\n", r.Repo.Name())
					}
				}
			}

			if hasErrors {
				return fmt.Errorf("some repositories failed to commit")
			}

			if committed == 0 {
				fmt.Println("No changes to commit")
			} else {
				fmt.Printf("Committed to %d repositories\n", committed)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message")
	cmd.Flags().BoolVarP(&addAll, "all", "a", false, "stage all changes before committing")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			results := ws.Status()

			// Check branch consistency
			branches := make(map[string]int)
			for _, r := range results {
				if r.Status != nil {
					branches[r.Status.Branch]++
				}
			}

			if len(branches) > 1 {
				fmt.Println("⚠ Warning: repositories are on different branches")
				fmt.Println()
			}

			for _, r := range results {
				fmt.Printf("%s:\n", r.Repo.Name())

				if r.Error != nil {
					fmt.Printf("  error: %v\n", r.Error)
					continue
				}

				s := r.Status
				fmt.Printf("  branch: %s", s.Branch)

				// Show ahead/behind
				if s.Ahead > 0 || s.Behind > 0 {
					fmt.Printf(" (")
					if s.Ahead > 0 {
						fmt.Printf("↑%d", s.Ahead)
					}
					if s.Behind > 0 {
						if s.Ahead > 0 {
							fmt.Printf(" ")
						}
						fmt.Printf("↓%d", s.Behind)
					}
					fmt.Printf(")")
				}
				fmt.Println()

				// Show changes
				if s.HasChanges {
					fmt.Printf("  changes: %d file(s)\n", len(s.Files))
					for _, f := range s.Files {
						fmt.Printf("    %s %s\n", f.Status, f.Path)
					}
				} else {
					fmt.Println("  changes: none")
				}

				fmt.Println()
			}

			return nil
		},
	}
}

func gitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "git [args...]",
		Short: "Run a git command across all repositories",
		Long: `Run an arbitrary git command across all configured repositories.

Examples:
  mergeish git status
  mergeish git log --oneline -5
  mergeish git remote -v
  mergeish git fetch --all`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("git command required")
			}

			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			fmt.Printf("Running: git %s\n\n", strings.Join(args, " "))
			results := ws.RunGit(args)

			hasErrors := false
			for _, r := range results {
				fmt.Printf("── %s ──\n", r.Repo.Name())

				if r.Error != nil {
					hasErrors = true
					if r.Stderr != "" {
						fmt.Print(r.Stderr)
					} else {
						fmt.Printf("error: %v\n", r.Error)
					}
				} else {
					if r.Stdout != "" {
						fmt.Print(r.Stdout)
					}
					if r.Stderr != "" {
						fmt.Print(r.Stderr)
					}
					if r.Stdout == "" && r.Stderr == "" {
						fmt.Println("(no output)")
					}
				}
				fmt.Println()
			}

			if hasErrors {
				return fmt.Errorf("command failed on some repositories")
			}

			return nil
		},
	}
}

func prCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Manage pull requests across all repositories",
		Long: `Manage GitHub pull requests across all configured repositories.

Requires the GitHub CLI (gh) to be installed and authenticated.`,
	}

	cmd.AddCommand(prStatusCmd())
	cmd.AddCommand(prCreateCmd())
	cmd.AddCommand(prCloseCmd())
	cmd.AddCommand(prOpenCmd())

	return cmd
}

func prStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show PR status for all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			branch, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				fmt.Println("⚠ Warning: repositories are on different branches")
				fmt.Println()
			} else {
				fmt.Printf("Branch: %s\n\n", branch)
			}

			results := ws.GetPRs()

			for _, r := range results {
				fmt.Printf("%s: ", r.Repo.Name())

				if r.Error != nil {
					fmt.Printf("error: %v\n", r.Error)
					continue
				}

				if r.PR == nil {
					fmt.Println("no PR")
				} else {
					fmt.Printf("#%d %s (%s)\n", r.PR.Number, r.PR.Title, r.PR.State)
					fmt.Printf("  %s\n", r.PR.URL)
				}
			}

			return nil
		},
	}
}

func prCreateCmd() *cobra.Command {
	var title string
	var body string
	var base string
	var infer bool
	var useAI bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create pull requests for all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" && !useAI {
				return fmt.Errorf("title required (-t), or use --ai to generate one")
			}

			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			branch, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				return fmt.Errorf("repositories are on different branches, cannot create PRs")
			}

			// Generate title/body with an AI CLI if requested
			if useAI && (title == "" || body == "") {
				genTitle, genBody, err := generatePRWithAI(ws, branch, base)
				if err != nil {
					return err
				}
				if title == "" {
					title = genTitle
				}
				if body == "" {
					body = genBody
				}
			}

			// Infer body from commits if requested
			if infer && body == "" {
				body = inferBodyFromCommits(ws, base)
			}

			fmt.Printf("Creating PRs for branch %s...\n\n", branch)
			results := ws.CreatePRs(title, body, base)

			hasErrors := false
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					hasErrors = true
				} else if r.PR != nil {
					if r.Existed {
						fmt.Printf("  - %s: already exists %s\n", r.Repo.Name(), r.PR.URL)
					} else {
						fmt.Printf("  ✓ %s: %s\n", r.Repo.Name(), r.PR.URL)
					}
				}
			}

			if hasErrors {
				return fmt.Errorf("failed to create PRs for some repositories")
			}

			fmt.Println("\nDone!")
			return nil
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "PR title (required unless --ai)")
	cmd.Flags().StringVarP(&body, "body", "b", "", "PR body/description")
	cmd.Flags().StringVar(&base, "base", "", "base branch (default: repo default)")
	cmd.Flags().BoolVar(&infer, "infer", false, "infer PR body from commit messages")
	cmd.Flags().BoolVar(&useAI, "ai", false, "generate PR title and body using an AI CLI (claude or pi)")

	return cmd
}

// maxDiffChars caps how much of each repo's diff is sent to the AI CLI
const maxDiffChars = 20000

// generatePRWithAI generates a PR title and body from branch context using an AI CLI
func generatePRWithAI(ws *workspace.Workspace, branch, base string) (string, string, error) {
	cli, err := ai.FindCLI()
	if err != nil {
		return "", "", err
	}

	context := buildPRContext(ws, branch, base)
	if context == "" {
		return "", "", fmt.Errorf("no commits or diffs found to generate PR content from")
	}

	fmt.Printf("Generating PR title and body with %s...\n", cli)
	content, err := ai.GeneratePR(cli, context)
	if err != nil {
		return "", "", err
	}

	fmt.Printf("  title: %s\n\n", content.Title)
	return content.Title, content.Body, nil
}

// buildPRContext collects branch name, commit messages, and diffs across all repos
func buildPRContext(ws *workspace.Workspace, branch, base string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Branch: %s\n", branch)

	hasContent := false
	for _, r := range ws.Repos {
		if !r.IsCloned() {
			continue
		}

		commits, err := r.GetBranchCommits(base)
		if err != nil || len(commits) == 0 {
			continue
		}
		hasContent = true

		fmt.Fprintf(&sb, "\n## Repository: %s\n\nCommits:\n", r.Name())
		for _, c := range commits {
			fmt.Fprintf(&sb, "- %s\n", c)
		}

		diff, err := r.GetBranchDiff(base)
		if err != nil || diff == "" {
			continue
		}
		if len(diff) > maxDiffChars {
			diff = diff[:maxDiffChars] + "\n... (diff truncated)"
		}
		fmt.Fprintf(&sb, "\nDiff:\n```\n%s\n```\n", diff)
	}

	if !hasContent {
		return ""
	}
	return sb.String()
}

// inferBodyFromCommits generates a PR body from commit messages across all repos
func inferBodyFromCommits(ws *workspace.Workspace, base string) string {
	var allCommits []string
	seen := make(map[string]bool)

	for _, r := range ws.Repos {
		if !r.IsCloned() {
			continue
		}

		commits, err := r.GetBranchCommits(base)
		if err != nil {
			continue
		}

		for _, commit := range commits {
			if commit != "" && !seen[commit] {
				seen[commit] = true
				allCommits = append(allCommits, commit)
			}
		}
	}

	if len(allCommits) == 0 {
		return ""
	}

	var body strings.Builder
	body.WriteString("## Changes\n\n")
	for _, commit := range allCommits {
		body.WriteString("- ")
		body.WriteString(commit)
		body.WriteString("\n")
	}

	return body.String()
}

func prCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "Close pull requests for all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			// Check branch consistency
			branch, consistent, err := ws.CheckBranchConsistency()
			if err != nil {
				return err
			}
			if !consistent {
				fmt.Println("⚠ Warning: repositories are on different branches")
			}

			fmt.Printf("Closing PRs for branch %s...\n\n", branch)
			results := ws.ClosePRs()

			hasErrors := false
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					hasErrors = true
				} else {
					fmt.Printf("  ✓ %s\n", r.Repo.Name())
				}
			}

			if hasErrors {
				return fmt.Errorf("failed to close PRs for some repositories")
			}

			fmt.Println("\nDone!")
			return nil
		},
	}
}

func prOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open pull requests in web browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := loadWorkspace()
			if err != nil {
				return err
			}

			results := ws.GetPRs()

			opened := 0
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %s: %v\n", r.Repo.Name(), r.Error)
					continue
				}

				if r.PR == nil {
					fmt.Printf("  - %s: no PR\n", r.Repo.Name())
					continue
				}

				if err := openBrowser(r.PR.URL); err != nil {
					fmt.Printf("  ✗ %s: failed to open browser: %v\n", r.Repo.Name(), err)
					continue
				}

				fmt.Printf("  ✓ %s: %s\n", r.Repo.Name(), r.PR.URL)
				opened++
			}

			if opened == 0 {
				fmt.Println("\nNo PRs to open")
			} else {
				fmt.Printf("\nOpened %d PR(s) in browser\n", opened)
			}

			return nil
		},
	}
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}
