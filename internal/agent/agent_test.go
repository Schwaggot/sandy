package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadAllBundled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	agents, warnings, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	for _, name := range []string{"pi", "opencode", "claude"} {
		if _, ok := agents[name]; !ok {
			t.Errorf("missing bundled agent %q", name)
		}
	}
}

func TestResolveImage(t *testing.T) {
	m := Manifest{Image: "{{registry}}/sandy-pi-{{toolchain}}:latest"}
	got := m.ResolveImage("ghcr.io/x", "python")
	want := "ghcr.io/x/sandy-pi-python:latest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandPathTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := expandPath("~/foo/bar")
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfigMountHostPath(t *testing.T) {
	t.Setenv("HOME", "/h")
	cm := ConfigMount{
		Host: map[string]string{
			"linux":   "~/.foo",
			"darwin":  "~/.foo",
			"windows": "~/.foo",
		},
	}
	got, ok := cm.HostPath()
	if !ok {
		t.Fatal("expected ok for current OS")
	}
	if !strings.HasSuffix(got, ".foo") {
		t.Errorf("path does not end with .foo: %q", got)
	}

	missing := ConfigMount{Host: map[string]string{"plan9": "/x"}}
	if _, ok := missing.HostPath(); ok && runtime.GOOS != "plan9" {
		t.Error("expected !ok for OS not in map")
	}
}

func TestLoadUserOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".sandy", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Override the bundled claude manifest.
	override := []byte("name: claude\ndescription: custom\nimage: custom:latest\ncommand: [foo]\n")
	if err := os.WriteFile(filepath.Join(dir, "claude.yaml"), override, 0o644); err != nil {
		t.Fatal(err)
	}
	agents, warnings, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if agents["claude"].Description != "custom" {
		t.Errorf("user override did not win: %+v", agents["claude"])
	}
}

func TestLoadAllSkipsBrokenUserManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".sandy", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(":::"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents, warnings, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 {
		t.Errorf("expected a warning for the broken manifest")
	}
	// Bundled agents must still be present.
	if _, ok := agents["claude"]; !ok {
		t.Errorf("bundled agents should still load when a user manifest is broken")
	}
}
