package agent

import (
	"encoding/json"
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

func TestModelVarsExpand(t *testing.T) {
	v := ModelVars{Model: "m1", Provider: "p1", URL: "http://h/v1", Context: 4096}
	if got := v.Expand("{{provider}}/{{model}}"); got != "p1/m1" {
		t.Errorf("got %q", got)
	}
	if got := v.Expand(`{"u":"{{url}}","c":{{context}}}`); got != `{"u":"http://h/v1","c":4096}` {
		t.Errorf("got %q", got)
	}
	// No provider: the separator goes with it.
	bare := ModelVars{Model: "m1"}
	if got := bare.Expand("{{provider}}/{{model}}"); got != "m1" {
		t.Errorf("got %q", got)
	}
	if got := bare.Expand("{{context}}"); got != "131072" {
		t.Errorf("unadvertised context should fall back, got %q", got)
	}
}

func TestModelSpecUserPinned(t *testing.T) {
	s := ModelSpec{Flag: "--model", Aliases: []string{"-m"}}
	for _, args := range [][]string{{"--model", "x"}, {"-m", "x"}, {"--model=x"}, {"run", "-m=x"}} {
		if !s.UserPinned(args) {
			t.Errorf("%v should count as pinned", args)
		}
	}
	for _, args := range [][]string{nil, {"--modelfoo"}, {"hello"}} {
		if s.UserPinned(args) {
			t.Errorf("%v should not count as pinned", args)
		}
	}
}

// The opencode manifest injects its provider config as JSON; a typo there
// would only surface at runtime inside the container.
func TestBundledModelEnvTemplatesRenderValidJSON(t *testing.T) {
	agents, _, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	v := ModelVars{Model: "m1", Provider: "p1", URL: "http://h:8090/v1", Context: 262144}
	for name, m := range agents {
		if m.Model == nil {
			continue
		}
		for key, tmpl := range m.Model.Env {
			rendered := v.Expand(tmpl)
			if !strings.HasPrefix(strings.TrimSpace(rendered), "{") {
				continue
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(rendered), &out); err != nil {
				t.Errorf("%s %s: %v\n%s", name, key, err, rendered)
			}
		}
	}
}
