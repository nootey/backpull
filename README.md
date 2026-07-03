# backpull

A Go CLI for pulling backups off a self-hosted server over SSH.

Runs from any machine, connects via ssh to your server, and handles:

- **Database dumps** — `dumps` executed inside running Docker containers, streamed back over SSH
- **Config backups** — discovers and archives config/data directories for specified services (compose files, bind mounts, etc.)
- **Docker volumes** — tars and pulls named volumes for services that don't expose bind-mounted config

Backups land on the source machine as timestamped archives, ready to move to external storage.
