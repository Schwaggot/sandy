package cli

import (
	"fmt"

	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/spf13/cobra"
)

func newCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove cache and home volumes for this project",
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
			rt, err := runtime.Select(cfg.Runtime)
			if err != nil {
				return err
			}
			hash := project.Hash(root)
			prefixes := []string{
				"sandy-home-" + hash,
				"sandy-cache-" + hash,
			}
			for _, prefix := range prefixes {
				vols, err := rt.VolumeList(prefix)
				if err != nil {
					return err
				}
				for _, v := range vols {
					if err := rt.VolumeRemove(v); err != nil {
						return err
					}
					fmt.Printf("removed %s\n", v)
				}
			}
			return nil
		},
	}
}
