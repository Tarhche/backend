//go:build linux

package main

import (
	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/presentation/commands/workload/vmhost"
)

// registerVMHost registers vmhost, which only runs on a linux host with KVM:
// what it does is boot microVMs, and plug them into the host's networks.
func registerVMHost(c *console.Console) {
	c.Register(vmhost.NewServeCommand())
}
