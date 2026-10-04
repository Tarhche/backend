//go:build microsandbox

package sdk

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// The networks a sandbox can be on.
const (
	networkPublic   = "public"
	networkIsolated = "isolated"
)

// createOptions is what a sandbox is made with, in the SDK's terms.
//
// It is detached, so its VM outlives this process. It is single-tenant,
// because microsandbox's multi-tenant profile drops published ports, and every
// network setting is the supervisor's, none of it a user's. Its writable root
// is sized to hold the disk limit; see rootDiskMiB.
func createOptions(spec runs.SandboxSpec) ([]msb.SandboxOption, error) {
	if spec.Name == "" {
		return nil, errors.New("microsandbox: a sandbox needs a name")
	}

	if spec.Image == "" {
		return nil, fmt.Errorf("microsandbox: sandbox %s needs an image", spec.Name)
	}

	cpus, err := vcpus(spec.CPUs)
	if err != nil {
		return nil, err
	}

	network, err := networkConfig(spec)
	if err != nil {
		return nil, err
	}

	options := []msb.SandboxOption{
		msb.WithImage(spec.Image),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyIfMissing),
		msb.WithCPUs(cpus),
		msb.WithSecurityProfile(msb.SecurityProfileDefault),
		msb.WithDeploymentProfile(msb.DeploymentProfileSingleTenant),
		msb.WithNetwork(network),
	}

	if spec.Memory > 0 {
		memory, err := memoryMiB(spec.Memory, cpus)
		if err != nil {
			return nil, err
		}
		options = append(options, msb.WithMemory(memory))
	}

	if spec.Disk > 0 {
		disk, err := rootDiskMiB(spec.Disk)
		if err != nil {
			return nil, err
		}
		options = append(options, msb.WithRootDisk(msb.RootDisk.Managed(disk)))
	}

	if len(spec.Env) > 0 {
		options = append(options, msb.WithEnv(maps.Clone(spec.Env)))
	}

	if len(spec.Labels) > 0 {
		options = append(options, msb.WithLabels(maps.Clone(spec.Labels)))
	}

	if spec.Workdir != "" {
		options = append(options, msb.WithWorkdir(spec.Workdir))
	}

	for _, port := range spec.Ports {
		options = append(options, msb.WithPortBindings(msb.PortBinding{
			Bind:      port.Bind,
			HostPort:  port.HostPort,
			GuestPort: port.GuestPort,
			Protocol:  msb.PortProtocolTCP,
		}))
	}

	return options, nil
}

// networkConfig is a sandbox's network policy.
//
// Public denies by default and allows the gateway's DNS and public addresses,
// so private ranges, loopback, link-local addresses, metadata services and the
// host stay out of reach. Isolated allows no egress at all, DNS included. Both
// let published ports in.
func networkConfig(spec runs.SandboxSpec) (*msb.NetworkConfig, error) {
	var config *msb.NetworkConfig
	switch spec.Network {
	case networkPublic:
		config = msb.NetworkPolicy.FromProfiles(msb.NetworkProfilePublic)
		if len(spec.Nameservers) > 0 {
			config.DNS = &msb.DNSConfig{Nameservers: slices.Clone(spec.Nameservers)}
		}
	case networkIsolated:
		isolated, err := msb.NetworkPolicy.FromProfilesChecked()
		if err != nil {
			return nil, fmt.Errorf("microsandbox: an isolated network: %w", err)
		}
		config = isolated
	default:
		return nil, fmt.Errorf("microsandbox: network %q is neither %q nor %q", spec.Network, networkIsolated, networkPublic)
	}

	if spec.MaxTCPConnections > 0 {
		connections := uint(spec.MaxTCPConnections)
		config.MaxTCPConnections = &connections
	}

	return config, nil
}

// execOptions is how a command runs, in the SDK's terms. There is no option
// for a terminal's size: newProcess sizes it once the command has started.
//
// Without a stdin pipe, the SDK leaves a command's stdin open and silent, so
// a command that reads it waits for ever. Docker gives a command without
// stdin an empty one instead, and so does this: a command without a terminal
// gets a pipe either way, which Exec closes at once unless the command asked
// for stdin. One with a terminal and no stdin keeps a silent terminal, as
// docker's does.
func execOptions(command runs.Command) ([]msb.ExecOption, error) {
	env, err := envMap(command.Env)
	if err != nil {
		return nil, err
	}

	var options []msb.ExecOption
	if command.TTY {
		options = append(options, msb.WithExecTTY(true))
	}

	if stdinPipe(command) {
		options = append(options, msb.WithExecStdinPipe())
	}

	if len(env) > 0 {
		options = append(options, msb.WithExecEnv(env))
	}

	if command.Workdir != "" {
		options = append(options, msb.WithExecCwd(command.Workdir))
	}

	return options, nil
}

// stdinPipe is whether a command is started with a stdin pipe: see
// execOptions.
func stdinPipe(command runs.Command) bool {
	return command.Stdin || !command.TTY
}

// imageConfigOf is what a cached image says about how it is run.
func imageConfigOf(detail *msb.ImageDetail) runs.ImageConfig {
	var config runs.ImageConfig
	if detail.ImageHandle != nil {
		config.Architecture = detail.Architecture()
	}

	if c := detail.Config; c != nil {
		config.Entrypoint = c.Entrypoint
		config.Cmd = c.Cmd
		config.Env = c.Env
		config.WorkingDir = c.WorkingDir
		config.User = c.User
		config.StopSignal = c.StopSignal
	}

	return config
}

// metricsOf is a sandbox's metrics in the port's terms.
//
// Its memory use is MemoryBytes: what the guest has in use, which is its
// limit less what it reports available, the memory its kernel keeps included.
// MemoryHostResidentBytes is something else: the host memory the VM holds,
// which grows as the guest touches memory and shrinks some time after it
// frees it.
func metricsOf(m *msb.Metrics) runs.Metrics {
	return runs.Metrics{
		CPUPercent:  m.CPUPercent,
		MemoryUsage: m.MemoryBytes,
		MemoryLimit: m.MemoryLimitBytes,
		NetRx:       m.NetRxBytes,
		NetTx:       m.NetTxBytes,
		DiskRead:    m.DiskReadBytes,
		DiskWrite:   m.DiskWriteBytes,
	}
}
