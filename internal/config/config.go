package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Output  string `yaml:"output"`
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
	return nil
}
