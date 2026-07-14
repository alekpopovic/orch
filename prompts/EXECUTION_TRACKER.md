# Prompt Execution Tracker

This file tracks the latest prompt that has been executed against this repository.

## Current State

- Last executed prompt: `32-reconciler-idempotency-under-restart.md`
- Last matching commit: `a11cec0 Harden reconciler restart idempotency`
- Completed prompt range: `00` through `32`
- Next prompt to execute: `33-fake-runtime-for-deterministic-tests.md`
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
