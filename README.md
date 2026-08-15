# backpull

A Go CLI that runs configured commands on a remote server over SSH and pulls
their output down to the local machine.

Each **job** in the config is a command whose stdout is captured into a local
file. What the command does is entirely up to you - backpull just runs it and
streams the bytes back. Jobs run on the remote host by default; mark one
`local: true` to run it on this machine instead.

Typical use cases (and why this exists):

- **Database dumps** - `pg_dump`/`mysqldump` executed inside a running Docker container via `docker exec`
- **Config/data directories** - `tar -czf -` of a directory on the server
- **Docker named volumes** - `docker run --rm -v vol:/data alpine tar -czf - -C /data .`
- **Local directories** - a `local: true` job tarring a directory on this machine straight onto a backup drive

Runs from Linux or Windows; The remote side just needs SSH and whatever tools your commands call.

## Usage

```
backpull -config config.yaml
```

To run a subset of jobs (e.g. re-running one that failed), pass `-only` with
comma-separated job names:

```
backpull -only grafana,db
```

Jobs marked `manual: true` in the config are skipped by a plain run and only
execute when named explicitly via `-only` - useful for large, expensive jobs
(e.g. a full media directory) that you don't want in the regular schedule.

To check a config without connecting, pass `-dry-run` — it prints each job's
command and target path, then exits:

```
backpull -dry-run
```

See [config.example.yaml](config.example.yaml) for a full example:


Every job needs a unique `name`, a `command`, and an `output` filename (backpull can't guess the right extension).

## Local jobs

A job marked `local: true` runs on the machine backpull runs from, not the
server. Everything else is the same - same `output`, `output_dir`, `timeout`,
`manual` and placeholders:

```yaml
- name: notes
  local: true
  command: tar -czf - -C ~/documents notes
  output: "{date}.tar.gz"
  output_dir: "{output_path}/notes/{year}"
```

Two things to know:

- **The `ssh:` block is optional** when every job is local. backpull only
  connects if the selected jobs include a remote one, so `-only notes`
  never touches the network.
- **Commands are shell strings**, run through `sh -c` on Linux and `cmd /c` on
  Windows. A local job's command is therefore tied to the OS you run it from.

## How it works

- **One SSH connection per run.** backpull dials `host:port`, authenticates
  with `key_file` (or ssh-agent if unset - including the Windows OpenSSH
  agent), then opens one session per job over the same connection.
- **Host keys are verified against `~/.ssh/known_hosts`.** Unknown hosts are
  rejected; connect once with plain `ssh` to add the key. A changed host key
  fails with a mismatch error.
- **Commands run verbatim.** The configured `command` is passed to a shell
  exactly as written - the remote one, or the local one for `local: true`
  jobs. No wrapping. stdout streams straight to the output file; it is never
  buffered in memory.
- **Output layout is `<destination>/<timestamp>/<name>/<output>`**. Each run
  gets a fresh timestamp directory, so runs never overwrite each other.
- **`output_dir` bypasses that layout** and writes straight into the given
  directory, overwriting on a rerun. A directory built from `{output_path}` is
  created if missing - `output_path` itself must exist, which is what proves
  the drive is mounted. A literal `output_dir` is never created: a missing one
  fails the job.
- **Writes are atomic.** Output streams to `<output>.partial` and is only
  renamed into place after the command exits 0 and the file is synced. An
  interrupted or failed transfer never leaves a file that looks like a valid
  backup.
- **Failures are loud.** A non-zero exit fails the job with the command's
  stderr in the error message; a command that produces no output is also
  treated as a failure. A failed job does not stop the run - the remaining
  jobs still run, every job's result is listed in a summary at the end, and
  backpull exits non-zero if any of them failed.
- **Logging** goes to stdout and `logs/app.log`. Set `log.level: debug` to see
  every remote command as it executes; commands that write to stderr but still
  succeed (e.g. `pg_dump` warnings) are logged as warnings.

## Parsing parameters

- In the config, you can define parameters that will be parsed dynamically when executing a command.
- I add these as I need them, so no fully dynamic system yet.

### Currently supported:
- `{date}` - Current date, format yyyy-mm-dd
- `{year}` - Current year