---
name: draft-pr
description: Publish an order's finished work as a draft pull request, autonomously. Pushes the order's branch to the delivery remote and opens the draft PR, or pushes to the existing PR for that branch. Use as the `[review] publish_action` stage; it never asks questions.
---

# Draft PR

Publish the work of a finished order as a draft pull request. This runs as the final stage of an order,
as an unattended agent session. Nobody answers questions: decide from the inputs below, or stop and
yield the reason.

## Inputs

Read these; do not assume `origin`, a base branch or a repo.

1. **The stage prompt:** order id, order title, rationale and the order's branch. Use the branch the
   prompt names. If it names none, use `git branch --show-current`.
2. **`[delivery]` in `.noodle.toml`** (at the project root, or linked into this worktree):
   - `push_remote`: the git remote to push to.
   - `pr_repo`: the `owner/name` repo the PR is opened on.
   - `base`: the branch the PR targets.
   - `branch`: optional branch name template. Only matters if the prompt names no branch.

If `push_remote`, `pr_repo` or `base` is missing, or the branch is the same as `base`, stop and yield
`Not published: <what is missing>`.

## External writes and permissions

`git push` and `gh pr create` (and `gh pr edit`) write outside the repo. An agent's permission mode can
deny them. If any of these is denied or fails for permission reasons: stop, do not retry, do not try
another route (no other remote, no `--force`, no API call through `gh api`, no editing credentials or
hooks), and yield `Not published: <command> was denied: <reason>`. A human decides what happens next.
Read-only commands (`git fetch`, `git log`, `gh pr list`, `gh pr view`, `gh repo view`) are fine.

## Steps

1. **State.** `git branch --show-current`, `git status --short`. Do not commit anything and do not add
   files: publish only what is already committed. If there are uncommitted changes, leave them out and
   mention them in one line at the end of the yield message.
2. **Commits to publish.** `git fetch <push_remote> <base>`, then
   `git log --oneline <push_remote>/<base>..<branch>`. If the fetch fails, use the local `<base>` ref.
   If there are no commits, yield `NO CHANGE: nothing to publish` and stop.
3. **Existing PR.** `gh pr list --repo <pr_repo> --state open --json url,headRefName` and look for an
   entry whose `headRefName` is the branch. If one exists, reuse it: skip to step 7 and update it.
4. **Checks.** Run the checks the repo documents for the touched modules (look in `AGENTS.md`,
   `CLAUDE.md`, `CONTRIBUTING.md`, `README`, `Makefile`, then CI config). If none are documented, say so
   in the Validation section. Report only what you ran and what it printed. A failing check does not
   stop publication of a draft; say which one failed in Validation.
5. **Title and body** (below). Write the body to a temp file and pass it with `--body-file`.
6. **Push and create.**

   ```
   git push -u <push_remote> <branch>
   gh pr create --repo <pr_repo> --draft --base <base> --head <head> --title "<title>" --body-file <file>
   ```

   `<head>` is `<branch>` when `push_remote` is a remote of `pr_repo` itself. When `push_remote` is a
   fork, it is `<fork-owner>:<branch>`; take the owner from `git remote get-url <push_remote>`.
7. **Existing PR path.** `git push <push_remote> <branch>` to add the new commits, then refresh the PR
   so it describes the whole diff: `gh pr edit <url> --repo <pr_repo> --title "<title>" --body-file <file>`.
   Never force-push. If the push is rejected as non-fast-forward, stop and yield
   `Not published: push to <branch> was rejected as non-fast-forward`.
8. **Yield.** Emit this and finish:

   ```
   noodle event emit --session $NOODLE_SESSION_ID stage_yield --payload '{"message": "Draft PR: <url>"}'
   ```

   Use the other messages above for the no-op and stop cases. Keep messages to one line and free of
   double quotes so the JSON stays valid.

## Title

Match the repo. Run `gh pr list --repo <pr_repo> --state merged --limit 5 --json title` and follow its
convention. If it has none, use conventional-commit style: `feat:`, `fix:`, `refactor:`, `docs:` or
`chore:`. Lowercase, no period, under 70 characters, saying what the PR does. Build it from the order
title and the commits, not from an identifier.

## Body

Find the template: `.github/PULL_REQUEST_TEMPLATE.md`, `.github/pull_request_template.md`, `docs/`, the
repo root, or a `PULL_REQUEST_TEMPLATE/` directory. Use its headings in its order. If several templates
exist, use the one named `default` or the first alphabetically. Delete a section only if it is empty.
Read the last two or three merged PRs (`gh pr list --repo <pr_repo> --state merged --limit 3`, then
`gh pr view <n> --repo <pr_repo>`) and match their tone and length.

If there is no template, use:

```
## Summary
One to three sentences: what the PR changes and what it does, in plain words.

## Changes
- One bullet per change, naming the function, flag, env var or file in backticks.
- Say what the code does and why where it is not obvious.

## Validation
The exact commands that were run, and in which modules.
```

Rules:

- Describe only what is in the pushed diff. Never mention uncommitted or untracked files, local scripts or
  other local state; a reviewer cannot see them.
- Use the order's rationale for the reason a change exists. Do not paste the stage prompt.
- Template sections for a ticket or related PRs: write "None." Do not put ticket ids, issue keys or PR
  numbers anywhere in the title or body. Refer to related work by what it does.
- Check a checklist box only if it is true. Leave it unchecked, with a one-line reason, if not.
- Validation lists commands that were actually run. If something was not run, say so in one line.
- Note breaking changes under Changes, with who is affected.
- No "Not in this PR" section.
- Answer first, no recap, fewest words. No emojis, no emdashes, no stock phrases.
- End with one plain attribution line (the tool that opened it, and the model if you know it), with no
  emoji.
