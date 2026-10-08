# Mergeish

A Go CLI that allows multiple separate git repositories to act as a single monorepo by managing git state across repositories in sync.

## Installation

### From Source

```bash
go install github.com/willnewby/mergeish/cmd/mergeish@latest
```

### Using Goreleaser

```bash
just release-local
```

This builds the binary and installs it to `~/bin/`.

## Quick Start

1. Initialize a workspace:

```bash
mergeish init
```

2. Edit `mergeish.yml` to add your repositories:

```yaml
repos:
  - url: git@github.com:org/backend.git
    path: services/backend
  - url: git@github.com:org/frontend.git
    path: services/frontend
  - url: git@github.com:org/shared-lib.git
    path: libs/shared

settings:
  default_branch: main
  parallel: true
```

3. Clone all repositories:

```bash
mergeish clone
```

4. Work with your repos as a unified workspace:

```bash
mergeish status              # See status of all repos
mergeish branch feature-x    # Create branch on all repos
mergeish commit -am "msg"    # Commit changes across repos
mergeish push                # Push all repos
```

## Worktree Workspaces

`mergeish new <branch>` creates a full workspace on a branch from anywhere on the filesystem:

```bash
mergeish new my-feature        # create ~/.mergeish/workspaces/my-feature
cd ~/.mergeish/workspaces/my-feature
mergeish status
mergeish ls                    # list workspaces
mergeish rm my-feature         # remove it when done
```

It checks out the base repo (default `git@github.com:willnewby/shinfra.git`) as a git worktree on
`<branch>`, then checks out every repo from its `mergeish.yml` inside it as a worktree on the same
branch. If `<branch>` already exists locally or on the remote it is checked out (tracking the remote);
otherwise it is created from the remote's default branch.

Worktrees share one bare clone per repo, so only the first workspace pays the full clone cost:

```
~/.mergeish/
├── repos/                                  # bare clones, fetched when a workspace is created
│   └── github.com/<owner>/<repo>.git
└── workspaces/
    └── my-feature/                         # base repo worktree (branch my-feature)
        ├── mergeish.yml
        └── <repo>/                         # repo worktree (branch my-feature)
```

Set `MERGEISH_HOME` to move `~/.mergeish`, and `MERGEISH_BASE_REPO` (or `--from`) to use a different
base repo. Repo paths should be git-ignored in the base repo.

A branch can only be checked out in one worktree at a time, so each branch has a single workspace.

## Commands

### `mergeish init`

Initialize a new mergeish workspace by creating a `mergeish.yml` config file.

```bash
mergeish init
mergeish init --config path/to/config.yml
```

### `mergeish new`

Create a worktree workspace at `~/.mergeish/workspaces/<branch>` (see [Worktree Workspaces](#worktree-workspaces)).
Commands listed in `workspace.new.commands` run from the new workspace afterwards.

If `tern` is installed, `new` also creates a tern session named after the workspace
with an agent block (running tern's `agent_command` setting) beside a terminal block, both in the
workspace directory. `--no-tern` skips it, and `mergeish rm` ends the session.

```bash
mergeish new my-feature
mergeish new will/fix-thing                         # directory: will-fix-thing
mergeish new my-feature --from git@github.com:org/other-base.git
mergeish new my-feature --no-tern                   # no tern session
```

### `mergeish rm`

Remove a worktree workspace: every repo worktree, then the workspace itself. Refuses if anything has
uncommitted changes unless `--force` is given.

```bash
mergeish rm my-feature
mergeish rm my-feature -D       # also delete the branch (refuses if it has unpushed commits)
mergeish rm my-feature -D -f    # discard uncommitted changes and unpushed commits
```

### `mergeish ls`

List worktree workspaces and their branches.

```bash
mergeish ls
```

### `mergeish clone`

Clone all configured repositories into the workspace. Inside a worktree workspace, repos are checked
out as worktrees on the workspace's branch instead.

```bash
mergeish clone
```

### `mergeish status`

Show status of all repositories including current branch, ahead/behind counts, and uncommitted changes.

```bash
mergeish status
```

Example output:
```
services/backend:
  branch: main (↑2 ↓1)
  changes: 3 file(s)
    M  src/api.go
    A  src/new.go
    ?? untracked.txt

services/frontend:
  branch: main
  changes: none
```

### `mergeish pull`

Pull latest changes from remote for all repositories.

```bash
mergeish pull
mergeish pull --rebase
```

### `mergeish push`

Push commits to remote for all repositories.

```bash
mergeish push
mergeish push --force    # Requires confirmation
```

### `mergeish branch`

Manage branches across all repositories.

```bash
mergeish branch                      # List current branch for all repos
mergeish branch feature-x            # Create and switch to new branch
mergeish branch --checkout feature-x # Switch to branch (creates if missing)
mergeish branch -d feature-x         # Delete branch from all repos
```

The `--checkout` flag will create the branch in any repo where it doesn't exist.

### `mergeish commit`

Create a commit across all repositories with staged changes.

```bash
mergeish commit -m "Add new feature"
mergeish commit -a -m "Fix bug"      # Stage all changes first
```

Only repos with staged changes will have commits created.

## Configuration

Configuration is stored in `mergeish.yml`:

```yaml
repos:
  - url: git@github.com:org/repo.git   # Git URL (SSH or HTTPS)
    path: local/path                    # Local path relative to config file

settings:
  default_branch: main    # Default branch name (default: main)
  parallel: true          # Run operations in parallel (default: true)

workspace:
  new:
    commands:             # Commands run from the workspace after 'mergeish new'
      - direnv allow
```

### Global Flags

All commands support:

- `-c, --config <path>` - Path to config file (default: searches for `mergeish.yml` in current and parent directories)

## Development

### Prerequisites

- Go 1.21+
- [just](https://github.com/casey/just) (optional, for task running)
- [goreleaser](https://goreleaser.com/) (optional, for releases)

### Building

```bash
just build          # Build binary
just test           # Run tests
just fmt            # Format code
just lint           # Run linter
just clean          # Clean build artifacts
```

### Releasing

```bash
just snapshot       # Build snapshot release
just release-local  # Build and install to ~/bin
just release        # Full release (requires GITHUB_TOKEN)
```

## License

MIT
