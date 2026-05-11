package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Detect toolchains and write .sandy/config.yaml in the current directory",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := project.Cwd()
			if err != nil {
				return err
			}

			target := filepath.Join(cwd, ".sandy", "config.yaml")
			if _, err := os.Stat(target); err == nil && !force {
				return fmt.Errorf("%s already exists; pass --force to overwrite", target)
			}

			detected := project.DetectToolchains(cwd)
			tc := project.PickToolchain(detected)

			cfg := config.Config{
				Profile:       config.DefaultProfile,
				Toolchain:     tc,
				ImageRegistry: config.DefaultRegistry,
				Runtime:       "docker",
			}
			if err := config.Write(cwd, cfg); err != nil {
				return err
			}

			fmt.Printf("Detected toolchains: %v\n", detected)
			fmt.Printf("Selected toolchain: %s\n", tc)
			fmt.Printf("Wrote %s\n", target)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .sandy/config.yaml")
	return cmd
}
