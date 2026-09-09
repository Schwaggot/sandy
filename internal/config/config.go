package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRegistry  = "ghcr.io/schwaggot"
	DefaultProfile   = "open"
	DefaultToolchain = "fullstack"
	// AnthropicCloudURL is the stock Anthropic endpoint. Configuring it
	// explicitly is a no-op: sandy leaves the standard cloud client path alone.
	AnthropicCloudURL = "https://api.anthropic.com"
)

// Config is the merged effective configuration after layering.
type Config struct {
	Profile          string       `yaml:"profile"`
	Toolchain        string       `yaml:"toolchain"`
	ImageRegistry    string       `yaml:"image_registry"`
	AllowlistDomains []string     `yaml:"allowlist_domains"`
	Runtime          string       `yaml:"runtime"` // docker | podman
	ExtraMounts      []ExtraMount `yaml:"extra_mounts"`
	// ExtraHosts maps a container-visible hostname to a host-reachable IP,
	// rendered as docker --add-host. Project entries override user entries
	// on key collision. Reserved: "host.docker.internal" (sandy sets it).
	ExtraHosts map[string]string `yaml:"extra_hosts"`
	// Agents holds per-agent configuration. The map key is the agent name
	// (matches the manifest's name field).
	Agents map[string]AgentConfig `yaml:"agents"`
}

// AgentConfig holds per-agent settings. Currently only inference endpoints.
type AgentConfig struct {
	Endpoints []Endpoint `yaml:"endpoints"`
}

// Endpoint describes one inference endpoint wired into the agent.
// Sandy translates this into per-protocol env vars and optional --add-host.
type Endpoint struct {
	Protocol string `yaml:"protocol"` // openai | anthropic
	URL      string `yaml:"url"`      // required for openai; optional for anthropic
	AddHost  string `yaml:"add_host"` // optional; IP for the URL hostname when DNS cannot resolve it
	// Provider is the agent-side provider id this endpoint is registered
	// under (pi's models.json key, opencode's provider key). Needed by agents
	// whose model flag takes a "provider/model" pair.
	Provider string `yaml:"provider"`
	// Prefer holds glob patterns tried in order when the endpoint serves more
	// than one model. No model id is ever configured statically; this only
	// breaks the tie. First pattern matching a served model wins.
	Prefer []string `yaml:"prefer"`
	// CACert is a host path to a PEM CA bundle trusted for this endpoint,
	// for servers behind an internal CA. Sandy uses it for host-side model
	// discovery and bind-mounts it into the container for the agent itself.
	// Same path forms as ExtraMount.Source.
	CACert string `yaml:"ca_cert"`
}

// ExtraMount is an additional host path bound into the sandbox, declared in
// user- or project-level config. Defaults to read-only.
//
// Source may be absolute, ~-prefixed (expanded against the caller's home),
// or relative to the project root. Target must be an absolute container path
// and must not collide with sandy-managed mounts (/workspace, /home/sandy).
type ExtraMount struct {
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	Mode     string `yaml:"mode"`     // "ro" (default) | "rw"
	Optional bool   `yaml:"optional"` // skip silently if source is missing
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
	// Allowlist and extra mounts append across layers.
	dst.AllowlistDomains = append(dst.AllowlistDomains, src.AllowlistDomains...)
	dst.ExtraMounts = append(dst.ExtraMounts, src.ExtraMounts...)
	// Extra hosts merge by key (later layer wins).
	if len(src.ExtraHosts) > 0 {
		if dst.ExtraHosts == nil {
			dst.ExtraHosts = map[string]string{}
		}
		for k, v := range src.ExtraHosts {
			dst.ExtraHosts[k] = v
		}
	}
	// Per-agent endpoints merge by (agent, protocol). Project entries replace
	// user entries on protocol collision; other protocols are preserved.
	for name, srcAgent := range src.Agents {
		if dst.Agents == nil {
			dst.Agents = map[string]AgentConfig{}
		}
		merged := dst.Agents[name]
		merged.Endpoints = mergeEndpoints(merged.Endpoints, srcAgent.Endpoints)
		dst.Agents[name] = merged
	}
}

// mergeEndpoints returns the union of dst and src, where src entries replace
// dst entries with the same protocol. dst entries with no protocol match in
// src are preserved.
func mergeEndpoints(dst, src []Endpoint) []Endpoint {
	if len(src) == 0 {
		return dst
	}
	srcByProto := map[string]Endpoint{}
	for _, e := range src {
		srcByProto[e.Protocol] = e
	}
	out := make([]Endpoint, 0, len(dst)+len(src))
	seen := map[string]bool{}
	for _, e := range dst {
		if r, ok := srcByProto[e.Protocol]; ok {
			out = append(out, r)
			seen[e.Protocol] = true
		} else {
			out = append(out, e)
		}
	}
	for _, e := range src {
		if !seen[e.Protocol] {
			out = append(out, e)
		}
	}
	return out
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

// ResolveHostPath expands a user-supplied host path: ~ (or ~/...) to $HOME,
// relative paths against projectRoot. Returns an absolute, cleaned path.
// The ~user form is not supported. field names the config key for errors.
func ResolveHostPath(src, projectRoot, field string) (string, error) {
	if strings.HasPrefix(src, "~") {
		if src != "~" && !strings.HasPrefix(src, "~/") && !strings.HasPrefix(src, `~\`) {
			return "", fmt.Errorf("%s: ~user form is not supported in %q (use ~ or ~/path)", field, src)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: cannot expand %q: %w", field, src, err)
		}
		rest := strings.TrimLeft(strings.TrimPrefix(src, "~"), `/\`)
		src = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(src) {
		if projectRoot == "" {
			return "", fmt.Errorf("%s: relative source %q requires a project root", field, src)
		}
		src = filepath.Join(projectRoot, src)
	}
	return filepath.Clean(src), nil
}

// ResolveCACert resolves an endpoint's ca_cert to an absolute host path and
// requires it to hold at least one PEM certificate. The content check matters:
// a project-level config is not fully trusted, and without it any host file
// could be named here and would be bind-mounted into the container.
func ResolveCACert(src, projectRoot string) (string, error) {
	host, err := ResolveHostPath(src, projectRoot, "ca_cert")
	if err != nil {
		return "", err
	}
	pem, err := os.ReadFile(host)
	if err != nil {
		return "", fmt.Errorf("ca_cert: %w", err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		return "", fmt.Errorf("ca_cert: %s holds no PEM certificate", host)
	}
	return host, nil
}
