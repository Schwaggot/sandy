package runtime

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type Docker struct {
	bin string
}

func NewDocker() *Docker {
	return &Docker{bin: "docker"}
}

func (d *Docker) Name() string { return "docker" }

func (d *Docker) Available() error {
	cmd := exec.Command(d.bin, "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker not available: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *Docker) BuildArgs(spec RunSpec) []string {
	args := []string{"run"}
	if spec.AutoRemove {
		args = append(args, "--rm")
	}
	if spec.Interactive {
		args = append(args, "-i")
	}
	if spec.TTY {
		args = append(args, "-t")
	}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	if spec.WorkDir != "" {
		args = append(args, "-w", spec.WorkDir)
	}
	if spec.User != "" {
		args = append(args, "--user", spec.User)
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	for host, ip := range spec.AddHosts {
		args = append(args, "--add-host", fmt.Sprintf("%s:%s", host, ip))
	}
	for _, e := range envSorted(spec.Env) {
		args = append(args, "-e", e)
	}
	passthrough := append([]string(nil), spec.EnvPassthrough...)
	sort.Strings(passthrough)
	for _, k := range passthrough {
		args = append(args, "-e", k)
	}
	for _, m := range spec.Mounts {
		typ := "bind"
		if m.Volume {
			typ = "volume"
		}
		opt := fmt.Sprintf("type=%s,src=%s,dst=%s", typ, m.Source, m.Target)
		if m.ReadOnly {
			opt += ",readonly"
		}
		args = append(args, "--mount", opt)
	}
	for _, t := range spec.Tmpfs {
		args = append(args, "--tmpfs", t)
	}
	if spec.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, c := range spec.CapDrop {
		args = append(args, "--cap-drop", c)
	}
	for _, c := range spec.CapAdd {
		args = append(args, "--cap-add", c)
	}
	for _, s := range spec.SecurityOpts {
		args = append(args, "--security-opt", s)
	}
	if spec.PidsLimit > 0 {
		args = append(args, "--pids-limit", fmt.Sprintf("%d", spec.PidsLimit))
	}
	if spec.Memory != "" {
		args = append(args, "--memory", spec.Memory)
	}
	if spec.CPUs != "" {
		args = append(args, "--cpus", spec.CPUs)
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	args = append(args, spec.Args...)
	return args
}

func envSorted(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

func (d *Docker) Run(spec RunSpec) error {
	args := d.BuildArgs(spec)
	cmd := exec.Command(d.bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *Docker) Build(contextDir, dockerfile, tag string) error {
	args := []string{"build", "-t", tag}
	if dockerfile != "" {
		args = append(args, "-f", dockerfile)
	}
	args = append(args, contextDir)
	cmd := exec.Command(d.bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *Docker) Pull(image string) error {
	cmd := exec.Command(d.bin, "pull", image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (d *Docker) VolumeList(prefix string) ([]string, error) {
	cmd := exec.Command(d.bin, "volume", "ls", "--format", "{{.Name}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var names []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if prefix == "" || strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, sc.Err()
}

func (d *Docker) VolumeRemove(name string) error {
	cmd := exec.Command(d.bin, "volume", "rm", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
