// Command sandy runs AI coding agents in sandboxed containers.
package main

import (
	"fmt"
	"os"

	"github.com/schwaggot/sandy/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "sandy:", err)
		os.Exit(1)
	}
}
