package sandbox

import (
	"fmt"
	"os"
	goruntime "runtime"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
)

const (
	containerHome      = "/home/sandy"
	containerWorkspace = "/workspace"
)

// Build assembles a RunSpec from the resolved config, agent manifest, profile, and project root.
func Build(cfg config.Config, m agent.Manifest, p profile.Profile, projectRoot string, args []string) (runtime.RunSpec, error) {
	hash := project.Hash(projectRoot)

	spec := runtime.RunSpec{
		Image:       m.ResolveImage(cfg.ImageRegistry, cfg.Toolchain),
		Command:     m.Command,
		Args:        args,
		WorkDir:     containerWorkspace,
		Env:         map[string]string{},
		AddHosts:    map[string]string{"host.docker.internal": "host-gateway"},
		Interactive: true,
		TTY:         true,
		AutoRemove:  true,
	}

	// CWD bind mount.
	spec.Mounts = append(spec.Mounts, runtime.Mount{
		Source: projectRoot,
		Target: containerWorkspace,
	})

	// Per-project home volume (writable).
	spec.Mounts = append(spec.Mounts, runtime.Mount{
		Source: fmt.Sprintf("sandy-home-%s", hash),
		Target: containerHome,
		Volume: true,
	})

	// Agent config mounts (read-only, layered on top of home volume).
	for _, cm := range m.ConfigMounts {
		host, ok := cm.HostPath()
		if !ok {
			continue
		}
		if _, err := os.Stat(host); err != nil {
			if cm.Optional {
				continue
			}
			return spec, fmt.Errorf("required config path missing: %s", host)
		}
		spec.Mounts = append(spec.Mounts, runtime.Mount{
			Source:   host,
			Target:   cm.Container,
			ReadOnly: cm.Mode != "rw",
		})
	}

	// Env passthrough: docker reads the value from the caller's environment.
	// We never copy the value into the command line, so secrets stay out of
	// dry-run output and process listings.
	for _, k := range m.EnvPassthrough {
		if _, ok := os.LookupEnv(k); ok {
			spec.EnvPassthrough = append(spec.EnvPassthrough, k)
		}
	}
	spec.Env["SANDY"] = "1"
	spec.Env["SANDY_HOST"] = "host.docker.internal"

	// Network.
	switch p.Network {
	case "offline":
		spec.Network = "none"
	case "restricted":
		return spec, fmt.Errorf("network profile %q is not implemented in v1; use \"open\" or \"offline\"", p.Network)
	case "open", "":
		spec.Network = ""
	default:
		return spec, fmt.Errorf("unknown network profile %q", p.Network)
	}

	// Hardening.
	spec.CapDrop = p.Hardening.CapDrop
	spec.CapAdd = p.Hardening.CapAdd
	if p.Hardening.NoNewPrivileges {
		spec.SecurityOpts = append(spec.SecurityOpts, "no-new-privileges")
	}
	spec.ReadOnly = p.Hardening.ReadOnlyRootfs
	if spec.ReadOnly {
		spec.Tmpfs = append(spec.Tmpfs, "/tmp:rw,nosuid,nodev,size=512m")
	}
	spec.PidsLimit = p.Resources.Pids
	spec.Memory = p.Resources.Memory
	spec.CPUs = p.Resources.CPUs

	// UID/GID matching on Linux only.
	if goruntime.GOOS == "linux" {
		spec.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}

	return spec, nil
}
