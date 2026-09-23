# CLAUDE.md

## Project context

Go CLI that runs configured commands on a remote server over SSH and pulls
their stdout down to the local machine as timestamped files. 

Generic command runner at its core; the motivating use cases are backups: database dumps via
`docker exec pg_dump`, `tar` archives of config dirs and Docker volumes.

## Architecture notes

- Config-driven: YAML defines jobs — each job is a name, a remote command
  (run verbatim, no wrapping), and an output filename. The CLI walks this
  list; it has no knowledge of what the commands do.
- SSH orchestration only. No reimplementing transfer/dedup/encryption —
  the remote command shells out to existing tools (`docker exec`, `tar`,
  `borg`) rather than hand-rolling equivalents.
- Output lands at `<destination>/<timestamp>/<job>/<output>`, written
  atomically (`.partial` + rename) by `internal/store`.
- Logging via zap (`pkg/logger`): stdout + `logs/app.log`, level from
  `log.level` in config.
- Cross-platform — avoid anything that assumes a POSIX shell on the client side.

## Rules

- DO NOT suggest service to service injections, unless absolutely necessary - present your reasoning if so
- Match existing repository code patterns and conventions. If you'd do it differently, suggest
- Minimize helpers in any domain/service files. If they are needed, create them in utils package.
- DO NOT create separate test files, use shared per domain/service ones.
