package profile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/schwaggot/sandy/internal/assets"
	"gopkg.in/yaml.v3"
)

type Profile struct {
	Name             string    `yaml:"name"`
	Network          string    `yaml:"network"` // offline | open | restricted
	Resources        Resources `yaml:"resources"`
	Hardening        Hardening `yaml:"hardening"`
	AllowlistDomains []string  `yaml:"allowlist_domains"`
}

type Resources struct {
	Memory string `yaml:"memory"`
	CPUs   string `yaml:"cpus"`
	Pids   int    `yaml:"pids"`
}

type Hardening struct {
	CapDrop         []string `yaml:"cap_drop"`
	CapAdd          []string `yaml:"cap_add"`
	NoNewPrivileges bool     `yaml:"no_new_privileges"`
	ReadOnlyRootfs  bool     `yaml:"read_only_rootfs"`
	// ReadOnlyWorkspace mounts the project root read-only at /workspace.
	// Useful for exploration-only sessions; rw extra_mounts still override
	// to keep specific paths writable.
	ReadOnlyWorkspace bool `yaml:"read_only_workspace"`
}

// LoadAll loads bundled profiles plus any in ~/.sandy/profiles/.
// Bundled errors are fatal; user-file errors are returned as warnings so one
// bad file does not hide every other profile.
func LoadAll() (map[string]Profile, []error, error) {
	out := map[string]Profile{}
	var warnings []error

	bundled, err := fs.ReadDir(assets.Profiles, "profiles")
	if err != nil {
		return nil, nil, err
	}
	for _, e := range bundled {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(assets.Profiles, "profiles/"+e.Name())
		if err != nil {
			return nil, nil, err
		}
		var p Profile
		if err := yaml.Unmarshal(b, &p); err != nil {
			return nil, nil, fmt.Errorf("parse bundled %s: %w", e.Name(), err)
		}
		if p.Name == "" {
			return nil, nil, fmt.Errorf("bundled profile %s has empty name", e.Name())
		}
		out[p.Name] = p
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return out, warnings, nil
	}
	dir := filepath.Join(home, ".sandy", "profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, warnings, nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("read %s: %w", path, err))
			continue
		}
		var p Profile
		if err := yaml.Unmarshal(b, &p); err != nil {
			warnings = append(warnings, fmt.Errorf("parse %s: %w", path, err))
			continue
		}
		if p.Name == "" {
			warnings = append(warnings, fmt.Errorf("%s: empty name field", path))
			continue
		}
		out[p.Name] = p
	}
	return out, warnings, nil
}

func Get(name string) (Profile, error) {
	all, _, err := LoadAll()
	if err != nil {
		return Profile{}, err
	}
	p, ok := all[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown profile %q", name)
	}
	return p, nil
}
