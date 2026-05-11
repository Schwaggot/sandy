package profile

import "testing"

func TestLoadAllBundled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	profs, warnings, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	for _, name := range []string{"default", "open", "offline"} {
		p, ok := profs[name]
		if !ok {
			t.Errorf("missing bundled profile %q", name)
			continue
		}
		if p.Resources.Pids == 0 {
			t.Errorf("profile %q has zero pids limit", name)
		}
	}
	if profs["offline"].Network != "offline" {
		t.Errorf("offline profile network: %q", profs["offline"].Network)
	}
}

func TestGetUnknown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := Get("nope"); err == nil {
		t.Error("expected error for unknown profile")
	}
}
