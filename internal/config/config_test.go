package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validYAML = `
ssh:
  host: server.example.com
  user: deploy
  port: 2222
  key_file: ~/.ssh/id_ed25519

log:
  level: debug

destination: D:/backups

jobs:
  - name: postgres
    command: docker exec postgres pg_dump -U app appdb
    output: appdb.sql

  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz

  - name: grafana
    command: docker run --rm -v grafana-data:/data alpine tar -czf - -C /data .
    output: grafana-data.tar.gz
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if cfg.SSH.Host != "server.example.com" {
		t.Errorf("SSH.Host = %q, want %q", cfg.SSH.Host, "server.example.com")
	}
	if cfg.SSH.User != "deploy" {
		t.Errorf("SSH.User = %q, want %q", cfg.SSH.User, "deploy")
	}
	if cfg.SSH.Port != 2222 {
		t.Errorf("SSH.Port = %d, want 2222", cfg.SSH.Port)
	}
	if cfg.SSH.KeyFile != "~/.ssh/id_ed25519" {
		t.Errorf("SSH.KeyFile = %q, want %q", cfg.SSH.KeyFile, "~/.ssh/id_ed25519")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Destination != "D:/backups" {
		t.Errorf("Destination = %q, want %q", cfg.Destination, "D:/backups")
	}
	if len(cfg.Jobs) != 3 {
		t.Fatalf("len(Jobs) = %d, want 3", len(cfg.Jobs))
	}

	j := cfg.Jobs[0]
	if j.Name != "postgres" || j.Command != "docker exec postgres pg_dump -U app appdb" || j.Output != "appdb.sql" {
		t.Errorf("unexpected job: %+v", j)
	}
}

func TestParseDefaultPort(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.SSH.Port != 22 {
		t.Errorf("SSH.Port = %d, want default 22", cfg.SSH.Port)
	}
}

func TestParseJobTimeout(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: postgres
    command: docker exec postgres pg_dump -U app appdb
    output: appdb.sql
    timeout: 30m
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got := time.Duration(cfg.Jobs[0].Timeout); got != 30*time.Minute {
		t.Errorf("Jobs[0].Timeout = %s, want 30m", got)
	}
	if got := time.Duration(cfg.Jobs[1].Timeout); got != 2*time.Minute {
		t.Errorf("Jobs[1].Timeout = %s, want default 2m", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing ssh host",
			yaml: `
ssh:
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "ssh.host",
		},
		{
			name: "missing ssh user",
			yaml: `
ssh:
  host: server.example.com
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "ssh.user",
		},
		{
			name: "port out of range",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
  port: 70000
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "ssh.port",
		},
		{
			name: "negative port",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
  port: -1
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "ssh.port",
		},
		{
			name: "missing destination",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "destination",
		},
		{
			name: "no jobs",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs: []
`,
			wantErr: "jobs",
		},
		{
			name: "missing jobs key",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
`,
			wantErr: "jobs",
		},
		{
			name: "missing job name",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
`,
			wantErr: "name",
		},
		{
			name: "duplicate job names",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
  - name: caddy
    command: tar -czf - -C /etc/caddy .
    output: caddy-etc.tar.gz
`,
			wantErr: `duplicate job name "caddy"`,
		},
		{
			name: "missing command",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    output: caddy.tar.gz
`,
			wantErr: `job "caddy": command is required`,
		},
		{
			name: "missing output",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
`,
			wantErr: `job "caddy": output is required`,
		},
		{
			name: "output is a path",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: ../caddy.tar.gz
`,
			wantErr: "must be a filename",
		},
		{
			name: "negative timeout",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    timeout: -5m
`,
			wantErr: "timeout",
		},
		{
			name: "timeout over maximum",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    timeout: 13h
`,
			wantErr: "timeout",
		},
		{
			name: "unparseable timeout",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    timeout: fast
`,
			wantErr: "invalid duration",
		},
		{
			name: "unknown key",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    outputt: caddy.tar.gz
`,
			wantErr: "not found",
		},
		{
			name:    "invalid yaml syntax",
			yaml:    "ssh: [unclosed",
			wantErr: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("Parse succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backpull.yaml")
	if err := os.WriteFile(path, []byte(validYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Jobs) != 3 {
		t.Errorf("len(Jobs) = %d, want 3", len(cfg.Jobs))
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("Load succeeded on missing file, want error")
	}
}
