# Plan 120: Stage Outcomes and Hooks

## Summary

Give every stage one outcome model: a **state** (`accept` or `reject`), an
**action** (`promote`, `exit`, `park`, `retry`, `retry-previous`) and optional
**feedback**. Each stage type declares a default action per state, and a human
resolving a parked stage picks the state, action and feedback. Add **hooks**:
stages that wrap a whole order (`before`, `after`) or each work stage
(`before_each`, `after_each`), run unattended, and never change the order's
outcome. `after = ["reflect"]` is the default, so every order ends with one
reflect that sees every verdict and piece of feedback.

## Motivation

Today a parked stage has three controls, each with a fixed consequence:

| Control | Feedback | Consequence |
|---|---|---|
| `approve` | no | next stage |
| `reject` | no | order failed, worktree deleted |
| `request-changes` | yes | gated: re-run this stage; ungated: order failed |

What the user needs per stage type:

- `execute`: sometimes fail and park; sometimes fail and retry with feedback;
  sometimes continue to `adversarial-review` regardless ("I DID A THING!", and
  the review is where we say "no you didn't").
- `adversarial-review`: reject and send the work back to `execute` with
  feedback; reject with feedback and end the order now (for example "you
  hallucinated something; after reading the docs, this is not possible");
  accept and continue.
- In every case, reflect runs and learns from the verdicts and feedback.

The outcome of a stage (accept or reject) and what happens next (continue,
stop, wait, retry) are separate decisions. Coupling them is why "reject but
still go to reflect" cannot be expressed.

## Vocabulary

User-facing (config, control API, CLI, snapshot, skills):

| State | Meaning | Internal statuses |
|---|---|---|
| `pending` | not started | `pending` |
| `in-progress` | dispatched, running, merging or parked | `dispatching`, `running`, `merging`, `review` |
| `accept` | finished and accepted | `completed` |
| `reject` | finished and rejected | `failed` |
| `skipped` | never ran, the order ended first | `skipped`, `cancelled` |

The internal statuses stay: the loop relies on them (dispatch dedup, merge
recovery after a crash, parking, failure classification). The vocabulary is a
mapping on top, not a rewrite of the state machine.

| Action | Effect |
|---|---|
| `promote` | the next stage runs |
| `exit` | the remaining work stages are skipped; `after` hooks still run. Accept: the order completes (or awaits publish when gated). Reject: the order fails with the feedback as its reason and the worktree is kept |
| `park` | the stage waits for a human to resolve it |
| `retry` | this stage runs again with the feedback in its prompt |
| `retry-previous` | the stage before it runs again with the feedback, then a copy of this stage ("send back to sender") |

Both retries count against `[review] max_rework` (default 2); at the limit the
order fails, worktree kept. An order with any rejected work stage ends
`failed`, reason `rejected: <feedback>`, even when it promoted past it; nothing
from it is published.

## Config

```toml
[stages.execute]
accept = "promote"
reject = "promote"

[stages.adversarial-review]
accept = "park"
reject = "park"

[hooks]
before = []                 # once, before the order's first work stage
after = ["reflect"]         # once, after the work stages end, however they end
before_each = []            # before every work stage
after_each = []             # after every work-stage attempt
```

- An unset state keeps today's mode-driven behavior (supervised parks a stage with changes, auto promotes, a failed cook fails the order). Unknown
  action names fail config load.
- `[hooks] after` unset means `["reflect"]` when a `reflect` skill is
  registered; `after = []` turns it off. The other hook keys default to empty.
- An order-level hook is not added when the order already has a stage with
  that name (for example a reflect the scheduler proposed).
- `[review] extra_stages` keeps its job (stages such as `adversarial-review`
  added after quality). `reflect` belongs in `[hooks] after` instead.

## Behavior

- **Automatic state:** a cook that finishes is `accept`; a failed cook or a
  blocking `stage_message` is `reject`. The stage type's default action for
  that state then applies. `park` parks as today.
- **Resolving a parked stage:** the human gives state, action and feedback.
  `approve`, `reject` and `request-changes` stay as aliases for
  `accept promote`, `reject exit` and `reject retry`.
- **Hooks:** run as stages the loop inserts (marked in `Stage.Extra`), never
  park in any mode, and a failed hook is skipped and logged (this generalizes
  the failed-reflect skip from the `extra_stages` change). `after_each` is
  inserted after a work-stage attempt once its outcome is known, so it sees
  the verdict; `after` is inserted when the work stages end. Hooks never
  change the order's outcome.
- **Feedback reaches later stages:** each attempt's state, action and
  feedback are recorded on the stage, and `buildCookPrompt` lists them for
  every later stage, hooks included. `adversarial-review` sees execute's
  outcome; reflect sees all of them.

## Surfaces

- Control API: `{"action":"resolve","order_id":"14","task_key":"adversarial-review","state":"reject","do":"retry-previous","prompt":"..."}` (`task_key` and `prompt` as `stage-complete` uses them).
- CLI: `noodle order <id> resolve <stage> <accept|reject> <action> --feedback-file <f>`,
  alongside `noodle order <id> complete`.
- Snapshot: each stage carries its user-facing state and its outcomes
  (state, action, feedback per attempt).
- `/noodle-review` (dotfiles `.claude/commands/noodle-review.md`, outside the
  plugin): offers the stage's actions and points to the CLI.

## Phases

Each phase starts by pinning current behavior in tests, or with a failing
test for the new behavior.

- [x] 1. Config: `[stages.<task>]` and `[hooks]` keys, defaults, validation (`config/fork.go`).
- [x] 2. Outcomes: `loop/outcome.go` with one `applyOutcome(stage, state, action, feedback)` for every path; `promote`, `exit` (via `stage_remaining_skipped`, minus hooks), `park`, `retry`, `retry-previous`; rework limit.
- [x] 3. Automatic outcomes: cook completion and failure map to a state and the configured default action.
- [x] 4. Hooks: insertion for `before`, `after`, `before_each`, `after_each`; never park; failed hook skipped; order outcome from work stages only.
- [x] 5. Feedback: record outcomes per attempt; prompt context in `buildCookPrompt`; snapshot fields.
- [x] 6. Surfaces: `resolve` control action and aliases; `noodle order ... resolve` CLI.
- [x] 7. Docs: plugin docs, the `noodle-review` command, and moving dory's `reflect` from `extra_stages` to `[hooks] after` (ask first; outside the plugin).

## As built

- Unset `[stages.<task>]` states keep the mode-driven behavior instead of parking (see Config).
- The aliases keep their own code paths and record `accept promote`, `reject exit` and `reject retry` (ungated `request-changes`: `reject exit`); `stage-complete` records `accept promote` or `reject retry-previous` with the review as feedback.
- Hooks are inserted at promotion into unstarted orders, not when the work stages end; `exit` skips work stages and leaves hook stages pending. An amend from the scheduler keeps a pending `after` hook.
- `after_each` runs after a stage's last attempt, not between retries: inserting it between attempts would move the retried stage to a new index and lose its worktree. The stages `retry-previous` inserts get no `before_each`/`after_each` of their own.
- `retry-previous` targets the nearest earlier stage that is not a hook or a loop-inserted quality stage.
- Outcomes live in stage `Extra["outcomes"]`; the reducer ends an order `failed` when a stage's latest outcome is a reject with `promote` or `exit`.
- `promote` never merges: it keeps the branch like `approve`.

## Out of scope

- Replacing the internal stage statuses.
- UI buttons for the new actions (the existing Reviews tab buttons keep
  working through the aliases).

## Rollout

The loop running dory's ten review orders uses the current binary. Build and
install only after those orders finish and the user stops the loop.
