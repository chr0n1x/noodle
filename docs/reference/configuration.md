# Configuration

Noodle is configured through a `.noodle.toml` file at the root of your project. When no config file exists, Noodle uses sensible defaults for all values.

## Minimal config

Most projects only need routing defaults and a skills path:

```toml
[routing.defaults]
provider = "claude"
model = "claude-opus-4-6"

[skills]
paths = [".agents/skills"]
```

## Full reference

### `mode`

Controls human oversight level.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mode` | string | `"supervised"` | Autonomy mode: `"auto"`, `"supervised"`, or `"manual"` |

```toml
mode = "supervised"
```

---

### `[routing]`

Controls which Agent provider and model are used for skill execution.

#### `[routing.defaults]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | `"claude"` | Agent provider name: `claude`, `codex`, `pi`, `pig`, `maki` or `copilot` |
| `model` | string | `"claude-opus-4-6"` | Model identifier passed to the provider |

```toml
[routing.defaults]
provider = "claude"
model = "claude-opus-4-6"
```

### `[skills]`

Configures where Noodle discovers skill definitions.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `paths` | string[] | `[".agents/skills"]` | Directories to scan for SKILL.md files |

```toml
[skills]
paths = [".agents/skills"]
```

---

### `[concurrency]`

Controls parallel execution limits.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_concurrency` | int | `4` | Maximum number of concurrent agent sessions |

```toml
[concurrency]
max_concurrency = 4
```

::: warning Cost
Each agent consumes tokens independently. `max_concurrency = 4` means up to four concurrent API sessions. Start with `max_concurrency = 1` or `2` while you're learning the system, then scale up once you've seen the cost per session on your workload.
:::

---

### `[agents]`

Configures the agent CLIs. Each sub-table names a provider: `claude`, `codex`, `pi`, `pig`, `maki` or `copilot`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `inherit_env` | string[] | unset | Environment variables every agent inherits from noodle. Unset inherits the whole environment |

#### `[agents.<provider>]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | `""` | Directory holding the CLI binary. Unset runs the binary found on `PATH` |
| `args` | string[] | `[]` | Additional CLI arguments passed on every invocation |
| `exit_after_turn` | bool | `false` | Close the agent's stdin when its turn ends, so it exits and the stage completes. Only `claude` and `maki` keep stdin open |
| `inherit_env` | string[] | unset | Replaces `[agents] inherit_env` for this provider |

With `inherit_env` set, an agent gets only the listed variables plus a base set: `PATH`, `HOME`, `USER`, `LOGNAME`, `SHELL`, `TERM`, `TMPDIR`, `LANG`, `LC_*`, and on Windows `SYSTEMROOT`, `COMSPEC`, `PATHEXT`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `TEMP`, `TMP`. Names match case-insensitively, and a trailing `*` matches a prefix. `NOODLE_*` variables and the order's env vars are always set. An empty list (`[]`) keeps only the base set.

```toml
[agents]
inherit_env = ["GITHUB_TOKEN", "AWS_*"]

[agents.claude]
path = "~/.claude/local"
args = ["--verbose"]

# copilot fails to authenticate when GITHUB_TOKEN is set to a token
# without Copilot access, so drop it for copilot only.
[agents.copilot]
inherit_env = ["AWS_*"]
```

How each provider runs:

| Provider | Command | Prompt | Skill | Steering |
|----------|---------|--------|-------|----------|
| `claude` | `claude --input-format stream-json --output-format stream-json` | stream-json on stdin | system prompt | yes |
| `maki` | `maki -p --input-format stream-json --output-format stream-json --yolo` | stream-json on stdin | system prompt | yes |
| `codex` | `codex exec --json` | stdin, then closed | loaded by codex | no |
| `pi`, `pig` | `pi -p --mode json` | stdin, then closed | system prompt | no |
| `copilot` | `copilot --output-format json --yolo` | stdin, then closed | appended to the prompt | no |

pi, pig and maki report cost per session. copilot reports premium requests rather than cost, so its sessions show no cost.

---

### `[runtime]`

Controls which runtime executes agent sessions and per-runtime settings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default` | string | `"process"` | Default runtime kind: `"process"` or `"sprites"` |

```toml
[runtime]
default = "process"
```

#### `[runtime.process]`

Local process runtime. Runs agent CLIs as child processes on the host machine.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_concurrent` | int | `4` | Maximum concurrent process sessions |

```toml
[runtime.process]
max_concurrent = 4
```

#### `[runtime.sprites]`

Cloud runtime using [Sprites](https://sprites.dev) sandboxed environments.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token_env` | string | `"SPRITES_TOKEN"` | Environment variable name for the Sprites API token |
| `base_url` | string | `""` | Custom API base URL (leave empty for default) |
| `sprite_name` | string | `""` | Name prefix for spawned sprite instances |
| `git_token_env` | string | `"GITHUB_TOKEN"` | Environment variable name for the Git token used by sprites |
| `max_concurrent` | int | `50` | Maximum concurrent sprite sessions |

```toml
[runtime.sprites]
token_env = "SPRITES_TOKEN"
base_url = ""
sprite_name = "noodle-dev"
git_token_env = "GITHUB_TOKEN"
max_concurrent = 50
```

---

### `[server]`

Controls the web UI server.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `3000` | Port for the web UI server |
| `enabled` | bool | `true` | Whether to start the server. When omitted, the server starts automatically |

```toml
[server]
port = 3000
enabled = true
```

---

### `[stages.<task>]`

Sets what the loop does when a stage of that task finishes, by verdict. A cook that finishes is `accept`; a cook that fails, or leaves a blocking stage message, is `reject`. The message or the failure becomes the stage's feedback.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `accept` | string | unset | Action for an accepted stage |
| `reject` | string | unset | Action for a rejected stage |

| Action | Effect |
|--------|--------|
| `promote` | The next stage runs. The stage's branch is kept; nothing is merged |
| `exit` | The remaining work stages are skipped and `after` hooks still run. An accept completes the order (or holds it for publish); a reject fails it, keeping the worktree |
| `park` | The stage waits for a human (`noodle order <id> resolve`) |
| `retry` | This stage runs again in its worktree with the feedback |
| `retry-previous` | The work stage before it runs again with the feedback, then a copy of this stage |

An unset state keeps the mode's behavior: supervised mode parks a stage with changes, auto mode promotes, and a failed cook fails the order. An unknown action name stops the loop at startup. Both retries count against `[review] max_rework` (default 2); at the limit the order fails and the worktree is kept. An order with a stage whose latest verdict is a reject it was let past (`promote` or `exit`) ends failed, and nothing from it is published.

When a task sets `reject`, its stages must give a verdict. A cook that finishes without any stage message runs again, with feedback saying it ended without a verdict. That counts against `max_rework`, and at the limit the stage parks for a human instead of being accepted. A task with only `accept` set still reads a finish with no message as accept.

Each stage's prompt lists every verdict recorded so far in its order, so a review sees why the work before it was accepted or rejected.

```toml
[stages.execute]
accept = "promote"
reject = "promote"

[stages.adversarial-review]
reject = "retry-previous"
```

The same table sets how the task's cooks run:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `harness` | string | scheduler's choice | Agent CLI for the cook: `claude`, `codex`, `pi`, `pig`, `maki` or `copilot` |
| `model` | string | scheduler's choice | Model for the cook |
| `provider` | string | unset | Codex's `model_provider`, passed as `-c model_provider="<provider>"`. Needs `harness = "codex"`; the claude CLI takes its backend from the environment |
| `system_prompt` | string | unset | Text placed after the skill in the cook's system prompt, so it can override what the skill says. With codex and copilot it is appended to the prompt |

`harness` and `model` are set on every stage of the task that has not started, when orders are promoted and again when the cook starts. A stage already running keeps its model. An unknown harness, or `provider` without `harness = "codex"`, stops the loop at startup.

Use `system_prompt` to wrap a skill: run it as written, but change one instruction. Here the review runs on a different model from the work it checks, and its reviewers use pi instead of `codex exec`:

```toml
[stages.adversarial-review]
harness = "claude"
model = "claude-opus-5-5"
system_prompt = """
The adversarial-review skill says to run reviewers with `codex exec`. Ignore that.
Run each reviewer with: pi -p --provider openai-codex --model gpt-5.5 "<prompt>"
"""
```

---

### `[hooks]`

Skills the loop runs around an order's work stages. Hooks never park, a failed hook is skipped, and a hook's result never changes the order's outcome. Hooks are added when an order is promoted, before any of its stages starts.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `before` | array of strings | `[]` | Run once, before the order's first stage |
| `after` | array of strings | `["reflect"]` | Run once, after the work stages end, however they end. `[]` turns it off |
| `before_each` | array of strings | `[]` | Run before every work stage |
| `after_each` | array of strings | `[]` | Run after every work stage |

Each list runs in the order written; a name repeated in one list runs once, at its first place. An order-level hook is left out when the order already has a stage with that name. A hook with no registered skill is skipped with a warning (the default `reflect` is skipped quietly). `after_each` runs after a stage's last attempt, not between retries.

```toml
[hooks]
after = ["reflect"]
before_each = []
```

---

### `[adapters.<name>]`

Adapters bridge external systems (issue trackers, project boards) into Noodle's backlog. Each adapter is a named table mapping a skill to a set of shell scripts.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `skill` | string | — | Skill name this adapter extends |
| `scripts` | map | — | Named shell scripts the adapter invokes |

The default config includes a `backlog` adapter:

```toml
[adapters.backlog]
skill = "backlog"

[adapters.backlog.scripts]
sync = "my-adapters/backlog-sync"
done = "my-adapters/backlog-done"
```

Scripts are executed relative to the project root. Each script receives structured input on stdin and must produce structured output on stdout. See the [backlog skill docs](/concepts/skills) for the expected interface.

## Common configurations

### Local development with Claude

Minimal setup for a single developer using Claude as the Agent provider:

```toml
mode = "supervised"

[routing.defaults]
provider = "claude"
model = "claude-opus-4-6"

[skills]
paths = [".agents/skills"]

[agents.claude]
path = "~/.claude"

[concurrency]
max_concurrency = 2
```

### Cloud execution with Sprites

Scale out with cloud sandboxes while keeping a local fallback:

```toml
[routing.defaults]
provider = "claude"
model = "claude-opus-4-6"

[runtime]
default = "sprites"

[runtime.sprites]
sprite_name = "my-project"
max_concurrent = 20

[runtime.process]
max_concurrent = 2
```

### Defaults-only routing

Set a single project-wide default provider and model:

```toml
[routing.defaults]
provider = "claude"
model = "claude-opus-4-6"
```
