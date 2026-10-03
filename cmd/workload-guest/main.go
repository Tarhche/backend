// Command workload-guest is the init of every microVM vmhost boots: the agent
// that puts the task's root together from the machine's disks, brings up its
// network, runs its task, and answers vmhost on a vsock port
// (domain/workload/guest).
//
// It is a binary of its own, apart from the application's, because the kernel
// unpacks it into every machine's memory before anything else runs there: what
// it carries is what every machine pays for. So it is a plain main, with no
// console, flags or container: the kernel runs /init with no arguments
// (guest.KernelArgs), and a machine is told what it is over its vsock once it
// is up. It is built static (CGO_ENABLED=0), and vmhost makes an initramfs of
// it (infrastructure/workload/vmm/initrd).
//
// It refuses to run as anything but process 1. Run by mistake on a host, it
// would mount over the host's directories and reboot it.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/guest/agent"
)

func main() {
	if os.Getpid() != 1 {
		fmt.Fprintln(os.Stderr, "workload-guest is a microVM's init: it runs as process 1 of a machine vmhost booted, and as nothing else")
		os.Exit(2)
	}

	// what the agent says goes to the machine's console, which the host
	// keeps beside the machine for its operators.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := agent.New(logger).Run(context.Background()); err != nil {
		// an agent that cannot run leaves nothing to run the machine for, so
		// the machine is turned off rather than left up with nobody in it.
		logger.Error("the agent could not run, so the machine is turned off", "error", err)
		agent.Halt()
	}

	// init ending is the kernel panicking, which ends the machine too
	// (panic=1, reboot=k): this is only reached when turning it off did not.
	os.Exit(1)
}
