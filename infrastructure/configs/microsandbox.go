package configs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultWorkloadMicrosandboxPort        = 8443
	defaultWorkloadMicrosandboxStateDir    = "/data/service"
	defaultWorkloadMicrosandboxMinMemory   = 64 << 20 // 64 MiB
	defaultWorkloadMicrosandboxPortRange   = "20000-29999"
	defaultWorkloadMicrosandboxBindAddress = "0.0.0.0"
	defaultWorkloadMicrosandboxNameservers = "1.1.1.1,9.9.9.9"

	// microsandboxBudgetShare is the part of the container's memory limit the
	// VMs are admitted for, in percent, and microsandboxBudgetReserve what is
	// kept back from it for the service itself and microsandbox beside it.
	microsandboxBudgetShare   = 80
	microsandboxBudgetReserve = 512 << 20 // 512 MiB
)

// WorkloadMicrosandbox holds the configuration of the
// serve-workload-microsandbox command.
//
// Microsandbox itself reads three variables of its own, which are not here:
// MSB_HOME, where it keeps its database, images and disks; and MSB_PATH and
// MSB_LIBKRUNFW_PATH, the msb and libkrunfw it runs, which the image points at
// its own pair so that nothing is downloaded. The SDK reads them as the
// OpenTelemetry SDK reads OTEL_*.
type WorkloadMicrosandbox struct {
	Port int `usage:"specifies which port server should listen to." env:"SERVER_PORT" long:"port" short:"p"`

	Authority   string `usage:"The certificate authority, in PEM form, an orchestrator's client certificate has to be signed by: the workload tunnel's. Its private key is never needed here." env:"WORKLOAD_MICROSANDBOX_CA_CERT" long:"ca-cert"`
	Certificate string `usage:"The certificate, in PEM form, this service answers with." env:"WORKLOAD_MICROSANDBOX_CERT" long:"cert"`
	Key         string `usage:"The private key, in PEM form, for that certificate." env:"WORKLOAD_MICROSANDBOX_KEY" long:"key"`

	StateDir string `usage:"Directory the runs' records and log journals are kept in. It has to outlive the container, as MSB_HOME does." env:"WORKLOAD_MICROSANDBOX_STATE_DIR" long:"state-dir"`

	MemoryBudget uint64 `usage:"Memory, in bytes, admitted for running VMs. A run whose VM would take the admitted total past it is refused. Zero is 80% of the container's memory limit, less 512 MiB; with no limit and no budget, the service will not start." env:"WORKLOAD_MICROSANDBOX_MEMORY_BUDGET" long:"memory-budget"`
	MinMemory    uint64 `usage:"Memory, in bytes, a VM is given at the least, whatever its run asked for: the least a guest boots in." env:"WORKLOAD_MICROSANDBOX_MIN_MEMORY" long:"min-memory"`

	PortRange       string `usage:"Host ports runs' published ports are given, as first-last." env:"WORKLOAD_MICROSANDBOX_PORT_RANGE" long:"port-range"`
	PortBindAddress string `usage:"Address runs' published ports are bound on: one the orchestrators reach, since microsandbox binds 127.0.0.1 unless it is told otherwise." env:"WORKLOAD_MICROSANDBOX_PORT_BIND_ADDRESS" long:"port-bind-address"`

	Nameservers string `usage:"DNS servers a public run's guest asks, separated by commas, so that no guest asks docker's resolver, which would tell it the platform's own service names." env:"WORKLOAD_MICROSANDBOX_NAMESERVERS" long:"nameservers"`
}

// NewWorkloadMicrosandbox returns the configuration of the
// serve-workload-microsandbox command, holding the defaults it runs with until
// the console overrides them.
func NewWorkloadMicrosandbox() *WorkloadMicrosandbox {
	return &WorkloadMicrosandbox{
		Port:            defaultWorkloadMicrosandboxPort,
		StateDir:        defaultWorkloadMicrosandboxStateDir,
		MinMemory:       defaultWorkloadMicrosandboxMinMemory,
		PortRange:       defaultWorkloadMicrosandboxPortRange,
		PortBindAddress: defaultWorkloadMicrosandboxBindAddress,
		Nameservers:     defaultWorkloadMicrosandboxNameservers,
	}
}

// Budget is the memory admitted for running VMs: the one configured, or else
// 80% of the container's memory limit less 512 MiB, which leaves the service
// and microsandbox room beside the VMs.
//
// containerLimit is zero for a container with no limit, and then there is no
// telling how much memory the host can give, so the service refuses to guess:
// admission against a budget that is too large is no admission at all, and a
// host out of memory takes every VM with it.
func (c *WorkloadMicrosandbox) Budget(containerLimit uint64) (uint64, error) {
	if c.MemoryBudget > 0 {
		return c.MemoryBudget, nil
	}

	if containerLimit == 0 {
		return 0, errors.New("the container has no memory limit and WORKLOAD_MICROSANDBOX_MEMORY_BUDGET is not set, so there is no telling how much memory the VMs may have")
	}

	share := containerLimit / 100 * microsandboxBudgetShare
	if share <= microsandboxBudgetReserve {
		return 0, fmt.Errorf("the container's memory limit of %d bytes leaves the VMs nothing once 512 MiB is kept back", containerLimit)
	}

	return share - microsandboxBudgetReserve, nil
}

// HostPorts is the range of host ports runs' ports are published on, both
// ends included.
func (c *WorkloadMicrosandbox) HostPorts() (uint16, uint16, error) {
	first, last, found := strings.Cut(strings.TrimSpace(c.PortRange), "-")
	if !found {
		return 0, 0, fmt.Errorf("port range %q is not first-last", c.PortRange)
	}

	from, err := strconv.ParseUint(strings.TrimSpace(first), 10, 16)
	if err != nil {
		return 0, 0, fmt.Errorf("port range %q does not start with a port", c.PortRange)
	}

	to, err := strconv.ParseUint(strings.TrimSpace(last), 10, 16)
	if err != nil {
		return 0, 0, fmt.Errorf("port range %q does not end with a port", c.PortRange)
	}

	if from == 0 || to < from {
		return 0, 0, fmt.Errorf("port range %q holds no port", c.PortRange)
	}

	return uint16(from), uint16(to), nil
}

// NameserverList is the guests' DNS servers.
func (c *WorkloadMicrosandbox) NameserverList() []string {
	return commaSeparated(c.Nameservers)
}
