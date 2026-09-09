package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := defaults()
	if d.Profile != DefaultProfile || d.Toolchain != DefaultToolchain || d.ImageRegistry != DefaultRegistry {
		t.Fatalf("unexpected defaults: %+v", d)
	}
}

func TestLoadNoFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != DefaultProfile || cfg.Toolchain != DefaultToolchain {
		t.Fatalf("expected defaults, got %+v", cfg)
	}
}

func TestLoadLayering(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// User-level overrides profile and adds an allowlist entry.
	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte("profile: offline\nallowlist_domains:\n  - user.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Project-level overrides toolchain and adds another allowlist entry.
	proj := t.TempDir()
	projCfg := filepath.Join(proj, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projCfg, []byte("toolchain: python\nallowlist_domains:\n  - project.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(proj)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profile != "offline" {
		t.Errorf("profile: want offline, got %q", cfg.Profile)
	}
	if cfg.Toolchain != "python" {
		t.Errorf("toolchain: want python, got %q", cfg.Toolchain)
	}
	want := []string{"user.example", "project.example"}
	if !reflect.DeepEqual(cfg.AllowlistDomains, want) {
		t.Errorf("allowlist: want %v, got %v", want, cfg.AllowlistDomains)
	}
}

func TestLoadExtraMountsAppend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"extra_mounts:\n  - source: ~/shared\n    target: /workspace/shared\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	proj := t.TempDir()
	projCfg := filepath.Join(proj, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projCfg, []byte(
		"extra_mounts:\n  - source: ../sibling\n    target: /workspace/sibling\n    mode: rw\n  - source: /etc/ssl/certs\n    target: /etc/ssl/certs\n    optional: true\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ExtraMounts) != 3 {
		t.Fatalf("want 3 mounts (1 user + 2 project), got %d: %+v", len(cfg.ExtraMounts), cfg.ExtraMounts)
	}
	if cfg.ExtraMounts[0].Source != "~/shared" || cfg.ExtraMounts[0].Target != "/workspace/shared" {
		t.Errorf("user mount: %+v", cfg.ExtraMounts[0])
	}
	if cfg.ExtraMounts[1].Mode != "rw" {
		t.Errorf("project mount mode: %+v", cfg.ExtraMounts[1])
	}
	if !cfg.ExtraMounts[2].Optional {
		t.Errorf("optional flag did not parse: %+v", cfg.ExtraMounts[2])
	}
}

func TestLoadExtraHostsMergeProjectWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"extra_hosts:\n  halo: 10.0.0.5\n  shared: 10.0.0.9\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	proj := t.TempDir()
	projCfg := filepath.Join(proj, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projCfg, []byte(
		"extra_hosts:\n  halo: 192.168.1.50\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(proj)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ExtraHosts["halo"] != "192.168.1.50" {
		t.Errorf("project should override user for key collisions: %v", cfg.ExtraHosts)
	}
	if cfg.ExtraHosts["shared"] != "10.0.0.9" {
		t.Errorf("user-only host should remain: %v", cfg.ExtraHosts)
	}
}

func TestLoadAgentEndpointsMergeByProtocol(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"agents:\n  opencode:\n    endpoints:\n      - protocol: openai\n        url: http://halo:8080/v1\n        add_host: 192.168.1.50\n      - protocol: anthropic\n        url: https://api.anthropic.com\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	proj := t.TempDir()
	projCfg := filepath.Join(proj, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projCfg, []byte(
		"agents:\n  opencode:\n    endpoints:\n      - protocol: openai\n        url: https://api.openai.com/v1\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(proj)
	if err != nil {
		t.Fatal(err)
	}
	eps := cfg.Agents["opencode"].Endpoints
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints, got %d: %+v", len(eps), eps)
	}
	var openai, anthropic *Endpoint
	for i := range eps {
		switch eps[i].Protocol {
		case "openai":
			openai = &eps[i]
		case "anthropic":
			anthropic = &eps[i]
		}
	}
	if openai == nil || openai.URL != "https://api.openai.com/v1" {
		t.Errorf("project should replace user's openai entry: %+v", openai)
	}
	if anthropic == nil || anthropic.URL != "https://api.anthropic.com" {
		t.Errorf("user's anthropic entry should survive: %+v", anthropic)
	}
}

func TestLoadAgentEndpointsProjectAddsNewProtocol(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"agents:\n  opencode:\n    endpoints:\n      - protocol: openai\n        url: http://halo:8080/v1\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := t.TempDir()
	projCfg := filepath.Join(proj, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projCfg, []byte(
		"agents:\n  opencode:\n    endpoints:\n      - protocol: anthropic\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(proj)
	if err != nil {
		t.Fatal(err)
	}
	eps := cfg.Agents["opencode"].Endpoints
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints (openai from user + anthropic from project), got %d: %+v", len(eps), eps)
	}
}

func TestWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Config{Profile: "open", Toolchain: "cpp", ImageRegistry: "ghcr.io/test", Runtime: "docker"}
	if err := Write(dir, in); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir()) // no user config
	out, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.Profile != in.Profile || out.Toolchain != in.Toolchain || out.ImageRegistry != in.ImageRegistry {
		t.Fatalf("round-trip mismatch: in=%+v out=%+v", in, out)
	}
}

func TestLoadEndpointCACert(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"agents:\n  pi:\n    endpoints:\n      - protocol: openai\n        url: https://gpu/v1\n        ca_cert: ~/certs/internal.crt\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	eps := cfg.Agents["pi"].Endpoints
	if len(eps) != 1 || eps[0].CACert != "~/certs/internal.crt" {
		t.Fatalf("ca_cert not parsed: %+v", eps)
	}
}

func TestResolveCACertRejectsNonCertificate(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(secret, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCACert(secret, ""); err == nil {
		t.Fatal("ca_cert must not accept a file holding no certificate")
	}
	if _, err := ResolveCACert(filepath.Join(dir, "absent.crt"), ""); err == nil {
		t.Fatal("ca_cert must not accept a missing file")
	}
}

func TestResolveHostPathRejectsTildeUser(t *testing.T) {
	if _, err := ResolveHostPath("~other/certs/ca.crt", "/proj", "ca_cert"); err == nil {
		t.Fatal("~user form must be rejected")
	}
	if _, err := ResolveHostPath("certs/ca.crt", "", "ca_cert"); err == nil {
		t.Fatal("a relative path without a project root must be rejected")
	}
}

// no_auth and ca_cert are only ever set from YAML, so the struct tags are the
// contract: a typo there disables the feature while every Go-level test still
// passes.
func TestLoadEndpointNoAuth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userCfg := filepath.Join(home, ".sandy", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(userCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userCfg, []byte(
		"agents:\n  qwen:\n    endpoints:\n      - protocol: openai\n        url: https://gpu/v1\n        no_auth: true\n      - protocol: anthropic\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	eps := cfg.Agents["qwen"].Endpoints
	if len(eps) != 2 {
		t.Fatalf("endpoints: %+v", eps)
	}
	if !eps[0].NoAuth {
		t.Errorf("no_auth not parsed: %+v", eps[0])
	}
	if eps[1].NoAuth {
		t.Errorf("no_auth must default to false: %+v", eps[1])
	}
}
