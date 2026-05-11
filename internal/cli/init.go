package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/schwaggot/sandy/internal/config"
	"github.com/schwaggot/sandy/internal/project"
	"github.com/spf13/cobra"
)

var (
	toolchainChoices = []string{"python", "cpp", "node", "fullstack"}
	profileChoices   = []string{"open", "offline"}
)

func newInitCmd() *cobra.Command {
	var (
		force          bool
		nonInteractive bool
	)
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
			defaultToolchain := project.PickToolchain(detected)

			if len(detected) == 0 {
				fmt.Println("No language markers detected in this directory.")
			} else {
				fmt.Printf("Detected toolchains: %s\n", strings.Join(detected, ", "))
			}

			in := bufio.NewReader(os.Stdin)
			toolchain := defaultToolchain
			prof := config.DefaultProfile
			if !nonInteractive {
				toolchain, err = promptChoice(in, "Toolchain", toolchainChoices, defaultToolchain)
				if err != nil {
					return err
				}
				prof, err = promptChoice(in, "Profile", profileChoices, config.DefaultProfile)
				if err != nil {
					return err
				}
			}

			cfg := config.Config{
				Profile:       prof,
				Toolchain:     toolchain,
				ImageRegistry: config.DefaultRegistry,
				Runtime:       "docker",
			}
			if err := config.Write(cwd, cfg); err != nil {
				return err
			}
			fmt.Printf("Wrote %s (toolchain=%s profile=%s)\n", target, toolchain, prof)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .sandy/config.yaml")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "accept defaults without prompting")
	return cmd
}

// promptChoice reads one line from r and validates it against choices.
// Empty input returns def. EOF (e.g. non-interactive pipe) also returns def.
func promptChoice(r *bufio.Reader, label string, choices []string, def string) (string, error) {
	fmt.Printf("%s [%s] (default: %s): ", label, strings.Join(choices, "/"), def)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	for _, c := range choices {
		if c == line {
			return line, nil
		}
	}
	return "", fmt.Errorf("invalid choice %q; must be one of: %s", line, strings.Join(choices, ", "))
}
