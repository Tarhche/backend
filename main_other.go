//go:build !linux

package main

import (
	"github.com/danceable/console"
)

// registerVMHost registers nothing: vmhost only runs on a linux host with KVM.
func registerVMHost(c *console.Console) {}
