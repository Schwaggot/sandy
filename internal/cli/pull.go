package cli

import (
	"fmt"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/spf13/cobra"
)

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull [agent]",
		Short: "Pull the image(s) needed for an agent (or all agents if omitted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := project.Cwd()
			root := project.Root(cwd)
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			rt, err := runtime.Select(cfg.Runtime)
			if err != nil {
				return err
			}
			agents, _, err := agent.LoadAll()
			if err != nil {
				return err
			}
			var targets []agent.Manifest
			if len(args) == 1 {
				m, ok := agents[args[0]]
				if !ok {
					return fmt.Errorf("unknown agent %q", args[0])
				}
				targets = []agent.Manifest{m}
			} else {
				for _, m := range agents {
					targets = append(targets, m)
				}
			}
			for _, m := range targets {
				img := m.ResolveImage(cfg.ImageRegistry, cfg.Toolchain)
				fmt.Printf("pulling %s\n", img)
				if err := rt.Pull(img); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
