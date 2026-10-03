package microvm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// What a task is travels with the VM running it, written on it as labels,
// which vmhost keeps with the VM as docker keeps them with a container. It is
// why a node can say what it is holding, and whose, without asking anything
// that keeps records, and why an orchestrator that is redeployed finds every
// VM where it left it.
//
// The keys are the container driver's, on purpose: they are a storage format
// that VMs running right now carry, and a VM and a container that hold the
// same task say so in the same words, so whatever reads one reads the other.
const (
	taskUUIDLabel    = "task.uuid"
	taskNameLabel    = "task.name"
	taskSlugLabel    = "task.slug"
	taskKindLabel    = "task.kind"
	nodeNameLabel    = "node.name"
	taskOwnerLabel   = "task.owner"
	taskStackLabel   = "task.stack"
	taskInteractive  = "task.interactive"
	taskTTLLabel     = "task.ttl"
	taskAttemptLabel = "task.attempt"
)

// labelsOf writes down what a VM is running, so that reading the VM back says
// it again. The node is the driver's own, which is the only node whose VMs it
// will ever list again.
func labelsOf(execution *task.Execution, node string) map[string]string {
	labels := map[string]string{
		taskUUIDLabel:    execution.TaskUUID,
		taskNameLabel:    execution.TaskName,
		taskSlugLabel:    execution.Slug,
		taskKindLabel:    string(execution.Kind),
		nodeNameLabel:    node,
		taskOwnerLabel:   execution.OwnerUUID,
		taskAttemptLabel: strconv.Itoa(execution.Attempt),
		taskInteractive:  strconv.FormatBool(execution.Interactive),
	}

	if len(execution.StackUUID) > 0 {
		labels[taskStackLabel] = execution.StackUUID
	}

	// how long it may run for once it is up. What that is counted from is not
	// written down: the VM itself says when it started.
	if execution.TTL > 0 {
		labels[taskTTLLabel] = strconv.Itoa(int(execution.TTL.Seconds()))
	}

	return labels
}

// identify reads back what a VM is running. A label that is missing reads as
// nothing rather than as an error: it is still a VM this node is holding.
func identify(execution *task.Execution, labels map[string]string) {
	execution.TaskUUID = labels[taskUUIDLabel]
	execution.TaskName = labels[taskNameLabel]
	execution.Slug = labels[taskSlugLabel]
	execution.NodeName = labels[nodeNameLabel]
	execution.OwnerUUID = labels[taskOwnerLabel]
	execution.StackUUID = labels[taskStackLabel]
	execution.Interactive = labels[taskInteractive] == "true"

	if kind := task.Kind(labels[taskKindLabel]); kind.IsValid() {
		execution.Kind = kind
	} else {
		execution.Kind = task.DefaultKind
	}

	if attempt, err := strconv.Atoi(labels[taskAttemptLabel]); err == nil && attempt > 0 {
		execution.Attempt = attempt
	}

	if seconds, err := strconv.Atoi(labels[taskTTLLabel]); err == nil && seconds > 0 {
		execution.TTL = time.Duration(seconds) * time.Second
	}
}

// specOf is the VM vmhost is asked to make for a run, in the words a task is
// asked for in: vmhost turns them into a machine.
//
// What is not said is what a VM has no use for. Its ports are published
// nowhere, so there are no bindings: a VM's port is reached through vmhost,
// which is what makes the driver a task.Dialer. And it has no health check,
// which the control plane refuses before any task gets this far.
func specOf(execution *task.Execution, node string) (vm.Spec, error) {
	// a VM is found again only by the node it is labelled with. One made for
	// another node would be lost to both: this one would never list it, and
	// the other one never asked vmhost for it.
	if len(execution.NodeName) > 0 && execution.NodeName != node {
		return vm.Spec{}, fmt.Errorf("%w: a run for node %q cannot be made by node %q", vm.ErrInvalid, execution.NodeName, node)
	}

	ports, err := exposedPortsOf(execution.ExposedPorts)
	if err != nil {
		return vm.Spec{}, err
	}

	return vm.Spec{
		Name: execution.Name,

		// what a task's ports are served under, which is a hostname's
		// leftmost label already, so the task calls itself what it is called.
		Hostname: execution.Slug,

		Image:      execution.Image,
		Labels:     labelsOf(execution, node),
		Entrypoint: execution.Entrypoint,
		Command:    execution.Command,
		Env:        execution.Environment,
		WorkingDir: execution.WorkingDirectory,

		// bytes, as they are everywhere in the workload: vmhost turns them
		// into a machine's size, and the disk is finally a limit rather than
		// a wish, since it is the size of the disk the task writes to.
		Resources: vm.Resources{
			CPU:    execution.ResourceLimits.Cpu,
			Memory: execution.ResourceLimits.Memory,
			Disk:   execution.ResourceLimits.Disk,
		},

		ReadOnly:      execution.ReadOnly,
		RestartPolicy: execution.RestartPolicy,
		AutoRemove:    execution.AutoRemove,
		Networks:      attachmentsOf(execution.Networks),
		ExposedPorts:  ports,
	}, nil
}

// executionOf is a VM as the rest of the workload sees a run.
//
// A listing and an inspection say the same here, unlike docker's: vmhost
// answers both from the same record, so a listed run already carries its exit
// code and when it started.
func executionOf(v vm.VM, class workloadRuntime.Class) task.Execution {
	execution := task.Execution{
		ID:      v.ID,
		Name:    v.Spec.Name,
		Runtime: class,
		Status:  statusOf(v.State),
		Image:   v.Spec.Image,
		ResourceLimits: task.ResourceLimits{
			Cpu:    v.Spec.Resources.CPU,
			Memory: v.Spec.Resources.Memory,
			Disk:   v.Spec.Resources.Disk,
		},
		RestartPolicy:    v.Spec.RestartPolicy,
		RestartCount:     v.RestartCount,
		WorkingDirectory: v.Spec.WorkingDir,
		ExposedPorts:     portSetOf(v.Spec.ExposedPorts),

		// reachable now, which a VM is on every port it exposes while it runs
		// on a network; nothing is published anywhere, so there are no
		// bindings to say where.
		Endpoints: endpointsOf(v.Endpoints()),

		Networks:    networksOf(v.Spec.Networks),
		AutoRemove:  v.Spec.AutoRemove,
		Environment: v.Spec.Env,
		Entrypoint:  v.Spec.Entrypoint,
		Command:     v.Spec.Command,
		CreatedAt:   v.CreatedAt,
		StartedAt:   v.StartedAt,
		ExitCode:    v.ExitCode,
		ReadOnly:    v.Spec.ReadOnly,
	}

	identify(&execution, v.Spec.Labels)

	return execution
}

// statuses are vmhost's states as a run's: they are docker's states, so they
// map one to one.
var statuses = map[vm.State]task.Status{
	vm.StateCreated:    task.StatusCreated,
	vm.StateRunning:    task.StatusRunning,
	vm.StateRestarting: task.StatusRestarting,
	vm.StateExited:     task.StatusExited,
	vm.StateDead:       task.StatusDead,
	vm.StateRemoving:   task.StatusRemoving,
}

// statusOf is a VM's state as a run's. A state this driver does not know — a
// newer vmhost's — is one it cannot say the task is running in, so it is
// reported dead, which hands the task to the control plane's retries rather
// than leaving it on a run nobody can account for.
func statusOf(state vm.State) task.Status {
	if status, found := statuses[state]; found {
		return status
	}

	return task.StatusDead
}

// attachmentsOf is the networks a VM joins, from the ones a task is given.
//
// Docker's words for two of them mean nothing to vmhost. "none" is no network
// device at all, which a VM gets by joining nothing. And docker's default
// bridge, which is the network that routes out, is vmhost's public network.
func attachmentsOf(attachments []network.Attachment) []vm.Attachment {
	joined := make([]vm.Attachment, 0, len(attachments))

	for _, attachment := range attachments {
		if attachment.Name == network.NoNetworkName {
			continue
		}

		joined = append(joined, vm.Attachment{
			Network: networkName(attachment.Name),
			Aliases: attachment.Aliases,
			Gateway: attachment.Gateway,
		})
	}

	if len(joined) == 0 {
		return nil
	}

	return joined
}

// networksOf is the networks a VM joins, in the words the task was given
// them in, so that what a run reports about itself is the shape of what it
// was asked for.
func networksOf(attachments []vm.Attachment) []network.Attachment {
	if len(attachments) == 0 {
		return []network.Attachment{{Name: network.NoNetworkName}}
	}

	networks := make([]network.Attachment, len(attachments))
	for i, attachment := range attachments {
		name := attachment.Network
		if name == vm.PublicNetwork {
			name = network.PublicNetworkName
		}

		networks[i] = network.Attachment{Name: name, Aliases: attachment.Aliases, Gateway: attachment.Gateway}
	}

	return networks
}

// maxNetworkName is the longest name vmhost gives a network
// (vm.IsNetworkName).
const maxNetworkName = 63

// networkName is what vmhost calls one of the workload's networks.
//
// A stack's network is named after the stack's slug, which is a hostname's
// whole leftmost label, so with the prefix in front the name can be longer than
// vmhost takes one. Such a name is cut short and told apart from every other
// one cut short by a hash of the whole of it. The same name is always cut the
// same way, which is all that making a network, joining it and removing it
// need of it.
func networkName(name string) string {
	if name == network.PublicNetworkName {
		return vm.PublicNetwork
	}

	if len(name) <= maxNetworkName {
		return name
	}

	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:8])

	kept := strings.TrimRight(name[:maxNetworkName-len(suffix)-1], "-")

	return kept + "-" + suffix
}

// exposedPortsOf is the ports a VM serves on, in order. A port that cannot be
// one is refused rather than dropped: a task that serves on it would not be
// reachable on it, and nothing would say why.
func exposedPortsOf(set port.PortSet) ([]uint16, error) {
	if len(set) == 0 {
		return nil, nil
	}

	ports := make([]uint16, 0, len(set))
	for p := range set {
		if p == 0 || p > math.MaxUint16 {
			return nil, fmt.Errorf("%w: %d is not a port", vm.ErrInvalid, p)
		}

		ports = append(ports, uint16(p))
	}

	slices.Sort(ports)

	return ports, nil
}

func portSetOf(ports []uint16) port.PortSet {
	set := make(port.PortSet, len(ports))
	for _, p := range ports {
		set[port.Port(p)] = struct{}{}
	}

	return set
}

func endpointsOf(ports []uint16) []port.Port {
	if len(ports) == 0 {
		return nil
	}

	endpoints := make([]port.Port, len(ports))
	for i, p := range ports {
		endpoints[i] = port.Port(p)
	}

	return endpoints
}

// statsOf is what a VM uses, as a run's.
func statsOf(stats vm.Stats) task.Stats {
	used := task.Stats{
		PIDs:          stats.PIDs,
		CPUPercent:    stats.CPUPercent,
		MemoryUsage:   stats.MemoryUsage,
		MemoryLimit:   stats.MemoryLimit,
		NetworkInput:  stats.NetworkInput,
		NetworkOutput: stats.NetworkOutput,
		BlockInput:    stats.BlockInput,
		BlockOutput:   stats.BlockOutput,
	}

	if stats.MemoryLimit > 0 {
		used.MemoryPercent = float64(stats.MemoryUsage) / float64(stats.MemoryLimit) * 100
	}

	return used
}

// streamOf is the stream a line of a VM's output came from.
func streamOf(stream string) task.Stream {
	if stream == guest.StreamStderr {
		return task.StreamStderr
	}

	return task.StreamStdout
}
