package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/AndrewPBerg/wtf/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return cli.Execute(ctx, os.Args[1:])
}
