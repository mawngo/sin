package cmd

import (
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"sin/internal/core"
	"sin/internal/store"
)

func NewListCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "list <adapter...?>",
		Args:  cobra.MinimumNArgs(0),
		Short: "List remote backup files",
		Run: func(cmd *cobra.Command, args []string) {
			syncher, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				return
			}

			extension := lo.Must(cmd.Flags().GetString("ext"))
			destFileName := app.Name
			if extension == "*" {
				destFileName += "(.\\w+)?"
			} else if extension == "+" {
				destFileName += ".\\w+"
			} else if extension != "" {
				destFileName += "." + extension
			}
			destFileName += core.BackupFileExt

			err = syncher.List(app.Ctx, destFileName, args...)
			if err != nil {
				pterm.Error.Println(err)
			}
		},
	}
	command.Flags().StringP("ext", "e", "*", "specify the extension of target file (without dot)")
	return &command
}
