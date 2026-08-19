package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// placeholders in output/output_dir; expansion happens in util.Expand
var placeholderRe = regexp.MustCompile(`\{[^{}]*\}`)

const (
	defaultJobTimeout = 2 * time.Minute
	maxJobTimeout     = 12 * time.Hour
)

type Config struct {
	SSH         SSH    `yaml:"ssh"`
	Log         Log    `yaml:"log"`
	Destination string `yaml:"destination"`
	// OutputPath expands the {output_path} placeholder in a job's output_dir.
	OutputPath string `yaml:"output_path"`
	Jobs       []Job  `yaml:"jobs"`
}

type SSH struct {
	Host    string `yaml:"host"`
	User    string `yaml:"user"`
	Port    int    `yaml:"port"`
	KeyFile string `yaml:"key_file"`
}

type Log struct {
	Level string `yaml:"level"`
}

type Job struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Output  string   `yaml:"output"`
	Timeout Duration `yaml:"timeout"`
	// OutputDir, when set, receives the output file directly, bypassing the
	// destination's <timestamp>/<name> layout. It must already exist.
	// Supports the same placeholders as Output.
	OutputDir string `yaml:"output_dir"`
	// Manual jobs are skipped unless named explicitly via -only.
	Manual bool `yaml:"manual"`
	// Local jobs run on this machine instead of the remote host.
	Local bool `yaml:"local"`
	// OutputRoot is the config's output_path, set only for jobs whose
	// output_dir was built from {output_path}. Those directories may be
	// created, because the root existing proves the drive is mounted.
	OutputRoot string `yaml:"-"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.SSH.Port == 0 {
		cfg.SSH.Port = 22
	}
	for i := range cfg.Jobs {
		if cfg.Jobs[i].Timeout == 0 {
			cfg.Jobs[i].Timeout = Duration(defaultJobTimeout)
		}
		// {output_path} is static, so expand it here with parameters
		// depend on run time and are expanded later by util.Expand
		if strings.Contains(cfg.Jobs[i].OutputDir, "{output_path}") {
			if cfg.OutputPath == "" {
				return nil, fmt.Errorf("job %q: output_dir uses {output_path}, but output_path is not set", cfg.Jobs[i].Name)
			}
			cfg.Jobs[i].OutputDir = strings.ReplaceAll(cfg.Jobs[i].OutputDir, "{output_path}", cfg.OutputPath)
			cfg.Jobs[i].OutputRoot = cfg.OutputPath
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// HasRemoteJobs reports whether any job needs an SSH connection.
func HasRemoteJobs(jobs []Job) bool {
	for _, j := range jobs {
		if !j.Local {
			return true
		}
	}
	return false
}

func (c *Config) validate() error {
	// a config of only local jobs never connects, so ssh may be omitted
	if HasRemoteJobs(c.Jobs) {
		if c.SSH.Host == "" {
			return errors.New("ssh.host is required")
		}
		if c.SSH.User == "" {
			return errors.New("ssh.user is required")
		}
		if c.SSH.Port < 1 || c.SSH.Port > 65535 {
			return fmt.Errorf("ssh.port %d is out of range 1-65535", c.SSH.Port)
		}
	}
	if c.Destination == "" {
		return errors.New("destination is required")
	}
	if len(c.Jobs) == 0 {
		return errors.New("jobs must list at least one job")
	}

	seen := make(map[string]bool, len(c.Jobs))
	seenPath := make(map[string]string) // cleaned output_dir path -> job name
	for i, j := range c.Jobs {
		if j.Name == "" {
			return fmt.Errorf("jobs[%d]: name is required", i)
		}
		if seen[j.Name] {
			return fmt.Errorf("duplicate job name %q", j.Name)
		}
		seen[j.Name] = true

		if err := j.validate(); err != nil {
			return fmt.Errorf("job %q: %w", j.Name, err)
		}

		// jobs without output_dir land in <timestamp>/<name>/, so unique
		// names already keep them apart; only output_dir jobs can collide
		if j.OutputDir != "" {
			p := filepath.Clean(filepath.Join(j.OutputDir, j.Output))
			if prev, ok := seenPath[p]; ok {
				return fmt.Errorf("job %q: output_dir path %s already used by job %q", j.Name, p, prev)
			}
			seenPath[p] = j.Name
		}
	}
	return nil
}

func (j *Job) validate() error {
	if j.Command == "" {
		return errors.New("command is required")
	}
	if j.Output == "" {
		return errors.New("output is required")
	}
	// output is joined into the run directory, so it must be a bare filename
	if strings.ContainsAny(j.Output, `/\`) {
		return fmt.Errorf("output %q must be a filename, not a path", j.Output)
	}
	if err := validatePlaceholders("output", j.Output); err != nil {
		return err
	}
	if err := validatePlaceholders("output_dir", j.OutputDir); err != nil {
		return err
	}
	if d := time.Duration(j.Timeout); d <= 0 || d > maxJobTimeout {
		return fmt.Errorf("timeout %s must be between 0 and %s", d, maxJobTimeout)
	}
	return nil
}

func validatePlaceholders(field, s string) error {
	for _, p := range placeholderRe.FindAllString(s, -1) {
		if p != "{date}" && p != "{year}" && p != "{month}" {
			return fmt.Errorf("%s %q contains unknown placeholder %s (supported: {date}, {year}, {month})", field, s, p)
		}
	}
	return nil
}
