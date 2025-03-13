package main

import (
	"sin/cmd"
)

func main() {
	cli := cmd.NewCLI()
	cli.Execute()
}
