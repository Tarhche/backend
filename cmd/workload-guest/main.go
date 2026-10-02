// Command workload-guest is the init of every microVM vmhost boots: the agent
// that puts the task's root together from the machine's disks, brings up its
// network, runs its task, and answers vmhost on a vsock port
// (domain/workload/guest).
//
// It is a binary of its own, apart from the application's, because the kernel
// unpacks it into every machine's memory before anything else runs there: what
// it carries is what every machine pays for. It is built static
// (CGO_ENABLED=0), and vmhost makes an initramfs of it
// (infrastructure/workload/vmm/initrd).
package main

import (
	"fmt"
	"os"
)

// main is a placeholder until the agent is here: a machine booted with it
// ends at once, rather than staying up with nobody in it.
func main() {
	fmt.Fprintln(os.Stderr, "workload-guest: the agent is not built into this binary yet")
	os.Exit(1)
}
