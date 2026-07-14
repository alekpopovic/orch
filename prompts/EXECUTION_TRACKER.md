# Prompt Execution Tracker

This file tracks the latest prompt that has been executed against this repository.

## Current State

- Last executed prompt: `36-traefik-integration.md`
- Last matching commit: `ea0760e Add optional Traefik integration`
- Completed prompt range: `00` through `36`
- Next prompt to execute: `37-secrets-mvp.md`
- Updated: `2026-07-14`

## Update Rule

After a prompt is executed and its repository changes are committed, update this file in the same commit or in the next tracking-only commit:

- Set `Last executed prompt` to the prompt file that was just completed.
- Set `Last matching commit` to the commit hash and subject for that work.
- Extend `Completed prompt range` when prompts are executed sequentially.
- Set `Next prompt to execute` to the next pending prompt.
- Refresh `Updated` with the UTC date of the tracker change.

## Source Of Truth

When this file disagrees with Git history, inspect `git log --oneline` and prefer commits whose subjects clearly match prompt titles.
