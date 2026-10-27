---
name: reflect
description: >-
  Reflect on the conversation and update the brain. Use when wrapping up, after mistakes
  or corrections, or when significant codebase knowledge was gained. Triggers: "reflect",
  "remember this".
schedule: "Follow-up stage after quality — capture learnings from the completed execute→quality cycle."
---

# Reflect

Review the conversation and persist learnings — to `brain/`, to skill files, or as structural enforcement.

When you run as a scheduled reflect stage, you start in a new session and have no conversation to read. Do not claim you have conversation access. Inspect the order's work instead:

- The order's commits: `git log` and `git diff` against the base branch.
- `brain/reviews/*.md` files for the order.
- The `stage_yield` messages of the order's earlier sessions, if readable (`.noodle/sessions/<order_id>-*/events.ndjson` in the main checkout).

## Process

**Use Tasks to track progress** (TaskCreate per step, TaskUpdate to in_progress/completed).

1. **Read `brain/index.md`** to understand what notes already exist
2. **Scan the conversation** (or, in a scheduled stage, the order's work listed above) for:
   - Mistakes made and corrections received
   - User preferences and workflow patterns
   - Codebase knowledge gained (architecture, gotchas, patterns)
   - Tool/library quirks discovered
   - Decisions made and their rationale
   - Friction in skill execution, orchestration, or delegation
   - Repeated manual steps that could be automated or encoded
3. **Skip** anything trivial or already captured in existing brain files
4. **Route each learning** to the right destination (see Routing below)
5. **Update `brain/index.md`** if any brain files were added or removed
6. **Yield** after writing learnings, then end your turn:

   ```bash
   noodle event emit --session $NOODLE_SESSION_ID stage_yield --payload '{"message": "Reflect: <one-line summary>"}'
   ```

   Without this, a finished reflect stage can be treated as ending its turn without changes and parked.

## Routing

Not everything belongs in the brain. Route each learning to where it will have the most impact.

### Structural enforcement check

Before routing a learning to `brain/`, ask: can this be a lint rule, script, metadata flag, or runtime check? If yes, encode it structurally and skip the brain note. See `brain/principles/encode-lessons-in-structure.md`.

### Brain files (`brain/`)

Codebase knowledge, delegation principles, gotchas — anything that informs future sessions. This is the default destination. Use the `brain` skill for writing conventions.

- One topic per file. File name = topic slug.
- Group in directories with index files using `[[wikilinks]]`.
- No inlined content in index files.

### Skill improvements (`.agents/skills/<skill>/`)

If a learning is about how a specific skill works — its process, prompts, or edge cases — update the skill directly. Use the `skill-creator` skill for guidelines on effective skill content.

### Orchestration workflow improvements

If the session revealed systemic orchestration issues, route to:
- **Brain principle** (new delegation heuristic) → `brain/delegation/`
- **Skill mechanics change** (monitoring loop, review step) → relevant skill file
- **New skill opportunity** (recurring workflow that could be encoded) → note in summary for the user

### Backlog items (`brain/todos.md`)

Follow-up work that can't be done during reflection — bugs, non-trivial rewrites, tooling gaps. Use the `todo` skill to file.

## Summary

```
## Reflect Summary
- Brain: [files created/updated, one-line each]
- Skills: [skill files modified, one-line each]
- Structural: [rules/scripts/checks added]
- Todos: [follow-up items filed]
```
