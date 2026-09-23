//go:build !linux

package main

import (
	"github.com/danceable/console"
)

// registerLauncher registers nothing: the launcher only runs on a linux host.
func registerLauncher(c *console.Console) {}
