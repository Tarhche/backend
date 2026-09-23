//go:build linux

// Command runner-guest is the init of every microVM the runner starts.
//
// It is kept apart from the application's own binary on purpose: it is
// unpacked into every machine's memory before anything else runs there, so
// what it carries is what every machine pays for. It carries its agent and
// nothing else.
package main

import (
	"context"
	"os"
	"os/signal"
	"path"

	"github.com/danceable/console"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/presentation/commands/runner/guest"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	c := console.NewConsole(
		path.Base(os.Args[0]),
		"A microVM's init.",
		os.Stdout,
		os.Stderr,
		provider.Default,
	)

	c.Register(guest.NewServeCommand())

	code := c.Run(ctx, os.Args)

	cancel()
	os.Exit(code)
}
