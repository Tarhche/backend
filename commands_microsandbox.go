//go:build microsandbox

package main

import (
	"github.com/danceable/console"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload"
	"github.com/khanzadimahdi/testproject/presentation/commands/workload/microsandbox"
)

// registerOptionalCommands registers serve-workload-microsandbox, which only
// a build with the microsandbox tag has: it runs the workload's microVMs over
// microsandbox's SDK, which is cgo, and which only the workload-microsandbox
// image is built with.
func registerOptionalCommands(c *console.Console) {
	c.Register(microsandbox.NewServeCommand(workload.NewMicrosandboxSDKProvider()))
}
