<!-- prove:managed:start -->
# claude-multi-sub

<!-- prove:plugin-version:0.30.0 -->
**Prove plugin v0.30.0** — if the installed plugin version (`cat /Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/.claude-plugin/plugin.json | grep version`) does not match v0.30.0, run `/prove:update` to sync.

Go (go)

## Structure

- `cmd/` — Go CLI entry points
- `internal/` — Internal packages
- `pkg/` — Go packages

## Conventions

- File naming: snake_case
- Test files: *_test.ext (suffix)

## Tool Directives

### acb

Before every `git commit` on a feature branch, write an intent manifest via `python3 -m tools.acb save-manifest` describing what changed and why. The PreToolUse hook blocks commits without a manifest. After committing, manifests are assembled into a reviewable ACB document via `/prove:review`.

## References

### Creator Conventions

@/Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/references/creator-conventions.md

### Interaction Patterns

@/Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/references/interaction-patterns.md

### LLM Coding Standards

@/Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/references/llm-coding-standards.md

### Prompt Engineering Guide

@/Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/references/prompt-engineering-guide.md

### Validation Config

@/Users/manuelmorales/.claude-envs/default/plugins/cache/prove/prove/0.30.0/references/validation-config.md

## Prove Commands

- `/prove:autopilot` — Autonomous execution with validation gates
- `/prove:brainstorm` — Explore options and record decisions
- `/prove:comprehend` — Socratic quiz on recent diffs to build code comprehension
- `/prove:index` — Update the file index (run after significant changes)
- `/prove:plan-task` — Plan implementation for a task
- `/prove:tools` — Manage prove tools — list, install, remove, status

<!-- prove:managed:end -->
