package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"sin/internal/core"
)

func NewMongoCmd(_ *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "mongo <uri>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for mongo",
		Run: func(_ *cobra.Command, _ []string) {
			pterm.Info.Println("Not implemented yet")
		},
	}

	return &command
}
