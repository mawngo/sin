package cmd

import (
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
)

type CLI struct {
	command *cobra.Command
}

// NewCLI create new CLI instance and setup application config.
func NewCLI() *CLI {
	command := cobra.Command{
		Use:   "sin",
		Short: "Database backup tools.",
		Run: func(_ *cobra.Command, _ []string) {
			pterm.Println("Hello World!")
		},
	}
	return &CLI{
		command: &command,
	}
}

func (cli *CLI) Execute() {
	if err := cli.command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}
