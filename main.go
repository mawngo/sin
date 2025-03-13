package main

import (
	"github.com/pterm/pterm"
	"sin/cmd"
	"sin/internal/core"
)

func main() {
	app := &core.App{}
	defer func() {
		if err := app.Close(); err != nil {
			pterm.Error.Println(err)
		}
	}()
	cli := cmd.NewCLI(app)
	cli.Execute()
}
