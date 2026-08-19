package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/inference"
	"github.com/schwaggot/sandy/internal/profile"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available agents, profiles, or endpoints",
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
	cmd.AddCommand(&cobra.Command{
		Use:   "endpoints",
		Short: "List configured inference endpoints (merged user + project)",
		RunE: func(c *cobra.Command, args []string) error {
			cwd, _ := project.Cwd()
			cfg, err := config.Load(cwd)
			if err != nil {
				return err
			}
			if len(cfg.Agents) == 0 {
				fmt.Println("(no endpoints configured; see SPECIFICATION.md for the schema)")
				return nil
			}
			names := make([]string, 0, len(cfg.Agents))
			for n := range cfg.Agents {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				eps := cfg.Agents[n].Endpoints
				if len(eps) == 0 {
					continue
				}
				fmt.Printf("%s:\n", n)
				for _, ep := range eps {
					url := ep.URL
					if url == "" {
						url = "(default)"
					}
					addHost := ""
					if ep.AddHost != "" {
						addHost = fmt.Sprintf("  add_host=%s", ep.AddHost)
					}
					fmt.Printf("  %-10s %s%s\n", ep.Protocol, url, addHost)
					fmt.Printf("             %s\n", servedBy(ep))
				}
			}
			return nil
		},
	})
	return cmd
}

// servedBy reports what the endpoint currently serves, which is the only place
// a model id exists - sandy stores none. Probing is best-effort: an endpoint
// that is down still lists, it just cannot say what it holds.
func servedBy(ep config.Endpoint) string {
	if strings.TrimSpace(ep.URL) == "" || ep.URL == config.AnthropicCloudURL {
		return "serves: (cloud default; sandy does not pin a model)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), inference.DefaultTimeout)
	defer cancel()

	models, err := inference.List(ctx, ep.Protocol, ep.URL, os.Getenv(apiKeyEnv(ep.Protocol)), ep.AddHost)
	if err != nil {
		return fmt.Sprintf("serves: (unreachable: %v)", err)
	}
	sel, _ := inference.Select(models, ep.Prefer)
	out := "serves: " + sel.ID
	if sel.Context > 0 {
		out += fmt.Sprintf(" (ctx %d)", sel.Context)
	}
	if len(models) > 1 {
		others := make([]string, 0, len(models)-1)
		for _, m := range models {
			if m.ID != sel.ID {
				others = append(others, m.ID)
			}
		}
		out += "  also available: " + strings.Join(others, ", ")
	}
	return out
}
