package main

import (
	"fmt"
	"os"

	"codex-switcher/internal/app"
	"codex-switcher/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(app.ExitCode(err))
	}
}
