package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/schwaggot/sandy/internal/runtime"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check sandy environment health",
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
			if err := rt.Available(); err != nil {
				fmt.Printf("[fail] runtime %q: %v\n", rt.Name(), err)
			} else {
				fmt.Printf("[ ok ] runtime %q\n", rt.Name())
			}

			agents, warnings, err := agent.LoadAll()
			if err != nil {
				fmt.Printf("[fail] agents: %v\n", err)
			} else {
				fmt.Printf("[ ok ] %d agent manifests\n", len(agents))
			}
			for _, w := range warnings {
				fmt.Printf("[warn] %v\n", w)
			}

			home, _ := os.UserHomeDir()
			userCfg := filepath.Join(home, ".sandy", "config.yaml")
			if _, err := os.Stat(userCfg); err == nil {
				fmt.Printf("[ ok ] user config: %s\n", userCfg)
			} else {
				fmt.Printf("[info] no user config at %s\n", userCfg)
			}

			projCfg := filepath.Join(root, ".sandy", "config.yaml")
			if _, err := os.Stat(projCfg); err == nil {
				fmt.Printf("[ ok ] project config: %s\n", projCfg)
			} else {
				fmt.Printf("[info] no project config at %s (run `sandy init`)\n", projCfg)
			}

			fmt.Printf("Effective: profile=%s toolchain=%s registry=%s\n", cfg.Profile, cfg.Toolchain, cfg.ImageRegistry)
			return nil
		},
	}
}
