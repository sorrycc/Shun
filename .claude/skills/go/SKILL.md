---
name: go
description: Wrap up a finished dev task — add CHANGELOG bullets under Unreleased, then commit and push.
---

# Go

**Always delegate to a subagent.** Spawn one `general-purpose` Agent with the prompt below, wait for it, then print its summary verbatim.

## Agent prompt (pass verbatim)

> Finish a dev task in this project. Report back in under 80 words.
>
> 1. Inspect: `git status`, `git diff`, `git diff --staged`, `git log @{u}..HEAD` (if upstream set). If working tree is clean and nothing is ahead, stop and report "nothing to do".
> 2. If the diff changes `version` in `mygo.json`, STOP — tell the user to run `Scripts/bump.sh` instead.
> 3. Abort if the diff touches `.env` or contains obvious secret patterns (R2 keys, notary password).
> 4. Update `CHANGELOG.md`:
>    - Add bullets for the whole batch (all uncommitted changes + any unpushed commits) under `## Unreleased`, following the Changelog rules in `AGENTS.md`. Skip it if nothing is user-facing.
>    - Derive wording from the actual diff, not commit subjects. Reject wording that reads like a commit message (`feat:`, file paths, symbol names).
> 5. Commit — keep it to **one commit**:
>    - If there are uncommitted changes: stage them + `CHANGELOG.md` (specific paths, never `git add -A`) and make one new commit.
>    - If everything is already committed locally (not pushed) and only `CHANGELOG.md` is new: `git add CHANGELOG.md` and `git commit --amend --no-edit` onto the last commit.
>    - Message per the Commits rules in `AGENTS.md`: one imperative subject of 72 characters or fewer. HEREDOC. Never `--no-verify`.
> 6. `git push`. If no upstream, set it with `-u`. Never force-push.
> 7. Report: commit SHA, branch, and the CHANGELOG bullets you wrote.

## Instructions

1. Spawn the subagent with the prompt above (Agent tool, `subagent_type: general-purpose`, foreground).
2. Print its summary verbatim. Don't re-run git to double-check.
