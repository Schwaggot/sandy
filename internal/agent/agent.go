package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/schwaggot/sandy/internal/assets"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Name           string        `yaml:"name"`
	Description    string        `yaml:"description"`
	Image          string        `yaml:"image"`
	Command        []string      `yaml:"command"`
	EnvPassthrough []string      `yaml:"env_passthrough"`
	ConfigMounts   []ConfigMount `yaml:"config_mounts"`
	// Model declares how this agent is told which model to use. Absent means
	// sandy leaves model selection entirely to the agent's own config.
	Model *ModelSpec `yaml:"model"`
}

// ModelSpec is the agent-specific wiring for a model sandy resolved from an
// endpoint's /models listing. Templates may use {{model}}, {{provider}},
// {{protocol}}, {{url}} and {{context}}.
type ModelSpec struct {
	// Flag is appended to the agent's argv as "<flag> <rendered format>".
	Flag string `yaml:"flag"`
	// Format renders the flag's value, e.g. "{{provider}}/{{model}}".
	Format string `yaml:"format"`
	// Aliases are other spellings of Flag. If the user passes any of them,
	// sandy injects nothing and their choice stands.
	Aliases []string `yaml:"aliases"`
	// Args are extra argv entries appended after the model flag, rendered from
	// the same templates. Agents that must be told which protocol to speak
	// need it here (qwen via --auth-type).
	Args []string `yaml:"args"`
	// Env holds extra environment variables rendered from the same templates.
	// Agents that reject a model missing from their config need it declared
	// here (opencode via OPENCODE_CONFIG_CONTENT).
	Env map[string]string `yaml:"env"`
}

// defaultContextWindow is used when an endpoint does not advertise n_ctx but
// the agent's config injection needs a number.
const defaultContextWindow = 131072

// ModelVars are the substitutions for a ModelSpec template.
type ModelVars struct {
	Model    string
	Provider string
	Protocol string
	URL      string
	Context  int
}

// Expand renders a ModelSpec template.
func (v ModelVars) Expand(tmpl string) string {
	ctx := v.Context
	if ctx <= 0 {
		ctx = defaultContextWindow
	}
	if v.Provider == "" {
		// Drop the separator too, so "{{provider}}/{{model}}" degrades to
		// a bare model id rather than "/model".
		tmpl = strings.ReplaceAll(tmpl, "{{provider}}/", "")
	}
	r := strings.NewReplacer(
		"{{model}}", v.Model,
		"{{provider}}", v.Provider,
		"{{protocol}}", v.Protocol,
		"{{url}}", v.URL,
		"{{context}}", strconv.Itoa(ctx),
	)
	return r.Replace(tmpl)
}

// UserPinned reports whether args already pin a model, in which case sandy
// must not inject one.
func (s ModelSpec) UserPinned(args []string) bool {
	names := append([]string{s.Flag}, s.Aliases...)
	for _, a := range args {
		for _, n := range names {
			if n == "" {
				continue
			}
			if a == n || strings.HasPrefix(a, n+"=") {
				return true
			}
		}
	}
	return false
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
