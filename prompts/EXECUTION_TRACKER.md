# Prompt Execution Tracker

This file tracks the latest prompt that has been executed against this repository.

## Current State

- Last executed prompt: `90-v0-3-0-release-hardening.md`
- Last matching commit: `7985b9e Harden v0.3 operations and release`
- Completed prompt range: `00` through `90`
- Next prompt to execute: none; all repository prompts are complete
- Updated: `2026-08-21`

## Update Rule

After a prompt is executed and its repository changes are committed, update this file in the same commit or in the next tracking-only commit:

- Set `Last executed prompt` to the prompt file that was just completed.
- Set `Last matching commit` to the commit hash and subject for that work.
- Extend `Completed prompt range` when prompts are executed sequentially.
- Set `Next prompt to execute` to the next pending prompt.
- Refresh `Updated` with the UTC date of the tracker change.

## Source Of Truth

When this file disagrees with Git history, inspect `git log --oneline` and prefer commits whose subjects clearly match prompt titles.
