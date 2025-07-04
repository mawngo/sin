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
	cobra.EnableCommandSorting = false
	flags := core.AppInitConfig{}

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
	command.Flags().SortFlags = false
	command.PersistentFlags().StringVarP(&flags.ConfigFile, "config", "c", flags.ConfigFile, "specify config file")
	command.PersistentFlags().StringVar(&flags.Name, "name", flags.Name, "name of output backup and log file")
	command.PersistentFlags().BoolVar(&flags.EnableFailFast, "ff", flags.EnableFailFast, "enable fail-fast mode")
	command.PersistentFlags().IntVar(&flags.Keep, "keep", flags.Keep, "number of local backups to keep")
	command.PersistentFlags().BoolVar(&flags.EnableAutomaticEnv, "env", flags.EnableAutomaticEnv, "(experimental) enable automatic environment binding")
	command.PersistentFlags().BoolVar(&flags.EnableLocalMode, "local", flags.EnableLocalMode, "(local mode) create backup in current directory without syncing")
	command.PersistentFlags().BoolVar(&flags.NoMkdir, "no-mkdir", flags.NoMkdir, "does not create local backup directory if it not exist")

	command.AddCommand(NewListCmd(app))
	command.AddCommand(NewPullCmd(app))

	command.AddCommand(NewFileCmd(app))
	command.AddCommand(NewMongoCmd(app))
	command.AddCommand(NewPGCmd(app))
	return &CLI{
		command: &command,
	}
}

func (cli *CLI) Execute() {
	if err := cli.command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}
