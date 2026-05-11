package runtime

import "fmt"

// RunSpec is the runtime-agnostic description of one container invocation.
type RunSpec struct {
	Image string
	Command []string
	Args    []string
	WorkDir string
	// Env are explicit key=value pairs. Always rendered as `-e KEY=VALUE`.
	Env map[string]string
	// EnvPassthrough are variable names whose values come from the caller's
	// environment. Rendered as `-e KEY` (no value on the command line) so
	// secrets are not exposed in dry-run output or process listings.
	EnvPassthrough []string
	Mounts         []Mount
	Network        string // "none", "bridge", "" (default)
	AddHosts       map[string]string
	User           string // "uid:gid"
	ReadOnly       bool
	Tmpfs          []string
	CapDrop        []string
	CapAdd         []string
	SecurityOpts   []string
	PidsLimit      int
	Memory         string
	CPUs           string
	Interactive    bool
	TTY            bool
	AutoRemove     bool
	Name           string
}

type Mount struct {
	Source   string // host path or volume name
	Target   string
	ReadOnly bool
	Volume   bool // true = named volume, false = bind
}

type Runtime interface {
	Name() string
	Run(spec RunSpec) error
	BuildArgs(spec RunSpec) []string // for --dry-run
	Build(contextDir, dockerfile, tag string) error
	Pull(image string) error
	VolumeRemove(name string) error
	VolumeList(prefix string) ([]string, error)
	Available() error
}

func Select(name string) (Runtime, error) {
	switch name {
	case "", "docker":
		return NewDocker(), nil
	case "podman":
		return nil, fmt.Errorf("podman runtime not yet implemented")
	default:
		return nil, fmt.Errorf("unknown runtime %q", name)
	}
}
