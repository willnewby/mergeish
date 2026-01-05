# Mergeish Design Document

A Go CLI that allows multiple separate git repositories to act as a single monorepo by managing git state across repositories in sync.

## Overview

Mergeish provides a unified interface for managing multiple git repositories as if they were a single monorepo. It synchronizes branch state, commits, and remote operations across all configured repositories.

## Configuration

Configuration is stored in `mergeish.yml` at the root of the workspace:

```yaml
# mergeish.yml
repos:
  - url: git@github.com:org/repo-a.git
    path: services/repo-a        # local path relative to workspace root
  - url: git@github.com:org/repo-b.git
    path: libs/repo-b
  - url: https://github.com/org/repo-c.git
    path: tools/repo-c

# Optional settings
settings:
  default_branch: main           # default branch name for new branches
  parallel: true                 # run operations in parallel where possible
```

## CLI Commands

### `mergeish init`

Initialize a new mergeish workspace.

```
mergeish init [--config <path>]
```

- Creates `mergeish.yml` if it doesn't exist
- Validates configuration syntax

### `mergeish clone`

Clone all configured repositories into the workspace.

```
mergeish clone [--config <path>]
```

- Reads `mergeish.yml` for repository list
- Clones each repository to its configured path
- Checks out the default branch on all repos
- Fails fast or continues on error (configurable)

### `mergeish pull`

Pull latest changes from remote for all repositories.

```
mergeish pull [--rebase]
```

- Runs `git pull` (or `git pull --rebase`) on all repos
- Ensures all repos are on the same branch before pulling
- Reports per-repo status

### `mergeish push`

Push commits to remote for all repositories.

```
mergeish push [--force]
```

- Runs `git push` on all repos that have local commits
- Validates branch consistency before pushing
- Supports `--force` for force push (with confirmation)

### `mergeish branch`

Manage branches across all repositories.

```
mergeish branch                    # list current branch for all repos
mergeish branch <name>             # create new branch on all repos
mergeish branch -d <name>          # delete branch on all repos
mergeish branch --checkout <name>  # switch to branch on all repos
```

- Creates/deletes/switches branches atomically across repos
- Validates branch exists (for checkout/delete) or doesn't exist (for create)

### `mergeish commit`

Create a commit across all repositories with changes.

```
mergeish commit -m "message"
mergeish commit -a -m "message"    # stage all changes first
```

- Commits staged changes in each repo that has changes
- Uses the same commit message across all repos
- Reports which repos had changes committed

### `mergeish status`

Show status of all repositories.

```
mergeish status
```

- Shows current branch for each repo
- Shows uncommitted changes per repo
- Shows ahead/behind status relative to remote

## Architecture

```
mergeish/
├── cmd/
│   └── mergeish/
│       └── main.go              # CLI entry point
├── internal/
│   ├── config/
│   │   └── config.go            # YAML config parsing
│   ├── git/
│   │   └── git.go               # Git operations wrapper
│   ├── repo/
│   │   └── repo.go              # Repository management
│   └── workspace/
│       └── workspace.go         # Workspace orchestration
├── mergeish.yml                  # Example config
├── go.mod
└── go.sum
```

### Key Components

**Config (`internal/config`)**: Parses and validates `mergeish.yml`. Handles defaults and validation.

**Git (`internal/git`)**: Wraps git CLI commands. Provides typed responses and error handling. Does not maintain state.

**Repo (`internal/repo`)**: Represents a single repository. Combines config with git operations.

**Workspace (`internal/workspace`)**: Orchestrates operations across all repos. Handles parallel execution and aggregates results.

## Error Handling

- Operations are atomic where possible
- On failure during multi-repo operations:
  - Report which repos succeeded/failed
  - Provide clear error messages per repo
  - Exit with non-zero status if any repo fails
- Pre-validate state before destructive operations

## Branch Synchronization

The core invariant: all repos should be on the same branch name.

- `mergeish status` warns if repos are on different branches
- `mergeish branch --checkout` fails if target branch doesn't exist in all repos
- `mergeish branch <name>` creates branch in all repos from their current HEAD
- Operations like `pull`, `push`, `commit` verify branch consistency first

## Future Considerations

- Support for repo-specific branch mappings
- Hooks for pre/post operations
- Selective operations on subset of repos
- Integration with CI/CD pipelines
- Support for git subcommands passthrough
