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

func TestParseJobOutputDir(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: wealth-warden
    command: docker exec wealth-warden-db-1 pg_dump -U postgres wealth_warden
    output: "{date}.sql"
    output_dir: H:\_backup\_current\wealth_warden
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
  - name: wealth-warden-alt
    command: docker exec wealth-warden-db-1 pg_dump -U postgres wealth_warden
    output: "{date}.sql"
    output_dir: H:\_backup\_alt\wealth_warden
  - name: homepage
    command: tar -czf - -C ~/services/homepage config data docker-compose.yml
    output: "{date}.tar.gz"
    output_dir: /backups/{year}/homepage
`
	// same output filename in a different output_dir must not be a collision
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got := cfg.Jobs[0].OutputDir; got != `H:\_backup\_current\wealth_warden` {
		t.Errorf("Jobs[0].OutputDir = %q, want %q", got, `H:\_backup\_current\wealth_warden`)
	}
	if got := cfg.Jobs[1].OutputDir; got != "" {
		t.Errorf("Jobs[1].OutputDir = %q, want empty", got)
	}
	if got := cfg.Jobs[3].OutputDir; got != "/backups/{year}/homepage" {
		t.Errorf("Jobs[3].OutputDir = %q, want %q (placeholders expand later, at run time)", got, "/backups/{year}/homepage")
	}
}

func TestParseOutputPath(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
output_path: /mnt/hdd/_backup
jobs:
  - name: wealth-warden
    command: docker exec wealth-warden-db-1 pg_dump -U postgres wealth_warden
    output: "{date}.sql"
    output_dir: "{output_path}/wealth_warden/{year}"
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    output_dir: /backups/caddy
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	want := "/mnt/hdd/_backup/wealth_warden/{year}"
	if got := cfg.Jobs[0].OutputDir; got != want {
		t.Errorf("Jobs[0].OutputDir = %q, want %q ({year} expands later, at run time)", got, want)
	}
	if got := cfg.Jobs[1].OutputDir; got != "/backups/caddy" {
		t.Errorf("Jobs[1].OutputDir = %q, want %q", got, "/backups/caddy")
	}
	// only {output_path} jobs may have their directories created
	if got := cfg.Jobs[0].OutputRoot; got != "/mnt/hdd/_backup" {
		t.Errorf("Jobs[0].OutputRoot = %q, want %q", got, "/mnt/hdd/_backup")
	}
	if got := cfg.Jobs[1].OutputRoot; got != "" {
		t.Errorf("Jobs[1].OutputRoot = %q, want empty for a literal output_dir", got)
	}
}

func TestParseJobManual(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: nextcloud-data
    command: tar -czf - -C ~/services/nextcloud data
    output: nextcloud-data.tar.gz
    manual: true
  - name: nextcloud-db
    command: docker exec nextcloud-db mysqldump -u root nextcloud
    output: nextcloud-db.sql
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.Jobs[0].Manual {
		t.Errorf("Jobs[0].Manual = false, want true")
	}
	if cfg.Jobs[1].Manual {
		t.Errorf("Jobs[1].Manual = true, want default false")
	}
}

func TestParseJobLocal(t *testing.T) {
	yaml := `
destination: /backups
output_path: /mnt/hdd/_backup
jobs:
  - name: notes
    local: true
    command: tar -czf - -C /home/user/documents notes
    output: "{date}.tar.gz"
    output_dir: "{output_path}/notes/{year}"
`
	// a config of only local jobs never connects, so ssh may be omitted
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.Jobs[0].Local {
		t.Errorf("Jobs[0].Local = false, want true")
	}
	if HasRemoteJobs(cfg.Jobs) {
		t.Errorf("HasRemoteJobs = true, want false for an all-local config")
	}
}

func TestHasRemoteJobs(t *testing.T) {
	if !HasRemoteJobs([]Job{{Name: "notes", Local: true}, {Name: "db"}}) {
		t.Errorf("HasRemoteJobs = false, want true when one job is remote")
	}
	if HasRemoteJobs(nil) {
		t.Errorf("HasRemoteJobs(nil) = true, want false")
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
			name: "ssh omitted but a job is remote",
			yaml: `
destination: /backups
jobs:
  - name: notes
    local: true
    command: tar -czf - -C /home/user/documents notes
    output: notes.tar.gz
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
			name: "unknown placeholder in output",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: "{daet}.tar.gz"
`,
			wantErr: "unknown placeholder {daet}",
		},
		{
			name: "unknown placeholder in output_dir",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    output_dir: /backups/{decade}/caddy
`,
			wantErr: "unknown placeholder {decade}",
		},
		{
			name: "output_path used but not set",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: caddy.tar.gz
    output_dir: "{output_path}/caddy"
`,
			wantErr: "output_path is not set",
		},
		{
			name: "output_path in output",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
output_path: /mnt/hdd/_backup
jobs:
  - name: caddy
    command: tar -czf - -C /srv/caddy .
    output: "{output_path}.tar.gz"
`,
			wantErr: "unknown placeholder {output_path}",
		},
		{
			name: "duplicate output_dir path",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
jobs:
  - name: wealth-warden
    command: docker exec wealth-warden-db-1 pg_dump -U postgres wealth_warden
    output: "{date}.sql"
    output_dir: /backups/current
  - name: wealth-warden-again
    command: docker exec wealth-warden-db-1 pg_dump -U postgres wealth_warden
    output: "{date}.sql"
    output_dir: /backups/current
`,
			wantErr: "already used by job",
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
