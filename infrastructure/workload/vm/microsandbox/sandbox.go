//go:build microsandbox

package microsandbox

import (
	"maps"
	"slices"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// createOptions is what a sandbox is made with for a record.
//
// A VM's root disk is flat: the whole filesystem, image and all, on one disk
// of the size it was given, which is what a VM's disk is and what a Docker VM
// needs, since dockerd's overlay cannot sit on another overlay. An instance
// with a main process, a code-runner task, is a container in a VM instead:
// the image stays read-only underneath and its disk is the managed layer it
// writes to, sized so that what it was given is what it can write.
//
// A Docker VM boots its own image with vminit as the guest's init, which
// starts dockerd and stops it gracefully when the VM is stopped.
func (e *engine) createOptions(r *record) ([]msb.SandboxOption, error) {
	spec := r.Spec

	cpus, err := vcpus(spec.Resources.CPUs)
	if err != nil {
		return nil, err
	}

	memory, err := mebibytes(spec.Resources.Memory)
	if err != nil {
		return nil, err
	}

	options := []msb.SandboxOption{
		msb.WithImage(r.Image),
		msb.WithDetached(),
		msb.WithPullPolicy(msb.PullPolicyIfMissing),
		msb.WithCPUs(cpus),
		msb.WithNetwork(e.policy(spec.Network)),
	}

	if memory > 0 {
		options = append(options, msb.WithMemory(memory))
	}

	if spec.HasMainProcess() {
		if spec.Resources.Disk > 0 {
			size, err := managedDiskMiB(spec.Resources.Disk)
			if err != nil {
				return nil, err
			}

			options = append(options, msb.WithRootDisk(msb.RootDisk.Managed(size)))
		}

		command := mainCommandOf(spec)
		if command.entrypoint != nil {
			options = append(options, msb.WithEntrypoint(command.entrypoint...))
		}

		if command.cmd != nil {
			options = append(options, msb.WithCmd(command.cmd...))
		}
	} else {
		size, err := mebibytes(spec.Resources.Disk)
		if err != nil {
			return nil, err
		}

		options = append(options, msb.WithRootDisk(msb.RootDisk.Flat(msb.RootDiskFlatOptions{SizeMiB: size})))
	}

	if len(spec.Labels) > 0 {
		options = append(options, msb.WithLabels(maps.Clone(spec.Labels)))
	}

	if env := envOf(spec.Env); env != nil {
		options = append(options, msb.WithEnv(env))
	}

	if len(spec.WorkingDir) > 0 {
		options = append(options, msb.WithWorkdir(spec.WorkingDir))
	}

	if spec.Kind == vm.KindDocker {
		options = append(options,
			msb.WithScripts(map[string]string{"vminit": vminitScript}),
			msb.WithInit(msb.Init.Cmd(vminitInit, msb.InitOptions{Args: []string{"--pid1"}})),
		)
	}

	if bindings := e.bindings(r); len(bindings) > 0 {
		options = append(options, msb.WithPortBindings(bindings...))
	}

	return options, nil
}

// restoreConfig is what a sandbox restored from a snapshot is given: the
// resources, network and ports of the record, which a restore takes afresh
// rather than from the snapshot.
func (e *engine) restoreConfig(r *record) (msb.RestoreConfig, error) {
	cpus, err := vcpus(r.Spec.Resources.CPUs)
	if err != nil {
		return msb.RestoreConfig{}, err
	}

	config := msb.RestoreConfig{
		CPUs:          &cpus,
		NetworkPolicy: e.policy(r.Spec.Network),
		Ports:         e.bindings(r),
	}

	memory, err := mebibytes(r.Spec.Resources.Memory)
	if err != nil {
		return msb.RestoreConfig{}, err
	}

	if memory > 0 {
		config.MemoryMiB = &memory
	}

	return config, nil
}

// policy is a VM's network as microsandbox enforces it.
//
// A VM that may reach out reaches the public internet and nothing else:
// never the private ranges the vmhost and every other VM's published ports
// are on, nor the host. One that may not reaches nothing, and names do not
// even resolve. Whether anybody may reach a VM in is its orchestrator's
// proxy's decision, which can change its mind at once, so a VM whose ingress
// is allowed takes connections from the orchestrator alone; one whose ingress
// is denied takes none, and publishes nothing. Nothing ever allows all: that
// would let a VM reach the others, and the host.
func (e *engine) policy(network vm.Network) *msb.NetworkConfig {
	egress := network.Egress == vm.AccessAllow
	ingress := network.Ingress == vm.AccessAllow

	if !egress && !ingress {
		return msb.NetworkPolicy.None()
	}

	var policy *msb.NetworkConfig
	if egress {
		policy = msb.NetworkPolicy.FromProfiles(msb.NetworkProfilePublic)
	} else {
		// no profile: egress denied, and no rule that lets names resolve.
		policy, _ = msb.NetworkPolicy.FromProfilesChecked()
	}

	policy.DefaultIngress = msb.PolicyActionDeny

	if ingress {
		policy.Rules = append(policy.Rules, msb.PolicyRule{
			Action:      msb.PolicyActionAllow,
			Direction:   msb.PolicyDirectionIngress,
			Destination: e.orchestrator,
		})
	}

	return policy
}

// bindings are the ports a record's sandbox publishes: each guest port on the
// host port it was given, on the vmhost's address on its pair network. An
// instance whose ingress is denied publishes nothing.
func (e *engine) bindings(r *record) []msb.PortBinding {
	if r.Spec.Network.Ingress != vm.AccessAllow {
		return nil
	}

	bindings := make([]msb.PortBinding, 0, len(r.HostPorts))
	for _, guest := range slices.Sorted(maps.Keys(r.HostPorts)) {
		bindings = append(bindings, msb.PortBinding{
			Bind:      e.options.BindAddress,
			HostPort:  uint16(r.HostPorts[guest]),
			GuestPort: uint16(guest),
			Protocol:  msb.PortProtocolTCP,
		})
	}

	return bindings
}
