package project

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestHashStable(t *testing.T) {
	h1 := Hash("/tmp/a")
	h2 := Hash("/tmp/a")
	h3 := Hash("/tmp/b")
	if h1 != h2 {
		t.Errorf("hash not stable: %q vs %q", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("hash collision on different paths")
	}
	if len(h1) != 16 {
		t.Errorf("hash length want 16, got %d", len(h1))
	}
}

func TestRootFindsSandyDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sandy"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Root(sub); got != root {
		t.Errorf("Root: want %q, got %q", root, got)
	}
}

func TestRootFindsGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Root(sub); got != root {
		t.Errorf("Root: want %q, got %q", root, got)
	}
}

func TestRootFallsBackToCwd(t *testing.T) {
	dir := t.TempDir()
	if got := Root(dir); got != dir {
		t.Errorf("Root: want %q, got %q", dir, got)
	}
}

func TestRootIgnoresMarkersAtHome(t *testing.T) {
	// ~/.sandy is sandy's global config dir and $HOME may hold a dotfiles
	// .git; neither may turn the home directory into a project root.
	for _, marker := range []string{".sandy", ".git"} {
		t.Run(marker, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, marker), 0o755); err != nil {
				t.Fatal(err)
			}
			sub := filepath.Join(home, "projects", "foo")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			if got := rootFrom(sub, home); got != sub {
				t.Errorf("rootFrom: want cwd %q, got %q", sub, got)
			}
		})
	}
}

func TestRootStopsWalkingAtHome(t *testing.T) {
	// A marker above $HOME must not be reached either.
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, ".sandy"), 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	sub := filepath.Join(home, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rootFrom(sub, home); got != sub {
		t.Errorf("rootFrom: want cwd %q, got %q", sub, got)
	}
}

func TestRootFindsProjectUnderHome(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sandy"), 0o755); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(home, "projects", "foo")
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rootFrom(sub, home); got != proj {
		t.Errorf("rootFrom: want project root %q, got %q", proj, got)
	}
}

func TestRootRunDirectlyInHome(t *testing.T) {
	// Running in $HOME itself keeps cwd as root; the guard only blocks
	// implicit escalation from subdirectories.
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sandy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rootFrom(home, home); got != home {
		t.Errorf("rootFrom: want %q, got %q", home, got)
	}
}

func TestDetectToolchains(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "pyproject.toml"), "")
	mustWrite(t, filepath.Join(dir, "package.json"), "{}")
	got := DetectToolchains(dir)
	sort.Strings(got)
	want := []string{"node", "python"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPickToolchain(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want string
	}{
		"empty": {nil, "fullstack"},
		"one":   {[]string{"python"}, "python"},
		"two":   {[]string{"python", "node"}, "fullstack"},
		"three": {[]string{"python", "cpp", "node"}, "fullstack"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PickToolchain(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
