# CLI Reference

All commands accept the global `--project-dir` flag. When omitted, Noodle uses the current directory (or the `NOODLE_PROJECT_DIR` environment variable).

## Global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project-dir` | string | current directory | Project directory. Also settable via `NOODLE_PROJECT_DIR` |

---

## `noodle start`

Start the noodle loop. Spawns agent sessions and manages the full lifecycle.

Auto-starts a web server on port 3000 (configurable via `[server]`). Opens a browser unless `NOODLE_NO_BROWSER=1` is set.

```
noodle start [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--once` | bool | `false` | Run one scheduling cycle and exit |

---

## `noodle status`

Show compact runtime status. Prints active agent count, orders queue depth, and noodle loop state (running, paused, draining, or idle).

```
noodle status
```

---

## `noodle skills`

List resolved skills.

```
noodle skills
```

### `noodle skills list`

List all resolved skills.

```
noodle skills list
```

---

## `noodle schema`

Print generated schema docs for Noodle runtime contracts. Takes an optional target argument.

```
noodle schema [target]
```

### `noodle schema list`

List available schema targets.

```
noodle schema list
```

---

## `noodle worktree`

Manage linked git worktrees. Noodle uses worktrees to isolate concurrent agent sessions so they don't conflict on the working tree.

### `noodle worktree create`

Create a new linked worktree.

```
noodle worktree create <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string | `HEAD` | Branch or commit to base the new worktree on |

### `noodle worktree exec`

Run a command inside a worktree. Sets the working directory to the worktree path before executing.

```
noodle worktree exec <name> <command...>
```

### `noodle worktree merge`

Merge a worktree branch into a target branch.

```
noodle worktree merge <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--into` | string | integration branch | Target branch to merge into |

### `noodle worktree cleanup`

Remove a worktree without merging.

```
noodle worktree cleanup <name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | `false` | Remove even when unmerged commits exist |

### `noodle worktree list`

List all worktrees with merge status.

```
noodle worktree list
```

### `noodle worktree prune`

Remove merged and patch-equivalent worktrees.

```
noodle worktree prune
```

### `noodle worktree hook`

Run worktree session hook. Used internally by agent sessions.

```
noodle worktree hook
```

---

## `noodle event`

Manage noodle loop events.

### `noodle event emit`

Emit an external event into the noodle loop or a specific session.

```
noodle event emit <type> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--payload` | string | — | Event payload as JSON |
| `--session` | string | — | Session ID. When set, writes to the session event log instead of the noodle loop event log |

---

## `noodle order`

Complete or resolve the stage an order is held or parked at. The stage name must match that stage; the loop refuses any other name.

### `noodle order complete`

Submit an external review for a stage held in `[review] external_stages`.

```
noodle order <order_id> complete <stage> --file <review.md> [--reject] [--no-wait]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string | — | Markdown file with the review (required) |
| `--reject` | bool | `false` | Send the review back as rework |
| `--no-wait` | bool | `false` | Do not wait for the loop to acknowledge |

### `noodle order resolve`

Give a parked or held stage a verdict and an action (see [`[stages.<task>]`](/reference/configuration) for the actions).

```
noodle order <order_id> resolve <stage> <accept|reject> <promote|exit|park|retry|retry-previous> [--feedback-file <f>] [--no-wait]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--feedback-file` | string | — | File with feedback, recorded on the stage and shown to later stages |
| `--no-wait` | bool | `false` | Do not wait for the loop to acknowledge |

The control API takes the same command: `{"action": "resolve", "order_id": "14", "task_key": "adversarial-review", "state": "reject", "do": "retry-previous", "prompt": "<feedback>"}`. `approve`, `reject` and `request-changes` record `accept promote`, `reject exit` and `reject retry` (`reject exit` when the order is ungated).

---

## `noodle reset`

Clear all runtime state. Removes and recreates the runtime directory.

Refuses to run if Noodle is currently running (checks the lock file).

```
noodle reset
```
