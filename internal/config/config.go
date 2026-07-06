package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	TypeDBDump    = "db_dump"
	TypeConfigDir = "config_dir"
	TypeVolume    = "volume"
)

type Config struct {
	SSH         SSH       `yaml:"ssh"`
	Destination string    `yaml:"destination"`
	Services    []Service `yaml:"services"`
}

type SSH struct {
	Host    string `yaml:"host"`
	User    string `yaml:"user"`
	Port    int    `yaml:"port"`
	KeyFile string `yaml:"key_file"`
}

type Service struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`

	// db_dump
	Container string `yaml:"container"`
	Command   string `yaml:"command"`

	// config_dir
	Path string `yaml:"path"`

	// volume
	Volume string `yaml:"volume"`
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
	if len(c.Services) == 0 {
		return errors.New("services must list at least one service")
	}

	seen := make(map[string]bool, len(c.Services))
	for i, s := range c.Services {
		if s.Name == "" {
			return fmt.Errorf("services[%d]: name is required", i)
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate service name %q", s.Name)
		}
		seen[s.Name] = true

		if err := s.validate(); err != nil {
			return fmt.Errorf("service %q: %w", s.Name, err)
		}
	}
	return nil
}

func (s *Service) validate() error {
	var required, forbidden []field
	switch s.Type {
	case TypeDBDump:
		required = []field{{"container", s.Container}, {"command", s.Command}}
		forbidden = []field{{"path", s.Path}, {"volume", s.Volume}}
	case TypeConfigDir:
		required = []field{{"path", s.Path}}
		forbidden = []field{{"container", s.Container}, {"command", s.Command}, {"volume", s.Volume}}
	case TypeVolume:
		required = []field{{"volume", s.Volume}}
		forbidden = []field{{"container", s.Container}, {"command", s.Command}, {"path", s.Path}}
	default:
		return fmt.Errorf("type %q is not one of %s, %s, %s", s.Type, TypeDBDump, TypeConfigDir, TypeVolume)
	}

	for _, f := range required {
		if f.value == "" {
			return fmt.Errorf("%s is required for type %s", f.name, s.Type)
		}
	}
	for _, f := range forbidden {
		if f.value != "" {
			return fmt.Errorf("%s is not allowed for type %s", f.name, s.Type)
		}
	}
	return nil
}

type field struct {
	name  string
	value string
}
