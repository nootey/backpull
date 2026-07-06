package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
ssh:
  host: server.example.com
  user: deploy
  port: 2222
  key_file: ~/.ssh/id_ed25519

destination: D:/backups

services:
  - name: postgres
    type: db_dump
    container: postgres
    command: pg_dump -U app appdb

  - name: caddy
    type: config_dir
    path: /srv/caddy

  - name: grafana
    type: volume
    volume: grafana-data
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
	if cfg.Destination != "D:/backups" {
		t.Errorf("Destination = %q, want %q", cfg.Destination, "D:/backups")
	}
	if len(cfg.Services) != 3 {
		t.Fatalf("len(Services) = %d, want 3", len(cfg.Services))
	}

	db := cfg.Services[0]
	if db.Name != "postgres" || db.Type != TypeDBDump || db.Container != "postgres" || db.Command != "pg_dump -U app appdb" {
		t.Errorf("unexpected db_dump service: %+v", db)
	}
	dir := cfg.Services[1]
	if dir.Name != "caddy" || dir.Type != TypeConfigDir || dir.Path != "/srv/caddy" {
		t.Errorf("unexpected config_dir service: %+v", dir)
	}
	vol := cfg.Services[2]
	if vol.Name != "grafana" || vol.Type != TypeVolume || vol.Volume != "grafana-data" {
		t.Errorf("unexpected volume service: %+v", vol)
	}
}

func TestParseDefaultPort(t *testing.T) {
	yaml := `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.SSH.Port != 22 {
		t.Errorf("SSH.Port = %d, want default 22", cfg.SSH.Port)
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
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
`,
			wantErr: "ssh.host",
		},
		{
			name: "missing ssh user",
			yaml: `
ssh:
  host: server.example.com
destination: /backups
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
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
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
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
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
`,
			wantErr: "ssh.port",
		},
		{
			name: "missing destination",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
`,
			wantErr: "destination",
		},
		{
			name: "no services",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services: []
`,
			wantErr: "services",
		},
		{
			name: "missing services key",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
`,
			wantErr: "services",
		},
		{
			name: "missing service name",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - type: config_dir
    path: /srv/caddy
`,
			wantErr: "name",
		},
		{
			name: "duplicate service names",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: caddy
    type: config_dir
    path: /srv/caddy
  - name: caddy
    type: volume
    volume: caddy-data
`,
			wantErr: `duplicate service name "caddy"`,
		},
		{
			name: "invalid type",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: caddy
    type: snapshot
    path: /srv/caddy
`,
			wantErr: `service "caddy"`,
		},
		{
			name: "db_dump missing container",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: postgres
    type: db_dump
    command: pg_dump -U app appdb
`,
			wantErr: `service "postgres"`,
		},
		{
			name: "db_dump missing command",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: postgres
    type: db_dump
    container: postgres
`,
			wantErr: `service "postgres"`,
		},
		{
			name: "config_dir missing path",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: caddy
    type: config_dir
`,
			wantErr: `service "caddy"`,
		},
		{
			name: "volume missing volume",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: grafana
    type: volume
`,
			wantErr: `service "grafana"`,
		},
		{
			name: "field from another type",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: grafana
    type: volume
    volume: grafana-data
    path: /srv/grafana
`,
			wantErr: `service "grafana"`,
		},
		{
			name: "unknown key",
			yaml: `
ssh:
  host: server.example.com
  user: deploy
destination: /backups
services:
  - name: caddy
    type: config_dir
    pathh: /srv/caddy
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
	if len(cfg.Services) != 3 {
		t.Errorf("len(Services) = %d, want 3", len(cfg.Services))
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("Load succeeded on missing file, want error")
	}
}
