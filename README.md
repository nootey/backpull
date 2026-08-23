# backpull

A Go CLI that runs commands on a remote server over SSH. It writes
the output of those commands to files on the local machine.

Each job in the config is one command. Command output is captured into a local
file. What the command does does not matter. It runs, and the bytes are stored.
Jobs run on the remote host by default. Set `local: true` to run a job on this
machine.

Use cases:

- Database dumps. Run `pg_dump` or `mysqldump` in a Docker container with `docker exec`.
- Config and data directories. Run `tar -czf -` on a directory on the server.
- Docker named volumes. Run `docker run --rm -v vol:/data alpine tar -czf - -C /data .`
- Local directories. A `local: true` job writes a tar of a local directory to a backup drive.

OS independent. The remote host needs SSH and the tools that the commands call.

## Usage

```
backpull -config config.yaml
// or
make
```

Use `-only` to run some of the jobs. Give the job names, separated by commas.
This is useful when one job failed.

```
make only=grafana,db
```

A job with `manual: true` is skipped in a full run. It runs only when you name
it with `-only`. Use this for large, slow jobs, for example a full media
directory.

Use `-dry-run` to check a config. The command and target path of each job are
printed, then the run stops. No connection is made.

```
backpull -dry-run
```

## Configuration

Commands are defined in `config.yaml`, you must create your own.

[config.example.yaml](config.example.yaml) shows a full example. Each job needs a
unique `name`, a `command` and an `output` filename. The correct file extension
cannot be guessed, so they must be provided.

### Local jobs

A job with `local: true` runs on this machine, not on the server. The other
fields do not change.

The `ssh` block is not necessary if all jobs are local. A connection is made
only when a selected job is remote. Thus `-only notes` does not use the network.

## How it works

- One SSH connection per run. The connection goes to `host:port`.
  Authentication uses `key_file`. If the `key_file` is not set, the ssh-agent is
  used. This includes the Windows OpenSSH agent. One session per job is opened
  on that connection.
- Host keys are verified against `~/.ssh/known_hosts`. Unknown hosts are
  rejected.
- Commands run without changes. Each command goes to a shell exactly written.
  Stored directly to the output file, the data is never held in memory.
- The default output path is `<destination>/<timestamp>/<name>/<output>`. Each
  run makes a new timestamp directory, making idempotency configurable.
- `output_dir` replaces that layout. Output goes into the given directory and
  overwrites the file on the next run. A directory that starts with
  `{output_path}` is created if it is missing. `output_path` itself must exist.
  If that directory is missing, the job fails.
- Writes are atomic. Output goes to `<output>.partial`. The file is renamed only
  after the command exits with 0 and the file is synced. A failed transfer
  cannot leave a file that looks like a good backup.
- Failures are loud. A non-zero exit fails the job. The error message contains
  the stderr of the command. A command that gives no output also fails. One
  failed job does not stop the run. The other jobs continue. All results are
  listed in a summary at the end.
- Logs go to stdout and to `logs/app.log`. Set `log.level: debug` to see each
  command as it runs. A command that writes to stderr but is successful is
  logged as a warning. Example: `pg_dump` warnings.

## Placeholders

Placeholders in the config are replaced when a job runs. New ones are added when Needed. 
There is no fully dynamic system.

Currently supported:

- `{date}` - the current date, format yyyy-mm-dd
- `{year}` - the current year
- `{month}` - the current month, format mm with a leading zero
