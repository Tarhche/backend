//go:build linux

package main

import (
	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/presentation/commands/runner/launcher"
)

// registerLauncher registers the launcher, which only runs on a linux host:
// what it does is start microVMs, and plug them into the host's network.
func registerLauncher(c *console.Console) {
	c.Register(launcher.NewServeCommand())
}
