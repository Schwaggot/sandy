package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/inference"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/schwaggot/sandy/internal/sandbox"
	"github.com/spf13/cobra"
)

func newAgentCmd(name string, m agent.Manifest) *cobra.Command {
	cmd := &cobra.Command{
		Use:                name + " [-- args...]",
		Short:              m.Description,
		DisableFlagParsing: false,
		RunE: func(_ *cobra.Command, args []string) error {
			return runAgent(m, args)
		},
	}
	return cmd
}

func runAgent(m agent.Manifest, args []string) error {
	cwd, err := project.Cwd()
	if err != nil {
		return err
	}
	root := project.Root(cwd)

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	if flagProfile != "" {
		cfg.Profile = flagProfile
	}
	if flagRuntime != "" {
		cfg.Runtime = flagRuntime
	}

	prof, err := profile.Get(cfg.Profile)
	if err != nil {
		return err
	}
	// Append config-level allowlist additions onto the profile.
	prof.AllowlistDomains = append(prof.AllowlistDomains, cfg.AllowlistDomains...)

	rt, err := runtime.Select(cfg.Runtime)
	if err != nil {
		return err
	}

	models := resolveModels(m, cfg.Agents[m.Name].Endpoints, root)

	spec, err := sandbox.Build(cfg, m, prof, root, args, models)
	if err != nil {
		return err
	}

	if flagDryRun {
		fmt.Println(rt.Name(), strings.Join(rt.BuildArgs(spec), " "))
		return nil
	}

	if err := rt.Available(); err != nil {
		return err
	}
	return rt.Run(spec)
}

// resolveModels asks every configured endpoint which models it currently
// serves and picks one model per endpoint. Sandy stores no model id, so this
// is the only place a model name enters the run.
//
// Best-effort by design: a lookup failure is a warning, not an error, and the
// agent starts on whatever its own config defaults to.
func resolveModels(m agent.Manifest, endpoints []config.Endpoint, projectRoot string) []inference.Selection {
	if m.Model == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), inference.DefaultTimeout)
	defer cancel()

	var out []inference.Selection
	for _, ep := range endpoints {
		url := strings.TrimSpace(ep.URL)
		// No URL, or the stock cloud: nothing sandy should second-guess.
		if url == "" || url == config.AnthropicCloudURL {
			continue
		}
		// Skip discovery on a broken ca_cert without warning: sandbox.Build
		// re-checks it and aborts the run, so warning here would only print
		// the same cause twice - and "agent picks its own model" would be a
		// lie, since no agent starts at all.
		caCert, err := resolveCACert(ep, projectRoot)
		if err != nil {
			continue
		}
		served, err := inference.List(ctx, inference.Target{
			Protocol: ep.Protocol,
			BaseURL:  url,
			APIKey:   endpointAPIKey(ep),
			AddHost:  ep.AddHost,
			CACert:   caCert,
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "sandy: warning: %s endpoint %s: %v (agent picks its own model)\n", ep.Protocol, url, err)
			continue
		}
		sel, ok := inference.Select(served, ep.Prefer)
		if !ok {
			continue
		}
		out = append(out, inference.Selection{Model: sel, BaseURL: url, Provider: ep.Provider, Protocol: ep.Protocol})
	}
	return out
}

// resolveCACert expands an endpoint's ca_cert for host-side use. Empty stays
// empty: the lookup then runs against the system roots.
func resolveCACert(ep config.Endpoint, projectRoot string) (string, error) {
	if strings.TrimSpace(ep.CACert) == "" {
		return "", nil
	}
	return config.ResolveCACert(ep.CACert, projectRoot)
}

// endpointAPIKey is the credential used for the host-side lookup. A no_auth
// endpoint gets none: the same reasoning as in the container, plus an
// unauthenticated server has no reason to see the host's cloud key.
func endpointAPIKey(ep config.Endpoint) string {
	if ep.NoAuth {
		return ""
	}
	return os.Getenv(apiKeyEnv(ep.Protocol))
}

func apiKeyEnv(protocol string) string {
	if protocol == "anthropic" {
		return "ANTHROPIC_API_KEY"
	}
	return "OPENAI_API_KEY"
}
