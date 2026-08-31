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
sandy-base                       (debian:trixie-slim + sandy user + entrypoint)
  -> sandy-toolchain-fullstack   (Python 3.13 + uv, C++/LLVM, Node + npm/pnpm)
       -> sandy-{pi,opencode,claude,qwen}-fullstack    (final agent layer)
```

Only `fullstack` is currently published. Default registry `ghcr.io/schwaggot`; overridable in config. Agent manifest's `image:` field templates `{{registry}}` and `{{toolchain}}` (always resolves to `fullstack` for now).

## Project-specific invariants

- **Never copy a passthrough env value into `RunSpec.Env`** (map of explicit `KEY=VALUE`). Use `RunSpec.EnvPassthrough` (list of names) so docker reads the value from the parent process and the value never appears on the command line, dry-run output, or process listings. `sandbox.Build` does this and additionally always passes `TERM`/`COLORTERM`; tests in `internal/sandbox/sandbox_test.go` and `internal/runtime/docker_test.go` guard the behavior.
- **`restricted` network profile must error**, not silently fall through to `open`. v2 will add the proxy sidecar.
- **Linux UID matching only**: `sandbox.Build` sets `RunSpec.User` only when `goruntime.GOOS == "linux"`. On macOS/Windows Docker Desktop the FUSE layer handles ownership.
- **`/home/sandy` and its XDG dirs (`.cache`, `.config`, `.local/{bin,share,state}`) are `chmod 1777` in the base image** so the named home volume works under any `--user UID:GID`; the entrypoint runs unprivileged and can only `mkdir`, never `chown`. `HOME` is `/home/sandy` image-wide, so every `USER root` section in the toolchain and agent Dockerfiles sets `ENV HOME=/root` (keeping npm/pip caches and CLI state out of the runtime home and out of every seeded volume), resets `ENV HOME=/home/sandy`, then ends with `chown -R sandy:sandy /home/sandy && chmod 1777 /home/sandy` as a backstop. A root-owned `~/.local` makes the entrypoint's `mkdir` fail with `Permission denied` at startup, and a fixed image cannot repair a home volume already seeded from a broken one - the volume has to be removed.
- **`/tmp` tmpfs has `exec`**: `RunSpec.Tmpfs` mounts `/tmp` with `exec,nosuid,nodev`. Docker applies `noexec` to `--tmpfs` if omitted; Bun-based agents (opencode) extract a native `.so` to `/tmp` and `dlopen` it, which fails silently without `exec`.
- **Agent config mounts are `rw`**, not `ro`. Pi, claude, and opencode all write sessions/locks/settings under their config dir; a read-only mount blocks startup. The container boundary is the sandbox (caps, network, FS), not the agent's relation to its own config.
- **`hardening.read_only_workspace` mounts the project read-only**: profile flag that flips the `/workspace` bind to `ReadOnly`. Per-path overrides work because docker applies each mount independently, so a `mode: rw` entry in `extra_mounts` under `/workspace/...` remains writable. Bundled profiles default it to false; users opt in via their own profile.
- **Inference endpoints are per-agent in user/project config**: `agents.<name>.endpoints: [{protocol, url, add_host, provider, prefer}]`. `protocol` is `openai` or `anthropic`; sandy translates each entry into env vars (`OPENAI_BASE_URL`/`ANTHROPIC_BASE_URL`) and adds `OPENAI_API_KEY`/`ANTHROPIC_API_KEY` to env passthrough. `add_host` parses the URL hostname and injects `--add-host`. Per agent, at most one endpoint per protocol after merge; project entries replace user entries by `(agent, protocol)`. ANTHROPIC_BASE_URL is omitted when the URL is the default `https://api.anthropic.com` to keep claude on the standard cloud client path. No CLI override — edit YAML.
- **No model id is ever stored**: sandy resolves the model at launch from the endpoint's `/models` listing (`internal/inference`), then wires it in via the manifest's `model:` block (flag + optional env templates). Endpoint config carries only `provider` (agent-side provider id) and `prefer` (glob tie-breaker when several models are served). Discovery failure is a warning, never an error - the agent starts and uses its own default. opencode rejects models missing from its config, so its manifest also injects `OPENCODE_CONFIG_CONTENT`, which opencode merges over `opencode.json`.
- **Qwen Code must be told its protocol, not just its model**: qwen resolves the auth type as `--auth-type` > `security.auth.selectedType` in `~/.qwen/settings.json` > env (`OPENAI_API_KEY` + `OPENAI_BASE_URL` + `OPENAI_MODEL`, all three required). Since sandy bind-mounts the host `~/.qwen`, a cached cloud login there would silently outrank the endpoint config, so the qwen manifest injects `--auth-type {{protocol}}` via the `model.args` list. `model.env` also sets `OPENAI_MODEL`, which argv cannot reach: qwen forwards the environment, not its flags, to the daemon and subagents it spawns. The mount is `optional: true` - a host that has never run qwen still gets a working sandbox. `model.args` is injected under the same gate as the model flag (discovery resolved a model, user did not pin one), so a failed lookup or an explicit `-m` leaves the host's cached auth in charge.
- **`extra_hosts` adds `--add-host` entries**: user/project `config.yaml` can declare `extra_hosts: { name: ip }`. Project entries override user entries on key collision; `host.docker.internal` is reserved (sandy always sets it to `host-gateway`). Needed because container DNS does not see host mDNS, so bare LAN names like `halo` won't resolve without an explicit entry.
- **`extra_mounts` are bind mounts, default read-only**: user/project `config.yaml` can declare extra host paths under `extra_mounts`. `sandbox.resolveExtraMount` expands `~`, resolves relative sources against the project root, rejects `/workspace` and `/home/sandy` as targets, and treats missing sources as a fatal error unless `optional: true`. Lists from user and project config concatenate (like `allowlist_domains`).
- **Claude needs both `~/.claude` and `~/.claude.json`**: the JSON file at the home root holds onboarding/theme/userID; mounting only the directory makes Claude Code re-prompt the setup wizard. The manifest mounts the file as `optional: true` so Linux installs without it still work.
- **Tests live next to code** (`*_test.go`); CI runs `go mod tidy` and fails on diff.

## Asset/agent extension

Adding a bundled agent or profile: drop `internal/assets/{agents,profiles}/<name>.yaml`. `embed.FS` picks it up at build time. For agents, add a `images/agent-<name>/Dockerfile` and matrix entry in `.github/workflows/images.yml` if it should be published.
