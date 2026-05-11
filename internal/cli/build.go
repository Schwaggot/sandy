package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/spf13/cobra"
)

func newBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "build",
		Short: "Build the project-local image from .sandy/Dockerfile",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, err := project.Cwd()
			if err != nil {
				return err
			}
			root := project.Root(cwd)
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}
			df := filepath.Join(root, ".sandy", "Dockerfile")
			if _, err := os.Stat(df); err != nil {
				return fmt.Errorf("no .sandy/Dockerfile found")
			}
			rt, err := runtime.Select(cfg.Runtime)
			if err != nil {
				return err
			}
			tag := fmt.Sprintf("sandy-local-%s:latest", project.Hash(root))
			return rt.Build(root, df, tag)
		},
	}
}
