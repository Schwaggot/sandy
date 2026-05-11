package cli

import (
	"fmt"
	"sort"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available agents or profiles",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "agents",
		Short: "List available agents",
		RunE: func(c *cobra.Command, args []string) error {
			agents, _, err := agent.LoadAll()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(agents))
			for n := range agents {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Printf("%-12s %s\n", n, agents[n].Description)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "profiles",
		Short: "List available profiles",
		RunE: func(c *cobra.Command, args []string) error {
			profs, _, err := profile.LoadAll()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(profs))
			for n := range profs {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				p := profs[n]
				fmt.Printf("%-12s network=%s memory=%s cpus=%s\n", n, p.Network, p.Resources.Memory, p.Resources.CPUs)
			}
			return nil
		},
	})
	return cmd
}
