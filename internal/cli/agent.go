package cli

import (
	"fmt"
	"strings"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
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
		RunE: func(c *cobra.Command, args []string) error {
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

	spec, err := sandbox.Build(cfg, m, prof, root, args)
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
