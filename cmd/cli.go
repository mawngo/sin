package cmd

import (
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"os"
	"sin/internal/core"
)

type CLI struct {
	command *cobra.Command
}

// NewCLI create new CLI instance and setup application core.
func NewCLI(app *core.App) *CLI {
	flags := core.AppInitConfig{
		Name: "backup",
	}
	command := cobra.Command{
		Use:   "sin",
		Short: "Backup tools",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			err := app.Init(flags)
			if err != nil {
				pterm.Error.Printf("Error initializing: %s\n", err)
				app.MustClose()
				os.Exit(1)
			}
		},
	}

	command.PersistentFlags().SortFlags = false
	command.PersistentFlags().StringVarP(&flags.ConfigFile, "config", "c", flags.ConfigFile, "specify config file")
	command.PersistentFlags().StringVar(&flags.Name, "name", flags.Name, "name of output backup and log file")
	command.PersistentFlags().BoolVar(&flags.FailFast, "ff", flags.FailFast, "enable fail-fast mode")
	command.PersistentFlags().BoolVar(&flags.AutomaticEnv, "env", flags.AutomaticEnv, "(experimental) enable automatic environment binding")

	command.AddCommand(NewMongoCmd(app))
	command.AddCommand(NewFileCmd(app))
	command.AddCommand(NewPGCmd(app))
	command.AddCommand(NewPullCmd(app))
	return &CLI{
		command: &command,
	}
}

func (cli *CLI) Execute() {
	if err := cli.command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}
