# claude-env

Manage multiple Claude Code OAuth accounts with easy switching and declarative shared state. Works like `rbenv` or `pyenv`, but for Claude Code accounts.

Each environment gets its own directory at `~/.claude-envs/<name>/` used as `CLAUDE_CONFIG_DIR`, and each carries its **own OAuth token** as a `.credentials.json` file inside that directory. Switching environments changes which directory — and which token — Claude Code authenticates from. The token is an explicit, portable file you can import, export, copy between environments, and back up — no opaque, per-machine credential juggling.

## Prerequisites

- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and available as `claude` in `PATH`
- Go 1.25+ (to build from source)
- macOS, Linux, or Windows. Token storage is a plain `.credentials.json` file on every platform; on macOS, `login` additionally captures the token Claude Code writes to the Keychain into that file.

## Installation

### Homebrew

```sh
brew install mjmorales/tap/claude-env
```

### Go

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

## Upgrading from an earlier version

This version makes each environment's token an explicit `.credentials.json` file (the "token-file model"). Earlier versions relied on Claude Code's per-directory macOS Keychain entries, which are opaque and break when a directory moves. The upgrade is a **clean break**: run each environment's login once so its token is captured into a file —

```sh
claude-env login default
claude-env login work
# ...or, without a browser, import a token directly:
claude-env import work --setup-token
```

After that, `claude-env auth-status <name>` should report `Authenticated: yes`. Your environments, shared resources, and config are otherwise unchanged.

## How It Works

Claude Code reads configuration from `CLAUDE_CONFIG_DIR` (defaults to `~/.claude/`) and authenticates from an OAuth token. When a `.credentials.json` file is present in the config directory, Claude Code authenticates from it directly (on macOS this takes precedence over the Keychain; on Linux/Windows it is the native store).

`claude-env` gives each account its own directory under `~/.claude-envs/` containing its own `.credentials.json` token, and manages which one is active. The shell integration shim sets `CLAUDE_CONFIG_DIR` transparently on every `claude` invocation, so switching accounts swaps both the config and the token atomically.

> **macOS note.** Out of the box, `claude auth login` stores the token in the macOS Keychain under a service name derived from the config directory path (`Claude Code-credentials-<hash>`). That entry is opaque and breaks if the directory is moved. `claude-env login` runs the same flow, then **captures** the token into the environment's `.credentials.json` and removes the Keychain entry, so the portable file is the single source of truth.

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
| `claude-env init` | Create `~/.claude-envs/`, adopt the current Claude Code login (`~/.claude`'s token) as the `default` environment, copy `settings.json` and `CLAUDE.md` from `~/.claude/`, and set `default` as the global active environment. Run once after install. |
| `claude-env reset` | Restore the active environment's token to the default location (`~/.claude/.credentials.json`), then delete `~/.claude-envs/`. Removes all claude-env state. See [Uninstall](#uninstall). |

### Managing Environments

| Command | Alias | Description |
|---------|-------|-------------|
| `claude-env add <name>` | | Register a new environment. Creates `~/.claude-envs/<name>/` and copies `settings.json` and `CLAUDE.md` from `~/.claude/`. Does not authenticate — run `claude-env login <name>` afterward. |
| `claude-env remove <name>` | `rm` | Delete an environment and its directory. Refuses to remove the current global environment — switch first. |
| `claude-env list` | `ls` | List all environments. Active environment is marked with `*`. Shows `global`/`local` tags and shared resource count. |

### Switching

| Command | Alias | Description |
|---------|-------|-------------|
| `claude-env use <name>` | `global` | Set the global active environment. Persisted to `config.toml`. All new shells pick this up. Automatically syncs marketplace plugin paths (see `sync`). |
| `claude-env local <name>` | | Pin an environment to the current directory by writing a `.claude-env` file. Overrides the global setting for any shell in this directory tree. |
| `claude-env current` | | Print the active environment name and whether it is resolved from a local pin or the global setting. |

### Authentication

| Command | Description |
|---------|-------------|
| `claude-env login [name]` | Run `claude auth login` with `CLAUDE_CONFIG_DIR` pointed at the named environment (defaults to current). Opens the browser OAuth flow, then **captures the resulting token into `<env>/.credentials.json`** and removes the macOS Keychain entry so the token is portable. Also patches `.claude.json` with onboarding flags. |
| `claude-env import <name>` | Install a token without the browser flow — paste a `{"claudeAiOauth":{…}}` blob or a bare `sk-ant-*` token on stdin, copy from another environment with `--from-env <src>`, or capture a long-lived token with `--setup-token`. See [Importing & Exporting Tokens](#importing--exporting-tokens). |
| `claude-env export <name>` | Print an environment's token. Redacted by default; `--raw` emits the verbatim `.credentials.json` for backup or moving an account to another machine. |
| `claude-env auth-status [name]` | Report whether the named environment has a stored token, its subscription type, and token expiry — read **natively from `.credentials.json`, no network call**. |

**Important:** Before running `claude-env login` for a new environment, make sure you are signed into the correct account at [claude.ai](https://claude.ai/new). The OAuth flow in the browser attaches to whichever account is currently active. To avoid the browser entirely, use `claude-env import` instead.

### Importing & Exporting Tokens

Because each environment's identity is a plain `.credentials.json` token file, you can move tokens around without re-running the browser login.

```sh
# Paste a full token blob or a bare token (e.g. from `claude setup-token`)
pbpaste | claude-env import work
claude-env import work < token.json

# Capture a long-lived token via Claude Code's own flow (inference-only scope)
claude-env import work --setup-token

# Clone an account into another environment
claude-env import staging --from-env work

# Back up a token, or move an account to another machine
claude-env export work --raw > work-token.json
#   on the other machine:
claude-env add work && claude-env import work < work-token.json
```

A token blob has the shape Claude Code stores:

```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-…",
    "refreshToken": "sk-ant-ort01-…",
    "expiresAt": 1748276587173,
    "scopes": ["user:inference", "user:profile"],
    "subscriptionType": "max"
  }
}
```

### Security

Tokens are stored as `.credentials.json` files with `0600` (owner-only) permissions — the same model Claude Code uses natively on Linux and Windows, and comparable to `~/.aws/credentials`. They are **not** encrypted at rest. Treat an environment directory like any other secret material: do not commit it, sync it to a shared drive, or `export --raw` into a location others can read. `claude-env export` redacts tokens by default; `--raw` is required to emit the real values.

### Usage & Monitoring

| Command | Description |
|---------|-------------|
| `claude-env usage [name]` | Show token consumption, estimated costs, and rate limit reference for the active or named environment. Parses session data from `~/.claude-envs/<name>/projects/`. |
| `claude-env usage --all` | Show usage stats for every registered environment. |

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | `false` | Show usage for all environments instead of just the active one. |
| `--since <value>` | (all time) | Filter to a time window. Accepts durations (`24h`, `7d`, `30d`) or dates (`2026-04-01`). |

**Example output:**

```
Environment: default
Period: all time

Model                           Input       Output    Cache Write   Cache Read       Cost
────────────────────────────────────────────────────────────────────────────────────────────
claude-haiku-4-5-20251001       6,781       63,559      1,391,252   17,314,967     $3.7951
claude-opus-4-6                 6,088      327,072      4,823,052  119,211,100   $293.8706
────────────────────────────────────────────────────────────────────────────────────────────
Total                          12,869      390,631      6,214,304  136,526,067   $297.6657

Sessions: 24 │ Messages: 1722

Rate Limits (published, per minute):
  Opus     Requests: 1,000 │ Input: 2,000,000 tokens │ Output: 100,000 tokens
  Haiku    Requests: 1,000 │ Input: 2,000,000 tokens │ Output: 100,000 tokens
```

Costs are estimates based on published per-token pricing (Opus $15/$75, Sonnet $15/$75, Haiku $1/$5 per million input/output tokens). Unknown models use Sonnet pricing as a fallback, marked with `*`.

### Configuration

| Command | Description |
|---------|-------------|
| `claude-env config show` | Print the full `config.toml` contents. |
| `claude-env config path` | Print the config file path. |
| `claude-env config set-override <path> [--env name]` | Set `settings_override` for an environment (defaults to current). |
| `claude-env config clear-override [--env name]` | Clear `settings_override` for an environment (defaults to current). |

### Shared Resources

| Command | Alias | Description |
|---------|-------|-------------|
| `claude-env shared add <path> [--env name]` | | Add a pool resource to an environment's shared list and reconcile symlinks. Path is relative to the pool directory (e.g. `agents/reviewer.md`). |
| `claude-env shared remove <path> [--env name]` | `rm` | Remove a shared resource from an environment and reconcile symlinks. |
| `claude-env shared list [--env name]` | `ls` | List shared resources declared for an environment. |

All `--env` flags default to the current active environment.

### Sync

| Command | Description |
|---------|-------------|
| `claude-env sync [--env name]` | Rewrite `installLocation` entries in `known_marketplaces.json` to point to the target environment's plugin directory. Fixes path mismatches caused by installing or updating plugins from a different environment. Use `--dry-run` to preview changes. |

This runs automatically on `claude-env use`, but can be invoked manually to fix paths without switching environments.

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

Then use the CLI to declare which resources an environment shares:

```sh
claude-env shared add agents/my-agent.md --env work
claude-env shared add commands/deploy.md --env work
claude-env shared add commands/deploy.md --env personal
```

This updates `config.toml` and reconciles symlinks automatically. You can also edit `config.toml` directly:

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
    .credentials.json   # OAuth token (0600) — the account's identity
    .claude.json        # Onboarding + account metadata state
    settings.json       # Copied from ~/.claude/ on init/add
    CLAUDE.md           # Copied from ~/.claude/ on init/add
  work/
    .credentials.json
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

`reset` copies the active environment's token to `~/.claude/.credentials.json` so Claude Code continues to work after removal. If you want to preserve credentials for a non-active environment, run `claude-env use <name>` before `reset`, or `claude-env export <name> --raw > backup.json` first.
