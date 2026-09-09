package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print sandy version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(Version)
		},
	}
}
