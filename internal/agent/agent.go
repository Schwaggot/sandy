package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schwaggot/sandy/internal/assets"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Name           string         `yaml:"name"`
	Description    string         `yaml:"description"`
	Image          string         `yaml:"image"`
	Command        []string       `yaml:"command"`
	EnvPassthrough []string       `yaml:"env_passthrough"`
	ConfigMounts   []ConfigMount  `yaml:"config_mounts"`
}

type ConfigMount struct {
	Host      map[string]string `yaml:"host"` // os -> path
	Container string            `yaml:"container"`
	Mode      string            `yaml:"mode"` // "ro" or "rw"
	Optional  bool              `yaml:"optional"`
}

func (m ConfigMount) HostPath() (string, bool) {
	p, ok := m.Host[runtime.GOOS]
	if !ok {
		return "", false
	}
	return expandPath(p), true
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// ResolveImage substitutes {{registry}} and {{toolchain}} placeholders.
func (m Manifest) ResolveImage(registry, toolchain string) string {
	img := m.Image
	img = strings.ReplaceAll(img, "{{registry}}", registry)
	img = strings.ReplaceAll(img, "{{toolchain}}", toolchain)
	return img
}

// LoadAll loads bundled manifests plus any in ~/.sandy/agents/.
//
// Bundled-manifest errors are fatal (they are a build-time bug). Errors in
// user manifests are reported as warnings via the returned []error so a single
// bad file in ~/.sandy/agents/ does not hide every other agent.
func LoadAll() (map[string]Manifest, []error, error) {
	out := map[string]Manifest{}
	var warnings []error

	bundled, err := fs.ReadDir(assets.Agents, "agents")
	if err != nil {
		return nil, nil, fmt.Errorf("read bundled agents: %w", err)
	}
	for _, e := range bundled {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fs.ReadFile(assets.Agents, "agents/"+e.Name())
		if err != nil {
			return nil, nil, err
		}
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			return nil, nil, fmt.Errorf("parse bundled %s: %w", e.Name(), err)
		}
		if m.Name == "" {
			return nil, nil, fmt.Errorf("bundled agent %s has empty name", e.Name())
		}
		out[m.Name] = m
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return out, warnings, nil
	}
	dir := filepath.Join(home, ".sandy", "agents")
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
		var m Manifest
		if err := yaml.Unmarshal(b, &m); err != nil {
			warnings = append(warnings, fmt.Errorf("parse %s: %w", path, err))
			continue
		}
		if m.Name == "" {
			warnings = append(warnings, fmt.Errorf("%s: empty name field", path))
			continue
		}
		out[m.Name] = m
	}

	return out, warnings, nil
}

func Get(name string) (Manifest, error) {
	all, _, err := LoadAll()
	if err != nil {
		return Manifest{}, err
	}
	m, ok := all[name]
	if !ok {
		return Manifest{}, fmt.Errorf("unknown agent %q", name)
	}
	return m, nil
}
