package runtime

import (
	"strings"
	"testing"
)

func TestBuildArgsOrderAndFlags(t *testing.T) {
	d := NewDocker()
	spec := RunSpec{
		Image:          "img:tag",
		Command:        []string{"agent"},
		Args:           []string{"--x", "y"},
		WorkDir:        "/workspace",
		Env:            map[string]string{"B": "2", "A": "1"},
		EnvPassthrough: []string{"SECRET_TOKEN"},
		Mounts:       []Mount{{Source: "/src", Target: "/dst"}, {Source: "vol", Target: "/v", Volume: true, ReadOnly: true}},
		Network:      "none",
		AddHosts:     map[string]string{"host.docker.internal": "host-gateway"},
		User:         "1000:1000",
		ReadOnly:     true,
		Tmpfs:        []string{"/tmp:rw"},
		CapDrop:      []string{"ALL"},
		SecurityOpts: []string{"no-new-privileges"},
		PidsLimit:    1024,
		Memory:       "4g",
		CPUs:         "2",
		Interactive:  true,
		TTY:          true,
		AutoRemove:   true,
	}

	args := d.BuildArgs(spec)
	joined := strings.Join(args, " ")

	wantSubs := []string{
		"run",
		"--rm",
		"-i",
		"-t",
		"-w /workspace",
		"--user 1000:1000",
		"--network none",
		"--add-host host.docker.internal:host-gateway",
		"-e A=1",
		"-e B=2",
		"-e SECRET_TOKEN",
		"--mount type=bind,src=/src,dst=/dst",
		"--mount type=volume,src=vol,dst=/v,readonly",
		"--tmpfs /tmp:rw",
		"--read-only",
		"--cap-drop ALL",
		"--security-opt no-new-privileges",
		"--pids-limit 1024",
		"--memory 4g",
		"--cpus 2",
		"img:tag agent --x y",
	}
	for _, want := range wantSubs {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in: %s", want, joined)
		}
	}

	// Critical: passthrough must NOT be rendered as `-e KEY=value`.
	if strings.Contains(joined, "SECRET_TOKEN=") {
		t.Errorf("passthrough variable value leaked into command line: %s", joined)
	}

	// Env keys must be sorted (A before B).
	idxA := strings.Index(joined, "-e A=1")
	idxB := strings.Index(joined, "-e B=2")
	if idxA < 0 || idxB < 0 || idxA > idxB {
		t.Errorf("env keys not sorted: %s", joined)
	}

	// Image must appear before command and args.
	idxImg := strings.Index(joined, "img:tag")
	idxCmd := strings.Index(joined, "agent")
	if idxImg < 0 || idxCmd < 0 || idxImg > idxCmd {
		t.Errorf("image must precede command: %s", joined)
	}
}

func TestBuildArgsMinimal(t *testing.T) {
	d := NewDocker()
	args := d.BuildArgs(RunSpec{Image: "x"})
	if args[0] != "run" || args[len(args)-1] != "x" {
		t.Errorf("unexpected minimal args: %v", args)
	}
}

func TestSelectUnknown(t *testing.T) {
	if _, err := Select("nope"); err == nil {
		t.Error("expected error for unknown runtime")
	}
}

func TestSelectPodmanNotYet(t *testing.T) {
	if _, err := Select("podman"); err == nil {
		t.Error("podman should return not-yet-implemented error in v1")
	}
}
