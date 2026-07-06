# CLAUDE.md

## Project context

Go CLI that connects to a remote server over SSH and pulls backups down to
the local/client machine: database dumps (via `docker exec`), config/data
directory archives, and Docker named volumes. Runs from Linux or Windows.

## Architecture notes

- Config-driven: a YAML/TOML file defines target services, backup type per
  service (db dump / config dir / volume), and paths — the CLI walks this
  list rather than hardcoding service knowledge.
- SSH orchestration only. No reimplementing transfer/dedup/encryption —
  shell out to existing tools (e.g. `docker exec`, `tar`, `borg`) rather
  than hand-rolling equivalents.
- Cross-platform (Linux + Windows client) — avoid anything that assumes a
  POSIX shell on the client side.

## Out of scope

- Retention/pruning policy, dedup, encryption — delegate to backend tooling
  (e.g. Borg) rather than building these in-house.

## Workflow

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- Wait for explicit approval before writing any code or changing files
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## Development Guidelines

- For exploration tasks (finding files, grepping), prefer spawning Explore subagents rather than reading into main context
- DO NOT suggest service to service injections, unless absolutely necessary - present your reasoning if so
- Match existing code patterns and conventions even if you'd do it differently
- Minimum code that solves the problem. Nothing speculative.
- Don't "improve" adjacent code, comments, or formatting

## General guidelines
- Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify
- Don't assume. Don't hide confusion. Surface tradeoffs
- Define success criteria. Loop until verified
- Transform tasks into verifiable goals:
    - "Add validation" → "Write tests for invalid inputs, then make them pass"
    - "Fix the bug" → "Write a test that reproduces it, then make it pass"
    - "Refactor X" → "Ensure tests pass before and after"