package cmd

import (
	"fmt"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"os"
	"sin/internal/core"
)

type CLI struct {
	command *cobra.Command
}

// NewCLI create new CLI instance and setup application core.
func NewCLI(app *core.App) *CLI {
	command := cobra.Command{
		Use:   "sin",
		Short: "Backup tools",
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			configFile := lo.Must(cmd.Flags().GetString("config"))
			name := lo.Must(cmd.Flags().GetString("name"))
			env := lo.Must(cmd.Flags().GetBool("env"))
			ff := lo.Must(cmd.Flags().GetBool("ff"))
			err := app.Init(configFile, name, env, ff)
			if err != nil {
				pterm.Error.Printf("Error initializing: %s\n", err)
				app.MustClose()
				os.Exit(1)
			}
		},
	}

	command.PersistentFlags().SortFlags = false
	command.PersistentFlags().StringP("config", "c", "", "specify config file")
	command.PersistentFlags().String("name", "backup", "name of output backup and log file")
	command.PersistentFlags().Bool("env", false, "enable automatic environment binding")
	command.PersistentFlags().Bool("ff", false, "enable fail-fast mode")

	command.AddCommand(NewMongoCmd(app))
	command.AddCommand(NewFileCmd(app))
	return &CLI{
		command: &command,
	}
}

func (cli *CLI) Execute() {
	if err := cli.command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}
