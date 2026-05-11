package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRegistry = "ghcr.io/schwaggot"
	DefaultProfile  = "open"
	DefaultToolchain = "fullstack"
)

// Config is the merged effective configuration after layering.
type Config struct {
	Profile          string   `yaml:"profile"`
	Toolchain        string   `yaml:"toolchain"`
	ImageRegistry    string   `yaml:"image_registry"`
	AllowlistDomains []string `yaml:"allowlist_domains"`
	Runtime          string   `yaml:"runtime"` // docker | podman
}

func defaults() Config {
	return Config{
		Profile:       DefaultProfile,
		Toolchain:     DefaultToolchain,
		ImageRegistry: DefaultRegistry,
		Runtime:       "docker",
	}
}

// Load resolves the three-layer config:
// built-in defaults < ~/.sandy/config.yaml < <projectRoot>/.sandy/config.yaml.
// projectRoot may be empty.
func Load(projectRoot string) (Config, error) {
	cfg := defaults()

	if home, err := os.UserHomeDir(); err == nil {
		if err := mergeFile(&cfg, filepath.Join(home, ".sandy", "config.yaml")); err != nil {
			return cfg, err
		}
	}

	if projectRoot != "" {
		if err := mergeFile(&cfg, filepath.Join(projectRoot, ".sandy", "config.yaml")); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	var layer Config
	if err := yaml.Unmarshal(b, &layer); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	merge(cfg, layer)
	return nil
}

func merge(dst *Config, src Config) {
	if src.Profile != "" {
		dst.Profile = src.Profile
	}
	if src.Toolchain != "" {
		dst.Toolchain = src.Toolchain
	}
	if src.ImageRegistry != "" {
		dst.ImageRegistry = src.ImageRegistry
	}
	if src.Runtime != "" {
		dst.Runtime = src.Runtime
	}
	// Allowlist appends.
	dst.AllowlistDomains = append(dst.AllowlistDomains, src.AllowlistDomains...)
}

// Write serializes a project config to <projectRoot>/.sandy/config.yaml.
func Write(projectRoot string, cfg Config) error {
	dir := filepath.Join(projectRoot, ".sandy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.yaml"), b, 0o644)
}
