# backpull

A Go CLI that runs configured commands on a remote server over SSH and pulls
their output down to the local machine as timestamped files.

Each **job** in the config is a remote command whose stdout is captured into a
local file. What the command does is entirely up to you - backpull just runs
it and streams the bytes back.

Typical use cases (and why this exists):

- **Database dumps** - `pg_dump`/`mysqldump` executed inside a running Docker container via `docker exec`
- **Config/data directories** - `tar -czf -` of a directory on the server
- **Docker named volumes** - `docker run --rm -v vol:/data alpine tar -czf - -C /data .`

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

To check a config without connecting, pass `-dry-run` — it prints each job's
command and target path, then exits:

```
backpull -dry-run
```

See [config.example.yaml](config.example.yaml) for a full example:


Every job needs a unique `name`, a `command`, and an `output` filename (backpull can't guess the right extension for you).

## How it works

- **One SSH connection per run.** backpull dials `host:port`, authenticates
  with `key_file` (or ssh-agent if unset - including the Windows OpenSSH
  agent), then opens one session per job over the same connection.
- **Host keys are verified against `~/.ssh/known_hosts`.** Unknown hosts are
  rejected; connect once with plain `ssh` to add the key. A changed host key
  fails with a mismatch error.
- **Commands run verbatim.** The configured `command` is passed to the remote
  shell exactly as written - no wrapping, no local shell involved. stdout
  streams straight to the output file; it is never buffered in memory.
- **Output layout is `<destination>/<timestamp>/<name>/<output>`**. Each run
  gets a fresh timestamp directory, so runs never overwrite each other.
- **Writes are atomic.** Output streams to `<output>.partial` and is only
  renamed into place after the command exits 0 and the file is synced. An
  interrupted or failed transfer never leaves a file that looks like a valid
  backup.
- **Failures are loud.** A non-zero exit fails the job with the command's
  stderr in the error message; a command that produces no output is also
  treated as a failure. The run stops at the first failed job.
- **Logging** goes to stdout and `logs/app.log`. Set `log.level: debug` to see
  every remote command as it executes; commands that write to stderr but still
  succeed (e.g. `pg_dump` warnings) are logged as warnings.

## Planned
- Currently, only `{date}` parsing is supported to dynamically overwrite the output names per job
  - Support for more formats could be added
- Multiple hosts