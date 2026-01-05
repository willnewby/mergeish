package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFile = "mergeish.yml"

// RepoConfig represents a single repository configuration
type RepoConfig struct {
	URL  string `yaml:"url"`
	Path string `yaml:"path"`
}

// Settings represents optional configuration settings
type Settings struct {
	DefaultBranch string `yaml:"default_branch"`
	Parallel      bool   `yaml:"parallel"`
}

// Config represents the mergeish.yml configuration file
type Config struct {
	Repos    []RepoConfig `yaml:"repos"`
	Settings Settings     `yaml:"settings"`
}

// DefaultConfig returns a config with default settings
func DefaultConfig() *Config {
	return &Config{
		Repos: []RepoConfig{},
		Settings: Settings{
			DefaultBranch: "main",
			Parallel:      true,
		},
	}
}

// Load reads and parses a config file from the given path
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	return Parse(data)
}

// Parse parses config from YAML bytes
func Parse(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks the config for errors
func (c *Config) Validate() error {
	seen := make(map[string]bool)
	for i, repo := range c.Repos {
		if repo.URL == "" {
			return fmt.Errorf("repo %d: url is required", i)
		}
		if repo.Path == "" {
			return fmt.Errorf("repo %d: path is required", i)
		}
		if seen[repo.Path] {
			return fmt.Errorf("repo %d: duplicate path %q", i, repo.Path)
		}
		seen[repo.Path] = true
	}
	return nil
}

// Save writes the config to the given path
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// FindConfigFile searches for mergeish.yml starting from the given directory
// and walking up to parent directories
func FindConfigFile(startDir string) (string, error) {
	dir := startDir
	for {
		path := filepath.Join(dir, DefaultConfigFile)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("config file %s not found", DefaultConfigFile)
}
