package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"sin/internal/core"
)

func NewMongoCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "mongo <core>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for mongo",
		Run: func(cmd *cobra.Command, args []string) {
			pterm.Info.Println(app.Name)
		},
	}

	return &command
}
