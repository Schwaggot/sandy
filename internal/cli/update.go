package cli

import (
	"fmt"
	"os"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/schwaggot/sandy/internal/selfupdate"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [agent]",
		Short: "Update the sandy binary and pull the latest image(s)",
		Long: `Update the sandy binary to the latest GitHub release, then pull
the latest sandbox images. This fetches the base image, toolchain image,
and all agent images (or just the specified agent's image when an
argument is given).

This is the recommended way to update your sandbox images after the CI
workflow pushes new images to the registry.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// A failed binary update must not block the image refresh;
			// warn, carry on, and surface the error in the exit code.
			var binErr error
			if err := selfupdate.New().Run(Version, os.Stdout); err != nil {
				binErr = fmt.Errorf("binary self-update failed: %w", err)
				fmt.Fprintf(os.Stderr, "sandy: warning: %v\n", binErr)
			}

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

			// Always pull base + toolchain.
			fmt.Printf("pulling %s/sandy-base:latest\n", cfg.ImageRegistry)
			if err := rt.Pull(fmt.Sprintf("%s/sandy-base:latest", cfg.ImageRegistry)); err != nil {
				return err
			}

			fmt.Printf("pulling %s/sandy-toolchain-%s:latest\n", cfg.ImageRegistry, cfg.Toolchain)
			if err := rt.Pull(fmt.Sprintf("%s/sandy-toolchain-%s:latest", cfg.ImageRegistry, cfg.Toolchain)); err != nil {
				return err
			}

			// Pull agent images.
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

			return binErr
		},
	}
}
