package sandbox

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/inference"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/schwaggot/sandy/internal/runtime"
)

func newManifest(t *testing.T, configDir string) agent.Manifest {
	t.Helper()
	return agent.Manifest{
		Name:           "test",
		Image:          "{{registry}}/sandy-test-{{toolchain}}:latest",
		Command:        []string{"test-agent"},
		EnvPassthrough: []string{"TEST_TOKEN"},
		ConfigMounts: []agent.ConfigMount{
			{
				Host:      map[string]string{"linux": configDir, "darwin": configDir, "windows": configDir},
				Container: "/home/sandy/.test",
				Mode:      "ro",
			},
		},
	}
}

func newProfile() profile.Profile {
	return profile.Profile{
		Name:    "default",
		Network: "open",
		Resources: profile.Resources{
			Memory: "4g",
			CPUs:   "2",
			Pids:   1024,
		},
		Hardening: profile.Hardening{
			CapDrop:         []string{"ALL"},
			NoNewPrivileges: true,
			ReadOnlyRootfs:  true,
		},
	}
}

func TestBuildBasic(t *testing.T) {
	t.Setenv("TEST_TOKEN", "abc")
	configDir := t.TempDir()
	projectRoot := t.TempDir()

	m := newManifest(t, configDir)
	cfg := config.Config{ImageRegistry: "ghcr.io/x", Toolchain: "python"}

	spec, err := Build(cfg, m, newProfile(), projectRoot, []string{"--flag", "value"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if spec.Image != "ghcr.io/x/sandy-test-python:latest" {
		t.Errorf("image: %q", spec.Image)
	}
	if spec.WorkDir != "/workspace" {
		t.Errorf("workdir: %q", spec.WorkDir)
	}
	if !spec.AutoRemove || !spec.Interactive || !spec.TTY {
		t.Errorf("flags: rm=%v i=%v t=%v", spec.AutoRemove, spec.Interactive, spec.TTY)
	}
	if spec.AddHosts["host.docker.internal"] != "host-gateway" {
		t.Errorf("missing host.docker.internal add-host: %v", spec.AddHosts)
	}
	if !contains(spec.EnvPassthrough, "TEST_TOKEN") {
		t.Errorf("env passthrough should be by name only, got %v", spec.EnvPassthrough)
	}
	if _, leaked := spec.Env["TEST_TOKEN"]; leaked {
		t.Errorf("passthrough value must not be copied into Env (would leak in dry-run): %v", spec.Env)
	}
	if spec.Env["SANDY"] != "1" {
		t.Errorf("SANDY env missing")
	}
	if spec.Memory != "4g" || spec.CPUs != "2" || spec.PidsLimit != 1024 {
		t.Errorf("resource limits: mem=%q cpus=%q pids=%d", spec.Memory, spec.CPUs, spec.PidsLimit)
	}
	if !spec.ReadOnly {
		t.Errorf("read-only rootfs not set")
	}
	if !contains(spec.SecurityOpts, "no-new-privileges") {
		t.Errorf("no-new-privileges missing: %v", spec.SecurityOpts)
	}
	if !contains(spec.CapDrop, "ALL") {
		t.Errorf("cap-drop ALL missing")
	}
	if !contains(spec.Tmpfs, "/tmp:rw,exec,nosuid,nodev,size=512m") {
		t.Errorf("tmpfs /tmp missing: %v", spec.Tmpfs)
	}

	// Mounts: CWD bind RW + home volume + config RO.
	if len(spec.Mounts) != 3 {
		t.Fatalf("want 3 mounts, got %d: %+v", len(spec.Mounts), spec.Mounts)
	}
	if spec.Mounts[0].Source != projectRoot || spec.Mounts[0].Target != "/workspace" || spec.Mounts[0].ReadOnly {
		t.Errorf("cwd mount wrong: %+v", spec.Mounts[0])
	}
	if !spec.Mounts[1].Volume || !strings.HasPrefix(spec.Mounts[1].Source, "sandy-home-") || spec.Mounts[1].Target != "/home/sandy" {
		t.Errorf("home volume wrong: %+v", spec.Mounts[1])
	}
	if spec.Mounts[2].Source != configDir || spec.Mounts[2].Target != "/home/sandy/.test" || !spec.Mounts[2].ReadOnly {
		t.Errorf("config mount wrong: %+v", spec.Mounts[2])
	}

	// User flag only on Linux.
	if goruntime.GOOS == "linux" && spec.User == "" {
		t.Errorf("expected --user on linux")
	}
	if goruntime.GOOS != "linux" && spec.User != "" {
		t.Errorf("unexpected --user on %s: %q", goruntime.GOOS, spec.User)
	}

	// Args passed through.
	if len(spec.Args) != 2 || spec.Args[0] != "--flag" || spec.Args[1] != "value" {
		t.Errorf("args: %v", spec.Args)
	}
}

func TestBuildOfflineNetwork(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Network = "offline"
	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "fullstack"}, m, p, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Network != "none" {
		t.Errorf("network: want none, got %q", spec.Network)
	}
}

func TestBuildRequiredMountMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	m := agent.Manifest{
		Image:   "x:y",
		Command: []string{"a"},
		ConfigMounts: []agent.ConfigMount{{
			Host:      map[string]string{"linux": missing, "darwin": missing, "windows": missing},
			Container: "/c",
			Mode:      "ro",
		}},
	}
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error for missing required mount")
	}
}

func TestBuildOptionalMountSkipped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	m := agent.Manifest{
		Image:   "x:y",
		Command: []string{"a"},
		ConfigMounts: []agent.ConfigMount{{
			Host:      map[string]string{"linux": missing, "darwin": missing, "windows": missing},
			Container: "/c",
			Mode:      "ro",
			Optional:  true,
		}},
	}
	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("optional missing mount should not error: %v", err)
	}
	for _, mt := range spec.Mounts {
		if mt.Target == "/c" {
			t.Errorf("optional missing mount should be skipped")
		}
	}
}

func TestBuildPassthroughOnlyExported(t *testing.T) {
	if err := os.Unsetenv("UNSET_TOKEN"); err != nil {
		t.Fatal(err)
	}
	m := agent.Manifest{
		Image:          "x:y",
		Command:        []string{"a"},
		EnvPassthrough: []string{"UNSET_TOKEN"},
	}
	spec, _ := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, nil)
	if contains(spec.EnvPassthrough, "UNSET_TOKEN") {
		t.Errorf("unset env should not be propagated")
	}
}

func TestBuildRestrictedProfileErrors(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Network = "restricted"
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, p, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("restricted profile should error in v1")
	}
}

func TestBuildUnknownProfileErrors(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Network = "made-up"
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, p, t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("unknown network profile should error")
	}
}

func TestBuildExtraMountAbsoluteReadOnlyByDefault(t *testing.T) {
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: src, Target: "/workspace/shared"}},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findMount(spec.Mounts, "/workspace/shared")
	if m == nil {
		t.Fatalf("extra mount not present: %+v", spec.Mounts)
	}
	if m.Source != src {
		t.Errorf("source: want %q got %q", src, m.Source)
	}
	if !m.ReadOnly {
		t.Errorf("default mode must be read-only")
	}
	if m.Volume {
		t.Errorf("extra mount must be a bind mount, not a volume")
	}
}

func TestBuildExtraMountRWMode(t *testing.T) {
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: src, Target: "/workspace/shared", Mode: "rw"}},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findMount(spec.Mounts, "/workspace/shared")
	if m == nil || m.ReadOnly {
		t.Fatalf("expected RW mount, got %+v", m)
	}
}

func TestBuildExtraMountRelativeToProjectRoot(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "proj")
	sibling := filepath.Join(parent, "sibling")
	for _, d := range []string{project, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: "../sibling", Target: "/workspace/sibling"}},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), project, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findMount(spec.Mounts, "/workspace/sibling")
	if m == nil {
		t.Fatalf("relative mount missing: %+v", spec.Mounts)
	}
	if m.Source != sibling {
		t.Errorf("source: want %q got %q", sibling, m.Source)
	}
}

func TestBuildExtraMountTildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "shared")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: "~/shared", Target: "/workspace/shared"}},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findMount(spec.Mounts, "/workspace/shared")
	if m == nil || m.Source != sub {
		t.Fatalf("tilde expansion failed: %+v", m)
	}
}

func TestBuildExtraMountMissingRequiredErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: missing, Target: "/workspace/x"}},
	}
	_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error for missing required extra mount source")
	}
}

func TestBuildExtraMountMissingOptionalSkipped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: missing, Target: "/workspace/x", Optional: true}},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("optional missing source should not error: %v", err)
	}
	if findMount(spec.Mounts, "/workspace/x") != nil {
		t.Errorf("optional missing source should be skipped")
	}
}

func TestBuildExtraMountTargetCollisionRejected(t *testing.T) {
	src := t.TempDir()
	for _, target := range []string{"/workspace", "/home/sandy"} {
		cfg := config.Config{
			ImageRegistry: "x", Toolchain: "f",
			ExtraMounts: []config.ExtraMount{{Source: src, Target: target}},
		}
		_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
		if err == nil {
			t.Errorf("target %q should be rejected", target)
		}
	}
}

func TestBuildExtraMountRelativeTargetRejected(t *testing.T) {
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: src, Target: "shared"}},
	}
	_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("relative target must be rejected")
	}
}

func TestBuildOpenAIEndpoint(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{{
				Protocol: "openai", URL: "http://halo:8080/v1", AddHost: "192.168.1.50",
			}}},
		},
	}
	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["OPENAI_BASE_URL"] != "http://halo:8080/v1" {
		t.Errorf("OPENAI_BASE_URL: %v", spec.Env)
	}
	if !contains(spec.EnvPassthrough, "OPENAI_API_KEY") {
		t.Errorf("OPENAI_API_KEY should be in passthrough: %v", spec.EnvPassthrough)
	}
	if spec.AddHosts["halo"] != "192.168.1.50" {
		t.Errorf("add_host should parse URL hostname: %v", spec.AddHosts)
	}
}

func TestBuildAnthropicEndpointDefaultURLOmitsBaseURL(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "claude"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"claude": {Endpoints: []config.Endpoint{{Protocol: "anthropic"}}}, // no URL
		},
	}
	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Env["ANTHROPIC_BASE_URL"]; ok {
		t.Errorf("ANTHROPIC_BASE_URL must NOT be set for default anthropic cloud: %v", spec.Env)
	}
	if !contains(spec.EnvPassthrough, "ANTHROPIC_API_KEY") {
		t.Errorf("ANTHROPIC_API_KEY should be in passthrough: %v", spec.EnvPassthrough)
	}
}

func TestBuildAnthropicEndpointCustomURLSetsBaseURL(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "claude"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"claude": {Endpoints: []config.Endpoint{{
				Protocol: "anthropic", URL: "https://proxy.example.com",
			}}},
		},
	}
	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["ANTHROPIC_BASE_URL"] != "https://proxy.example.com" {
		t.Errorf("ANTHROPIC_BASE_URL: %v", spec.Env)
	}
}

func TestBuildEndpointMultipleProtocols(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{
				{Protocol: "openai", URL: "http://halo:8080/v1"},
				{Protocol: "anthropic", URL: "https://proxy.example.com"},
			}},
		},
	}
	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["OPENAI_BASE_URL"] == "" || spec.Env["ANTHROPIC_BASE_URL"] == "" {
		t.Errorf("both base URLs must be set: %v", spec.Env)
	}
}

func TestBuildEndpointUnknownProtocolRejected(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{{Protocol: "bedrock", URL: "x"}}},
		},
	}
	_, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("unknown protocol must error")
	}
}

func TestBuildEndpointDuplicateProtocolRejected(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{
				{Protocol: "openai", URL: "http://halo:8080/v1"},
				{Protocol: "openai", URL: "http://other:8080/v1"},
			}},
		},
	}
	_, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("duplicate protocol within an agent must error")
	}
}

func TestBuildOpenAIEndpointRequiresURL(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{{Protocol: "openai"}}},
		},
	}
	_, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("openai protocol requires url")
	}
}

func TestBuildEndpointAddHostReservedRejected(t *testing.T) {
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"opencode": {Endpoints: []config.Endpoint{{
				Protocol: "openai", URL: "http://host.docker.internal:8080/v1", AddHost: "1.2.3.4",
			}}},
		},
	}
	_, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("add_host must not override host.docker.internal")
	}
}

func TestBuildEndpointsScopedToCurrentAgent(t *testing.T) {
	// Endpoints under a different agent name should NOT affect this run.
	m := newManifest(t, t.TempDir())
	m.Name = "opencode"
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		Agents: map[string]config.AgentConfig{
			"pi": {Endpoints: []config.Endpoint{{Protocol: "openai", URL: "http://halo:8080/v1"}}},
		},
	}
	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Env["OPENAI_BASE_URL"]; ok {
		t.Errorf("only the active agent's endpoints should apply: %v", spec.Env)
	}
}

func TestBuildExtraHostsPropagated(t *testing.T) {
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraHosts: map[string]string{"halo": "192.168.1.50", "registry.lan": "10.0.0.7"},
	}
	spec, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.AddHosts["halo"] != "192.168.1.50" {
		t.Errorf("halo entry missing/wrong: %v", spec.AddHosts)
	}
	if spec.AddHosts["registry.lan"] != "10.0.0.7" {
		t.Errorf("second host missing: %v", spec.AddHosts)
	}
	if spec.AddHosts["host.docker.internal"] != "host-gateway" {
		t.Errorf("built-in host.docker.internal must remain: %v", spec.AddHosts)
	}
}

func TestBuildExtraHostsReservedNameRejected(t *testing.T) {
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraHosts: map[string]string{"host.docker.internal": "10.0.0.1"},
	}
	_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("overriding the reserved host.docker.internal must error")
	}
}

func TestBuildExtraHostsRequiresValues(t *testing.T) {
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraHosts: map[string]string{"halo": ""},
	}
	_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("empty IP must be rejected")
	}
}

func TestBuildReadOnlyWorkspace(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Hardening.ReadOnlyWorkspace = true

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, p, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cwd := findMount(spec.Mounts, "/workspace")
	if cwd == nil {
		t.Fatalf("/workspace mount missing: %+v", spec.Mounts)
	}
	if !cwd.ReadOnly {
		t.Errorf("workspace must be read-only when profile sets ReadOnlyWorkspace")
	}
}

func TestBuildReadOnlyWorkspaceWithRWExtraMount(t *testing.T) {
	// RW extra_mount should remain writable even when the workspace is RO,
	// because docker applies each mount independently.
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: src, Target: "/workspace/scratch", Mode: "rw"}},
	}
	p := newProfile()
	p.Hardening.ReadOnlyWorkspace = true

	spec, err := Build(cfg, newManifest(t, t.TempDir()), p, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cwd := findMount(spec.Mounts, "/workspace"); cwd == nil || !cwd.ReadOnly {
		t.Errorf("workspace should be RO: %+v", cwd)
	}
	if scratch := findMount(spec.Mounts, "/workspace/scratch"); scratch == nil || scratch.ReadOnly {
		t.Errorf("RW extra mount should override the RO workspace: %+v", scratch)
	}
}

func TestBuildExtraMountTildeUserFormRejected(t *testing.T) {
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: "~bob/shared", Target: "/workspace/x"}},
	}
	_, err := Build(cfg, newManifest(t, src), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("~user form must be rejected, not silently treated as current user's home")
	}
}

func TestBuildExtraMountInvalidMode(t *testing.T) {
	src := t.TempDir()
	cfg := config.Config{
		ImageRegistry: "x", Toolchain: "f",
		ExtraMounts: []config.ExtraMount{{Source: src, Target: "/workspace/x", Mode: "weird"}},
	}
	_, err := Build(cfg, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("unknown mode must be rejected")
	}
}

func findMount(ms []runtime.Mount, target string) *runtime.Mount {
	for i := range ms {
		if ms[i].Target == target {
			return &ms[i]
		}
	}
	return nil
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func modelManifest(t *testing.T, spec *agent.ModelSpec) agent.Manifest {
	t.Helper()
	m := newManifest(t, t.TempDir())
	m.Model = spec
	return m
}

func TestBuildInjectsResolvedModel(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{Flag: "--model", Format: "{{provider}}/{{model}}"})
	sel := []inference.Selection{{
		Model:    inference.Model{ID: "Qwen3.8-27B-think-MTP", Context: 262144},
		BaseURL:  "http://gpu01:8090/v1",
		Provider: "llama-cpp-gpu01",
	}}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), []string{"hello"}, sel)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"hello", "--model", "llama-cpp-gpu01/Qwen3.8-27B-think-MTP"}
	if strings.Join(spec.Args, " ") != strings.Join(want, " ") {
		t.Errorf("args: %v", spec.Args)
	}
}

func TestBuildModelWithoutProviderDropsSeparator(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{Flag: "--model", Format: "{{provider}}/{{model}}"})
	sel := []inference.Selection{{Model: inference.Model{ID: "claude-x"}, BaseURL: "http://h/v1"}}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 2 || spec.Args[1] != "claude-x" { // no user args, so flag then value
		t.Errorf("args: %v", spec.Args)
	}
}

// A model the user pinned on the command line always beats discovery.
func TestBuildUserPinnedModelWins(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{Flag: "--model", Aliases: []string{"-m"}, Format: "{{model}}"})
	sel := []inference.Selection{{Model: inference.Model{ID: "discovered"}, BaseURL: "http://h/v1"}}

	for _, args := range [][]string{{"--model", "mine"}, {"-m", "mine"}, {"--model=mine"}} {
		spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), args, sel)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(spec.Args, " ") != strings.Join(args, " ") {
			t.Errorf("args %v: got %v", args, spec.Args)
		}
	}
}

func TestBuildModelEnvInjection(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{
		Flag:   "--model",
		Format: "{{provider}}/{{model}}",
		Env:    map[string]string{"AGENT_CONFIG": `{"url":"{{url}}","model":"{{model}}","ctx":{{context}}}`},
	})
	sel := []inference.Selection{{
		Model:    inference.Model{ID: "m1", Context: 262144},
		BaseURL:  "http://gpu01:8090/v1",
		Provider: "p1",
	}}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"url":"http://gpu01:8090/v1","model":"m1","ctx":262144}`
	if spec.Env["AGENT_CONFIG"] != want {
		t.Errorf("AGENT_CONFIG: %q", spec.Env["AGENT_CONFIG"])
	}
}

// An endpoint that does not advertise n_ctx still has to render valid config.
func TestBuildModelContextFallback(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{Env: map[string]string{"CTX": "{{context}}"}})
	sel := []inference.Selection{{Model: inference.Model{ID: "m1"}, BaseURL: "http://h/v1"}}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Env["CTX"] != "131072" {
		t.Errorf("CTX: %q", spec.Env["CTX"])
	}
}

// No manifest model block, or a failed lookup, must leave the run untouched.
func TestBuildWithoutModelSpecOrSelection(t *testing.T) {
	sel := []inference.Selection{{Model: inference.Model{ID: "m1"}}}
	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, newManifest(t, t.TempDir()), newProfile(), t.TempDir(), []string{"x"}, sel)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 1 {
		t.Errorf("no model spec must not inject args: %v", spec.Args)
	}

	m := modelManifest(t, &agent.ModelSpec{Flag: "--model", Format: "{{model}}"})
	spec, err = Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), []string{"x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 1 {
		t.Errorf("failed discovery must not inject args: %v", spec.Args)
	}
}

// Config order decides which endpoint's model is used.
func TestBuildFirstResolvedEndpointWins(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{Flag: "--model", Format: "{{model}}"})
	sel := []inference.Selection{
		{Model: inference.Model{ID: "first"}, BaseURL: "http://a/v1"},
		{Model: inference.Model{ID: "second"}, BaseURL: "http://b/v1"},
	}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Args[1] != "first" {
		t.Errorf("args: %v", spec.Args)
	}
}

// qwen has to be told which protocol to speak, or a stale ~/.qwen/settings.json
// routes the sandbox back to the cloud.
func TestBuildInjectsModelArgs(t *testing.T) {
	m := modelManifest(t, &agent.ModelSpec{
		Flag:   "--model",
		Format: "{{model}}",
		Args:   []string{"--auth-type", "{{protocol}}"},
	})
	sel := []inference.Selection{{
		Model:    inference.Model{ID: "m1"},
		BaseURL:  "http://gpu01:8090/v1",
		Protocol: "openai",
	}}

	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "m1", "--auth-type", "openai"}
	if strings.Join(spec.Args, " ") != strings.Join(want, " ") {
		t.Errorf("args: %v", spec.Args)
	}
}

// The bundled qwen manifest is the reason model args exist: end to end it must
// pin both the model and the protocol at the configured endpoint.
func TestBuildQwenPinsEndpointAndProtocol(t *testing.T) {
	// Isolated home: read the bundled manifest, not a ~/.sandy/agents/ override,
	// and leave ~/.qwen missing so the optional config mount is skipped.
	t.Setenv("HOME", t.TempDir())
	m, err := agent.Get("qwen")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ImageRegistry: "x", Toolchain: "f", Agents: map[string]config.AgentConfig{
		"qwen": {Endpoints: []config.Endpoint{{Protocol: "openai", URL: "http://gpu01:8090/v1"}}},
	}}
	sel := []inference.Selection{{
		Model:    inference.Model{ID: "Qwen3-Coder"},
		BaseURL:  "http://gpu01:8090/v1",
		Protocol: "openai",
	}}

	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, sel)
	if err != nil {
		t.Fatal(err)
	}
	want := "--model Qwen3-Coder --auth-type openai"
	if strings.Join(spec.Args, " ") != want {
		t.Errorf("args: %v", spec.Args)
	}
	if spec.Env["OPENAI_BASE_URL"] != "http://gpu01:8090/v1" {
		t.Errorf("OPENAI_BASE_URL: %q", spec.Env["OPENAI_BASE_URL"])
	}
	if spec.Env["OPENAI_MODEL"] != "Qwen3-Coder" {
		t.Errorf("OPENAI_MODEL: %q", spec.Env["OPENAI_MODEL"])
	}
	if !contains(spec.EnvPassthrough, "OPENAI_API_KEY") {
		t.Errorf("api key must be forwarded by name: %v", spec.EnvPassthrough)
	}
}

// applyEndpoints and the manifest both name the protocol's API key. Docker
// must not be handed it twice, or every dry-run of an agent with a matching
// endpoint prints a duplicate -e.
func TestBuildEnvPassthroughDeduped(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "k")
	m := newManifest(t, t.TempDir())
	m.EnvPassthrough = []string{"OPENAI_API_KEY"}
	cfg := config.Config{ImageRegistry: "x", Toolchain: "f", Agents: map[string]config.AgentConfig{
		"test": {Endpoints: []config.Endpoint{{Protocol: "openai", URL: "http://h/v1"}}},
	}}

	spec, err := Build(cfg, m, newProfile(), t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, k := range spec.EnvPassthrough {
		if k == "OPENAI_API_KEY" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("OPENAI_API_KEY forwarded %d times: %v", n, spec.EnvPassthrough)
	}
}
