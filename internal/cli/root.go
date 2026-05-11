package cli

import (
	"fmt"
	"os"

	"github.com/schwaggot/sandy/internal/agent"
	"github.com/spf13/cobra"
)

var (
	flagProfile string
	flagRuntime string
	flagDryRun  bool
)

func Execute() error {
	root := newRootCmd()
	return root.Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "sandy",
		Short:         "Run AI coding agents in sandboxed containers",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flagProfile, "profile", "", "profile name (overrides config)")
	root.PersistentFlags().StringVar(&flagRuntime, "runtime", "", "container runtime (docker|podman)")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "print the resolved run command without executing")

	// Dynamic agent subcommands.
	agents, warnings, err := agent.LoadAll()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sandy: failed to load agents:", err)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "sandy: warning:", w)
	}
	for name, m := range agents {
		root.AddCommand(newAgentCmd(name, m))
	}

	root.AddCommand(
		newInitCmd(),
		newListCmd(),
		newDoctorCmd(),
		newCleanCmd(),
		newPullCmd(),
		newBuildCmd(),
		newVersionCmd(),
	)

	return root
}
