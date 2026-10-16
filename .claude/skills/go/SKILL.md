---
name: go
description: Wrap up a finished dev task — add one CHANGELOG bullet under Unreleased, then commit and push.
---

# Go

**Always delegate to a subagent.** Spawn one `general-purpose` Agent with the prompt below, wait for it, then print its summary verbatim.

## Agent prompt (pass verbatim)

> Finish a dev task in this project. Report back in under 80 words.
>
> 1. Inspect: `git status`, `git diff`, `git diff --staged`, `git log @{u}..HEAD` (if upstream set). If working tree is clean and nothing is ahead, stop and report "nothing to do".
> 2. If the diff touches `VERSION`, STOP — tell the user to run `Scripts/bump.sh` instead.
> 3. Abort if the diff touches `.env` or contains obvious secret patterns (R2 keys, notary password).
> 4. Update `CHANGELOG.md`:
>    - Write **one** bullet covering the whole batch (all uncommitted changes + any unpushed commits) under `## Unreleased`. Create the section above the latest released version if missing.
>    - Style: user-facing, terse, present tense, full sentences; bold the affected UI area (e.g. **Match editor**); prefix bugfixes with `Fix:`.
>    - Derive wording from the actual diff, not commit subjects. Reject wording that reads like a commit message (`feat:`, file paths, symbol names).
> 5. Commit — keep it to **one commit**:
>    - If there are uncommitted changes: stage them + `CHANGELOG.md` (specific paths, never `git add -A`) and make one new commit.
>    - If everything is already committed locally (not pushed) and only `CHANGELOG.md` is new: `git add CHANGELOG.md` and `git commit --amend --no-edit` onto the last commit.
>    - Message style `type: short summary` (`feat:`, `fix:`, `docs:`, `chore:`). HEREDOC. Never `--no-verify`.
> 6. `git push`. If no upstream, set it with `-u`. Never force-push.
> 7. Report: commit SHA, branch, and the CHANGELOG bullet you wrote.

## Instructions

1. Spawn the subagent with the prompt above (Agent tool, `subagent_type: general-purpose`, foreground).
2. Print its summary verbatim. Don't re-run git to double-check.
