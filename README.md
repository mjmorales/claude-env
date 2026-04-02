# claude-env

Manage multiple Claude Code OAuth sessions with easy switching and declarative shared state. Works like `rbenv` or `pyenv`, but for Claude Code accounts.

Each environment gets its own directory at `~/.claude-envs/<name>/` used as `CLAUDE_CONFIG_DIR`. Switching environments changes which directory Claude Code reads auth and state from. No credential juggling — just directory isolation.

## Prerequisites

- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and available as `claude` in `PATH`
- Go 1.25+ (to build from source)
- macOS (the `reset` command uses macOS Keychain; all other commands are platform-agnostic)

## Installation

```sh
go install github.com/mjmorales/claude-env/cmd/claude-env@latest
```

This installs the `claude-env` binary to `$GOPATH/bin` (or `$HOME/go/bin` if `GOPATH` is unset). Make sure this directory is in your `PATH`.

To build from a local clone instead:

```sh
git clone https://github.com/mjmorales/claude-env.git
cd claude-env
go install ./cmd/claude-env
```

## Quick Start

```sh
# 1. Initialize — adopts your existing ~/.claude credentials as "default"
claude-env init

# 3. Add shell integration (wraps the claude command automatically)
echo 'eval "$(claude-env shell-init)"' >> ~/.zshrc
source ~/.zshrc

# 4. Add a second account and authenticate it
claude-env add work
claude-env login work

# 5. Switch to it globally
claude-env use work
```

## How It Works

Claude Code reads configuration and auth tokens from `CLAUDE_CONFIG_DIR` (defaults to `~/.claude/`). `claude-env` gives each account its own directory under `~/.claude-envs/` and manages which one is active. The shell integration shim sets `CLAUDE_CONFIG_DIR` transparently on every `claude` invocation.

Resolution order for the active environment:

1. `.claude-env` file in the current directory or any parent directory (local pin)
2. Global setting in `~/.claude-envs/config.toml`

## Shell Integration

Add to `~/.zshrc` or `~/.bashrc`:

```sh
eval "$(claude-env shell-init)"
```

This installs a shell function that wraps `claude`, resolving the active environment and setting `CLAUDE_CONFIG_DIR` automatically. The underlying `claude` binary is still called — no proxying or subprocess overhead.

Tab completion is available separately:

```sh
# Zsh
eval "$(claude-env completion zsh)"

# Bash
eval "$(claude-env completion bash)"
```

## Commands Reference

### Setup

| Command | Description |
|---------|-------------|
| `claude-env init` | Create `~/.claude-envs/`, adopt existing `~/.claude/.claude.json` as the `default` environment, copy `settings.json` and `CLAUDE.md` from `~/.claude/`, and set `default` as the global active environment. Run once after install. |
| `claude-env reset` | Restore the active environment's credentials to macOS Keychain, then delete `~/.claude-envs/`. Removes all claude-env state. See [Uninstall](#uninstall). |

### Managing Environments

| Command | Alias | Description |
|---------|-------|-------------|
| `claude-env add <name>` | | Register a new environment. Creates `~/.claude-envs/<name>/` and copies `settings.json` and `CLAUDE.md` from `~/.claude/`. Does not authenticate — run `claude-env login <name>` afterward. |
| `claude-env remove <name>` | `rm` | Delete an environment and its directory. Refuses to remove the current global environment — switch first. |
| `claude-env list` | `ls` | List all environments. Active environment is marked with `*`. Shows `global`/`local` tags and shared resource count. |

### Switching

| Command | Alias | Description |
|---------|-------|-------------|
| `claude-env use <name>` | `global` | Set the global active environment. Persisted to `config.toml`. All new shells pick this up. |
| `claude-env local <name>` | | Pin an environment to the current directory by writing a `.claude-env` file. Overrides the global setting for any shell in this directory tree. |
| `claude-env current` | | Print the active environment name and whether it is resolved from a local pin or the global setting. |

### Authentication

| Command | Description |
|---------|-------------|
| `claude-env login [name]` | Run `claude auth login` with `CLAUDE_CONFIG_DIR` pointed at the named environment. Defaults to the current active environment. Opens the browser OAuth flow and patches `.claude.json` with onboarding flags so interactive sessions start cleanly. |
| `claude-env auth-status [name]` | Run `claude auth status` for the named environment (defaults to current). Shows whether the session is authenticated and its subscription type. |

**Important:** Before running `claude-env login` for a new environment, make sure you are signed into the correct account at [claude.ai](https://claude.ai/new). The OAuth flow in the browser will attach to whichever account is currently active. If you need to switch accounts, sign out at claude.ai first, then sign in with the desired account before running `login`.

### Utilities

| Command | Description |
|---------|-------------|
| `claude-env status` | Show the active environment, its config directory path, and symlink health for any declared shared resources. |
| `claude-env exec <command> [args...]` | Run a command with `CLAUDE_CONFIG_DIR` set to the active environment, like `pyenv exec`. Replaces the current process. Useful in scripts or CI where the shell shim is not loaded. |
| `claude-env shell-init` | Print the shell function that wraps `claude`. Pipe to `eval` in your shell profile. |
| `claude-env completion <shell>` | Generate tab-completion script for `bash`, `zsh`, `fish`, or `powershell`. |
| `claude-env version` | Print version, commit hash, and build date. |

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config <path>` | `~/.claude-envs/config.toml` | Use an alternate config file. |
| `--dry-run` | `false` | Preview filesystem changes without writing anything. Applies to all mutating commands. |

## Shared State

Environments can declare shared resources — agents, skills, commands, or plugins — from a pool at `~/.claude-envs/pool/`. Declared resources are symlinked into the environment's config directory.

### Setup

Place shared resources inside `~/.claude-envs/pool/`:

```
~/.claude-envs/
  pool/
    agents/
      my-agent.md
    commands/
      deploy.md
```

Declare which resources an environment shares in `~/.claude-envs/config.toml`:

```toml
global = "work"

[environments.work]
shared = ["agents/my-agent.md", "commands/deploy.md"]

[environments.personal]
shared = ["commands/deploy.md"]
```

### Reconciliation

The symlink reconciler:

- Creates symlinks in `~/.claude-envs/<name>/` for every declared resource
- Removes managed symlinks for resources no longer declared
- Skips resources where a real (non-symlink) file already exists at the target path
- Logs a warning for pool resources that are declared but missing from disk

Managed symlinks are tracked in `~/.claude-envs/.managed-symlinks`. Do not edit this file manually.

### Symlink Status Values

| Status | Meaning |
|--------|---------|
| `ok` | Symlink exists and points to the expected pool path |
| `missing` | Symlink was previously managed but is no longer present on disk |
| `stale` | Symlink exists but points to a different path than expected |
| `conflict` | A real file (not a symlink) exists at the target location |

Run `claude-env status` to see the current symlink health for the active environment.

## Configuration

`~/.claude-envs/config.toml` is the single source of truth.

### Full Example

```toml
# The globally active environment
global = "work"

# An environment with no shared resources
[environments.personal]

# An environment with shared resources from the pool
[environments.work]
shared = [
  "agents/code-reviewer.md",
  "commands/deploy.md",
]

# An environment with a settings override file
[environments.staging]
shared = ["agents/code-reviewer.md"]
settings_override = "/Users/alice/.claude-envs/pool/staging-settings.json"
```

### Filesystem Layout

```
~/.claude-envs/
  config.toml           # Top-level config
  .managed-symlinks     # Symlink lock file (managed automatically)
  pool/                 # Shared resource pool
    agents/
    commands/
  default/              # Environment config dirs (one per env)
    .claude.json        # Auth + onboarding state
    settings.json       # Copied from ~/.claude/ on init/add
    CLAUDE.md           # Copied from ~/.claude/ on init/add
  work/
    .claude.json
    settings.json
    agents/             # Symlinked from pool when declared
```

## Local Pins

Pin an environment to a project directory so every shell in that tree uses it automatically:

```sh
cd ~/projects/client-x
claude-env local work
```

This writes `~/projects/client-x/.claude-env` containing the environment name. To remove a local pin, delete the `.claude-env` file:

```sh
rm ~/projects/client-x/.claude-env
```

## Using in CI or Scripts

When the shell shim is not available, use `exec` to run commands under a specific environment:

```sh
claude-env use personal
claude-env exec claude --print "Summarize this PR"
```

Or set `CLAUDE_CONFIG_DIR` directly:

```sh
export CLAUDE_CONFIG_DIR="$(claude-env config-dir)"
claude --print "Summarize this PR"
```

## Uninstall

```sh
# 1. Restore credentials and remove ~/.claude-envs/
claude-env reset

# 2. Remove the shell integration line from your profile
#    Delete: eval "$(claude-env shell-init)"
#    Delete: eval "$(claude-env completion zsh)"   (if added)

# 3. Restart your shell
exec $SHELL

# 4. Remove the binary
rm "$(which claude-env)"
```

`reset` copies the active environment's `.claude.json` back to the macOS Keychain so Claude Code continues to work after removal. If you want to preserve credentials for a non-active environment, run `claude-env use <name>` before `reset`.
