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
func NewCLI() *CLI {
	app := &core.App{}

	command := cobra.Command{
		Use:   "sin <type> <core>",
		Short: "Database backup tools.",
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			configFile := lo.Must(cmd.Flags().GetString("config"))
			name := lo.Must(cmd.Flags().GetString("name"))
			env := lo.Must(cmd.Flags().GetBool("env"))
			err := app.Init(configFile, name, env)
			if err != nil {
				pterm.Error.Printf("Error initializing: %s\n", err)
				os.Exit(1)
			}
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			return app.Close()
		},
	}

	command.PersistentFlags().SortFlags = false
	command.PersistentFlags().StringP("config", "c", "", "Specify config file")
	command.PersistentFlags().String("name", "unnamed", "Name of application, affect output backup and log file name")
	command.PersistentFlags().Bool("env", false, "Enable automatic environment binding")

	command.AddCommand(NewMongoCmd(app))
	return &CLI{
		command: &command,
	}
}

func (cli *CLI) Execute() {
	if err := cli.command.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
}
