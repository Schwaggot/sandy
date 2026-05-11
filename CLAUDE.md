# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Sandy is a Go CLI that runs AI coding agents (pi, opencode, claude) in sandboxed Docker containers, using each agent's existing host-side config. Full design in `docs/SPECIFICATION.md`.

## Common commands

```
go build ./...                                   # type-check everything
go build -o sandy ./cmd/sandy                    # produce binary
go test ./...                                    # run all tests
go test -run TestBuildBasic ./internal/sandbox/  # run one test
go vet ./...
gofmt -w .
go mod tidy                                      # after changing imports; CI checks for diff
./sandy --dry-run <agent>                        # print resolved docker invocation
```

Module path: `github.com/schwaggot/sandy`. Go 1.26.

## Architecture

Run-path data flow (everything below `internal/` is private):

```
cmd/sandy/main.go
  -> internal/cli           (cobra; registers one subcommand per agent manifest)
       runAgent:
         config.Load(projectRoot)         # built-in < ~/.sandy/ < .sandy/, lists for allowlist_domains append
         profile.Get(cfg.Profile)         # bundled YAML + ~/.sandy/profiles/
         runtime.Select(cfg.Runtime)      # docker today, podman stub
         sandbox.Build(...)               # -> runtime.RunSpec
         runtime.Run(spec)                # shells out to docker
```

Agent manifests and profiles ship as `internal/assets/{agents,profiles}/*.yaml`, loaded via `//go:embed` in `internal/assets/assets.go`. User files in `~/.sandy/{agents,profiles}/` override bundled ones by `name`. Both `LoadAll()` functions return `(map, []error warnings, error)` - bundled errors are fatal, user-file errors are non-fatal warnings.

Image hierarchy (built and pushed by `.github/workflows/images.yml`):

```
sandy-base                              (debian:trixie-slim + sandy user + entrypoint)
  -> sandy-toolchain-{python,cpp,node,fullstack}    (BASE_IMAGE build arg)
       -> sandy-{pi,opencode,claude}-{toolchain}    (final agent layer)
```

Default registry `ghcr.io/schwaggot`; overridable in config. Agent manifest's `image:` field templates `{{registry}}` and `{{toolchain}}`.

## Project-specific invariants

- **Never copy a passthrough env value into `RunSpec.Env`** (map of explicit `KEY=VALUE`). Use `RunSpec.EnvPassthrough` (list of names) so docker reads the value from the parent process and the value never appears on the command line, dry-run output, or process listings. The `sandbox.Build` function does this; tests in `internal/sandbox/sandbox_test.go` and `internal/runtime/docker_test.go` guard the behavior.
- **`restricted` network profile must error**, not silently fall through to `open`. v2 will add the proxy sidecar.
- **Linux UID matching only**: `sandbox.Build` sets `RunSpec.User` only when `goruntime.GOOS == "linux"`. On macOS/Windows Docker Desktop the FUSE layer handles ownership.
- **`/home/sandy` is `chmod 1777` in the base image** so the named home volume works under any `--user UID:GID`. Don't change this without replacing it with a proper fixuid/gosu pattern.
- **Tests live next to code** (`*_test.go`); CI runs `go mod tidy` and fails on diff.

## Asset/agent extension

Adding a bundled agent or profile: drop `internal/assets/{agents,profiles}/<name>.yaml`. `embed.FS` picks it up at build time. For agents, add a `images/agent-<name>/Dockerfile` and matrix entry in `.github/workflows/images.yml` if it should be published.
