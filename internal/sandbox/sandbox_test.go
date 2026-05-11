package sandbox

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/profile"
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

	spec, err := Build(cfg, m, newProfile(), projectRoot, []string{"--flag", "value"})
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
	if !contains(spec.Tmpfs, "/tmp:rw,nosuid,nodev,size=512m") {
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
	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "fullstack"}, m, p, t.TempDir(), nil)
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
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil)
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
	spec, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil)
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
	spec, _ := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, newProfile(), t.TempDir(), nil)
	if contains(spec.EnvPassthrough, "UNSET_TOKEN") {
		t.Errorf("unset env should not be propagated")
	}
}

func TestBuildRestrictedProfileErrors(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Network = "restricted"
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, p, t.TempDir(), nil)
	if err == nil {
		t.Fatal("restricted profile should error in v1")
	}
}

func TestBuildUnknownProfileErrors(t *testing.T) {
	configDir := t.TempDir()
	m := newManifest(t, configDir)
	p := newProfile()
	p.Network = "made-up"
	_, err := Build(config.Config{ImageRegistry: "x", Toolchain: "f"}, m, p, t.TempDir(), nil)
	if err == nil {
		t.Fatal("unknown network profile should error")
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
