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
		"empty":  {nil, "fullstack"},
		"one":    {[]string{"python"}, "python"},
		"two":    {[]string{"python", "node"}, "fullstack"},
		"three":  {[]string{"python", "cpp", "node"}, "fullstack"},
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
