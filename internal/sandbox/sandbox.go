package sandbox

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/inference"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
)

const (
	containerHome      = "/home/sandy"
	containerWorkspace = "/workspace"
)

// Build assembles a RunSpec from the resolved config, agent manifest, profile,
// and project root. models holds the models discovered at the agent's
// endpoints, in config order; empty means no discovery ran (or all of it
// failed) and the agent picks its own model.
func Build(cfg config.Config, m agent.Manifest, p profile.Profile, projectRoot string, args []string, models []inference.Selection) (runtime.RunSpec, error) {
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

	// CWD bind mount. Read-only when the profile requests an
	// exploration-only posture; rw extra_mounts can still re-open specific
	// subpaths because docker applies each mount independently.
	spec.Mounts = append(spec.Mounts, runtime.Mount{
		Source:   projectRoot,
		Target:   containerWorkspace,
		ReadOnly: p.Hardening.ReadOnlyWorkspace,
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

	// Per-agent inference endpoints. Translates each endpoint into env vars
	// (OPENAI_BASE_URL / ANTHROPIC_BASE_URL), env passthrough for the API key,
	// and optional --add-host for LAN names with no DNS.
	if err := applyEndpoints(&spec, cfg.Agents[m.Name].Endpoints); err != nil {
		return spec, err
	}

	// Model discovered at launch from the endpoint's /models listing.
	applyModel(&spec, m, models)

	// Extra hosts from user/project config (rendered as --add-host).
	for name, ip := range cfg.ExtraHosts {
		if name == "host.docker.internal" {
			return spec, fmt.Errorf("extra_hosts: %q is reserved by sandy and cannot be overridden", name)
		}
		if strings.TrimSpace(name) == "" {
			return spec, fmt.Errorf("extra_hosts: hostname is required")
		}
		if strings.TrimSpace(ip) == "" {
			return spec, fmt.Errorf("extra_hosts: ip for %q is required", name)
		}
		spec.AddHosts[name] = ip
	}

	// Extra mounts from user/project config (applied after agent config mounts
	// so they can layer over the home volume or workspace).
	for _, em := range cfg.ExtraMounts {
		m, ok, err := resolveExtraMount(em, projectRoot)
		if err != nil {
			return spec, err
		}
		if !ok {
			continue
		}
		spec.Mounts = append(spec.Mounts, m)
	}

	// Env passthrough: docker reads the value from the caller's environment.
	// We never copy the value into the command line, so secrets stay out of
	// dry-run output and process listings.
	passthrough := append([]string{"TERM", "COLORTERM"}, m.EnvPassthrough...)
	for _, k := range passthrough {
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
		// Bun-based agents (opencode) extract a native .so into /tmp and
		// dlopen it; need exec on the tmpfs.
		spec.Tmpfs = append(spec.Tmpfs, "/tmp:rw,exec,nosuid,nodev,size=512m")
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

// applyEndpoints wires each endpoint into the runtime spec. Validates the
// protocol, enforces per-protocol uniqueness within the agent, and rejects
// add_host collisions with sandy-managed hostnames.
func applyEndpoints(spec *runtime.RunSpec, endpoints []config.Endpoint) error {
	seen := map[string]bool{}
	for _, ep := range endpoints {
		switch ep.Protocol {
		case "openai":
			if strings.TrimSpace(ep.URL) == "" {
				return fmt.Errorf("endpoints: openai protocol requires url")
			}
			if seen[ep.Protocol] {
				return fmt.Errorf("endpoints: duplicate %q protocol entries for the same agent", ep.Protocol)
			}
			seen[ep.Protocol] = true
			spec.Env["OPENAI_BASE_URL"] = ep.URL
			appendUnique(&spec.EnvPassthrough, "OPENAI_API_KEY")
		case "anthropic":
			if seen[ep.Protocol] {
				return fmt.Errorf("endpoints: duplicate %q protocol entries for the same agent", ep.Protocol)
			}
			seen[ep.Protocol] = true
			if u := strings.TrimSpace(ep.URL); u != "" && u != config.AnthropicCloudURL {
				spec.Env["ANTHROPIC_BASE_URL"] = u
			}
			appendUnique(&spec.EnvPassthrough, "ANTHROPIC_API_KEY")
		default:
			return fmt.Errorf("endpoints: unknown protocol %q (expected: openai, anthropic)", ep.Protocol)
		}

		if strings.TrimSpace(ep.AddHost) == "" {
			continue
		}
		urlStr := ep.URL
		if urlStr == "" {
			urlStr = config.AnthropicCloudURL
		}
		u, err := url.Parse(urlStr)
		if err != nil || u.Hostname() == "" {
			return fmt.Errorf("endpoints: cannot parse hostname from url %q: %v", urlStr, err)
		}
		host := u.Hostname()
		if host == "host.docker.internal" {
			return fmt.Errorf("endpoints: add_host cannot override reserved hostname %q", host)
		}
		spec.AddHosts[host] = ep.AddHost
	}
	return nil
}

// applyModel pins the discovered model via the manifest's model spec: an
// appended CLI flag plus any env the agent needs to accept that model. A
// model the user pinned on the command line always wins.
func applyModel(spec *runtime.RunSpec, m agent.Manifest, models []inference.Selection) {
	if m.Model == nil || len(models) == 0 || m.Model.UserPinned(spec.Args) {
		return
	}
	// First endpoint that resolved wins, so config order is the tie-breaker
	// when an agent has both an openai and an anthropic endpoint.
	sel := models[0]

	vars := agent.ModelVars{
		Model:    sel.ID,
		Provider: sel.Provider,
		URL:      sel.BaseURL,
		Context:  sel.Context,
	}
	if m.Model.Flag != "" && m.Model.Format != "" {
		// Appended, not prepended: opencode reads a flag placed before its
		// subcommand as the subcommand's own positional and starts the TUI.
		spec.Args = append(spec.Args, m.Model.Flag, vars.Expand(m.Model.Format))
	}
	for k, tmpl := range m.Model.Env {
		spec.Env[k] = vars.Expand(tmpl)
	}
}

func appendUnique(dst *[]string, v string) {
	for _, s := range *dst {
		if s == v {
			return
		}
	}
	*dst = append(*dst, v)
}

// resolveExtraMount validates and resolves a user-declared extra mount.
// Returns (mount, true, nil) on success, (_, false, nil) when an optional
// source is missing, and (_, _, err) on a fatal misconfiguration.
func resolveExtraMount(em config.ExtraMount, projectRoot string) (runtime.Mount, bool, error) {
	if strings.TrimSpace(em.Source) == "" {
		return runtime.Mount{}, false, fmt.Errorf("extra_mounts: source is required")
	}
	// Container paths are POSIX regardless of host OS. Use path/, not filepath/,
	// so this works when sandy runs on Windows.
	if !path.IsAbs(em.Target) {
		return runtime.Mount{}, false, fmt.Errorf("extra_mounts: target %q must be an absolute container path", em.Target)
	}
	clean := path.Clean(em.Target)
	if clean == containerWorkspace || clean == containerHome {
		return runtime.Mount{}, false, fmt.Errorf("extra_mounts: target %q collides with sandy-managed mount", em.Target)
	}
	switch em.Mode {
	case "", "ro", "rw":
	default:
		return runtime.Mount{}, false, fmt.Errorf("extra_mounts: mode %q must be \"ro\" or \"rw\"", em.Mode)
	}

	host, err := resolveSource(em.Source, projectRoot)
	if err != nil {
		return runtime.Mount{}, false, err
	}
	if _, err := os.Stat(host); err != nil {
		if em.Optional {
			return runtime.Mount{}, false, nil
		}
		return runtime.Mount{}, false, fmt.Errorf("extra_mounts: required source path missing: %s", host)
	}

	return runtime.Mount{
		Source:   host,
		Target:   clean,
		ReadOnly: em.Mode != "rw",
	}, true, nil
}

// resolveSource expands a user-supplied host path: ~ (or ~/...) to $HOME,
// relative paths against projectRoot. Returns an absolute, cleaned path.
// The ~user form is not supported.
func resolveSource(src, projectRoot string) (string, error) {
	if strings.HasPrefix(src, "~") {
		if src != "~" && !strings.HasPrefix(src, "~/") && !strings.HasPrefix(src, `~\`) {
			return "", fmt.Errorf("extra_mounts: ~user form is not supported in %q (use ~ or ~/path)", src)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("extra_mounts: cannot expand %q: %w", src, err)
		}
		rest := strings.TrimLeft(strings.TrimPrefix(src, "~"), `/\`)
		src = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(src) {
		if projectRoot == "" {
			return "", fmt.Errorf("extra_mounts: relative source %q requires a project root", src)
		}
		src = filepath.Join(projectRoot, src)
	}
	return filepath.Clean(src), nil
}
