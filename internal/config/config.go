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

// placeholders in output names; expansion happens in util.ExpandOutput
var placeholderRe = regexp.MustCompile(`\{[^{}]*\}`)

const (
	defaultJobTimeout = 2 * time.Minute
	maxJobTimeout     = 12 * time.Hour
)

type Config struct {
	SSH         SSH    `yaml:"ssh"`
	Log         Log    `yaml:"log"`
	Destination string `yaml:"destination"`
	Jobs        []Job  `yaml:"jobs"`
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

// Job is a remote command whose stdout is captured into a local file.
type Job struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Output  string   `yaml:"output"`
	Timeout Duration `yaml:"timeout"`
	// OutputDir, when set, receives the output file directly, bypassing the
	// destination's <timestamp>/<name> layout. It must already exist.
	OutputDir string `yaml:"output_dir"`
}

// Duration is a time.Duration that unmarshals from YAML strings like "30m".
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
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.SSH.Host == "" {
		return errors.New("ssh.host is required")
	}
	if c.SSH.User == "" {
		return errors.New("ssh.user is required")
	}
	if c.SSH.Port < 1 || c.SSH.Port > 65535 {
		return fmt.Errorf("ssh.port %d is out of range 1-65535", c.SSH.Port)
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
	for _, p := range placeholderRe.FindAllString(j.Output, -1) {
		if p != "{date}" {
			return fmt.Errorf("output %q contains unknown placeholder %s (supported: {date})", j.Output, p)
		}
	}
	if d := time.Duration(j.Timeout); d <= 0 || d > maxJobTimeout {
		return fmt.Errorf("timeout %s must be between 0 and %s", d, maxJobTimeout)
	}
	return nil
}
