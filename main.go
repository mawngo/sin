package main

import (
	"os"
	"os/signal"
	"sin/cmd"
	"sin/internal/core"
	"syscall"
)

func main() {
	app := &core.App{}
	defer app.MustClose()

	// Handle ctrl+c.
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-sigs
		app.MustClose()
		os.Exit(1)
	}()

	cli := cmd.NewCLI(app)
	cli.Execute()
}
