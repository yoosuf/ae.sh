// Command ae is the local-first Agentic Engineering Framework CLI.
package main

import (
	"context"
	"os"

	"github.com/yoosuf/ae.sh/internal/cli"
)

func main() {
	app := &cli.App{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Args:   os.Args[1:],
	}
	os.Exit(app.Run(context.Background()))
}
