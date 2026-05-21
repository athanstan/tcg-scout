package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"tcg-scout/internal/app"
	"tcg-scout/internal/cli"
	svecards "tcg-scout/internal/tcg/sve/cards"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service, err := app.NewService(svecards.NewRunner())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	command := cli.NewRootCommand(service, cli.Streams{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}, cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	command.SetArgs(cli.RewriteArgs(os.Args[1:]))
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(cli.ExitCode(err))
	}
}
