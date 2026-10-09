// Package stack is the stack kind's node strategy: what a node does to the
// stacks in its Docker VMs, and what it says they are doing.
//
// It keeps no record of a stack. What it knows of one is what a command
// carries, the stack as the control plane recorded it, and what its building
// blocks say:
//
//   - the Docker VM the stack lives in, which the engine says is running, or
//     is not, in which case nothing is done to the stack;
//   - that VM's dockerd, waited for while the VM comes up, whose containers
//     labelled as the stack's are what the stack is now;
//   - and the Applier, which brings those containers to what the stack's
//     compose file says.
//
// A stack's state is its containers mapped onto its services. The services
// that are to be running are those its containers' labels name, so a
// service whose containers are all gone is still counted: running when every
// one of them runs, stopped when none does, and degraded otherwise. A node
// carrying out a command on a stack says it is still in flight until the
// command is done, so that what its containers do halfway through a deploy
// is not taken for what the deploy came to.
package stack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// readMargin is how long before a state's deadline the reads of the Docker
// VMs are given up on, so that the report says which of them did not answer
// rather than not being made at all.
const readMargin = 100 * time.Millisecond

// exited reads a container's exit code out of what docker says of it.
var exited = regexp.MustCompile(`^Exited \((-?\d+)\)`)

// Daemons are the dockerds of this node's Docker VMs.
type Daemons interface {
	Daemon(vmUUID string) docker.Daemon
}

// Node is the stack kind's node strategy.
type Node struct {
	engine  vm.Engine
	daemons Daemons
	applier stackKind.Applier

	// timeout bounds a command: waiting for its VM's dockerd, and what the
	// applier does, which may pull images first.
	timeout time.Duration

	lock sync.Mutex

	// underway are the stacks a command is being carried out on, by uuid.
	underway map[string]underway

	// outputs are what the last command on each stack printed.
	outputs map[string]string

	// dockerVMs are the VMs a command named as a stack's, which are Docker
	// VMs whatever their labels say: those made before VMs were labelled.
	dockerVMs map[string]bool
}

// underway is a command being carried out on a stack.
type underway struct {
	state kind.State
	vm    string
}

var _ kind.Node[stackKind.Spec, stackKind.Status] = &Node{}

// New is the strategy that finds Docker VMs through engine, reads them
// through daemons, and applies stacks with applier, each command given
// timeout.
func New(engine vm.Engine, daemons Daemons, applier stackKind.Applier, timeout time.Duration) *Node {
	return &Node{
		engine:    engine,
		daemons:   daemons,
		applier:   applier,
		timeout:   timeout,
		underway:  make(map[string]underway),
		outputs:   make(map[string]string),
		dockerVMs: make(map[string]bool),
	}
}

// Execute carries out one of a stack's commands: its Docker VM has to be
// running, or coming up, and its dockerd is waited for; then the applier
// does what the command asks. What came of it is what the stack's containers
// are doing afterwards, and what the applier printed.
func (n *Node) Execute(ctx context.Context, s stackKind.Stack, action string, payload any) (kind.Outcome[stackKind.Status], error) {
	vmUUID := stackKind.VMOf(s)
	if len(vmUUID) == 0 {
		return kind.Outcome[stackKind.Status]{}, fmt.Errorf("%w: the stack names no vm", kind.ErrInvalidPayload)
	}

	n.begin(s.Metadata.UUID, vmUUID, stackKind.InFlightOf(action))
	defer n.end(s.Metadata.UUID)

	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	instance, err := n.engine.Inspect(ctx, vmUUID)
	switch {
	case errors.Is(err, domain.ErrNotExists) && action == stackKind.ActionDelete:
		// nothing of it is left anywhere to take down.
		n.forget(s.Metadata.UUID)

		return kind.Outcome[stackKind.Status]{Output: "its vm is not here: there is nothing to take down"}, nil
	case err != nil:
		return failed("", err)
	case instance.State != vm.InstanceRunning && instance.State != vm.InstanceCreated:
		// a VM that is not running, or coming up, has no dockerd to apply
		// anything with.
		return failed("", fmt.Errorf("%w: its vm is %s", vm.ErrNotRunning, instance.State))
	}

	if err := n.daemons.Daemon(vmUUID).Ping(ctx); err != nil {
		return failed("", err)
	}

	output, err := n.apply(ctx, s, vmUUID, action, payload)
	output = lastOf(output, kind.MaxOutput)

	if err != nil {
		n.said(s.Metadata.UUID, output)

		return failed(output, err)
	}

	if action == stackKind.ActionDelete {
		n.forget(s.Metadata.UUID)

		return kind.Outcome[stackKind.Status]{Output: output}, nil
	}

	n.said(s.Metadata.UUID, output)

	status := n.after(ctx, s, vmUUID, action)
	status.Output = output

	return kind.Outcome[stackKind.Status]{Status: status, Output: output}, nil
}

// Query answers none of a stack's queries but its state, which is read off
// State.
func (n *Node) Query(_ context.Context, _ stackKind.Stack, action string, _ any) (any, error) {
	return nil, fmt.Errorf("%w: a stack has no %q", kind.ErrUnknownAction, action)
}

// State is every stack in this node's running Docker VMs: the containers of
// each VM labelled as a stack's, read in one listing per VM, all of the VMs at
// once, and mapped onto each stack's services. A VM whose dockerd did not
// answer in time is unseen, and says nothing of the stacks in it; one that is
// not running is not read at all, and what is in it waits on it.
func (n *Node) State(ctx context.Context) (kind.Report[stackKind.Status], error) {
	instances, err := n.engine.List(ctx)
	if err != nil {
		return kind.Report[stackKind.Status]{}, err
	}

	reading := ctx
	if deadline, bounded := ctx.Deadline(); bounded {
		var cancel context.CancelFunc

		reading, cancel = context.WithDeadline(ctx, deadline.Add(-readMargin))
		defer cancel()
	}

	type listed struct {
		vm         string
		containers []docker.Container
		err        error
	}

	var vms []string

	for _, instance := range instances {
		if uuid, docker := n.dockerVM(instance); docker && instance.State == vm.InstanceRunning {
			vms = append(vms, uuid)
		}
	}

	answers := make(chan listed, len(vms))
	for _, uuid := range vms {
		go func() {
			containers, err := n.daemons.Daemon(uuid).Containers(reading, docker.ContainerFilter{All: true, Label: stackKind.LabelStack})
			answers <- listed{vm: uuid, containers: containers, err: err}
		}()
	}

	report := kind.Report[stackKind.Status]{Instances: []kind.Observed[stackKind.Status]{}}
	found := make(map[string]kind.Observed[stackKind.Status])

	for range vms {
		var answer listed

		select {
		case answer = <-answers:
		case <-reading.Done():
			answer = listed{err: reading.Err()}
		}

		if len(answer.vm) == 0 {
			break
		}

		if answer.err != nil {
			report.Unseen = append(report.Unseen, answer.vm)

			continue
		}

		report.Read = append(report.Read, answer.vm)

		for uuid, containers := range byStack(answer.containers) {
			found[uuid] = kind.Observed[stackKind.Status]{
				UUID:   uuid,
				Owners: []kind.Reference{{Kind: stackKind.Parent, UUID: answer.vm}},
				Status: statusOf(containers),
			}
		}
	}

	// the VMs that did not answer before the reads were given up on.
	for _, uuid := range vms {
		if !slices.Contains(report.Read, uuid) && !slices.Contains(report.Unseen, uuid) {
			report.Unseen = append(report.Unseen, uuid)
		}
	}

	n.lock.Lock()
	for uuid, command := range n.underway {
		if !slices.Contains(report.Read, command.vm) || len(command.state) == 0 {
			continue
		}

		observed, ok := found[uuid]
		if !ok {
			observed = kind.Observed[stackKind.Status]{UUID: uuid, Owners: []kind.Reference{{Kind: stackKind.Parent, UUID: command.vm}}}
		}

		observed.Status.State = command.state
		found[uuid] = observed
	}

	for uuid, observed := range found {
		observed.Status.Output = n.outputs[uuid]
		report.Instances = append(report.Instances, observed)
	}
	n.lock.Unlock()

	slices.SortFunc(report.Instances, func(a, b kind.Observed[stackKind.Status]) int { return strings.Compare(a.UUID, b.UUID) })
	slices.Sort(report.Read)
	slices.Sort(report.Unseen)

	return report, nil
}

// apply has the applier do what action asks of a stack. A start or a
// restart of a stack its VM has nothing of makes it, which is what starting
// one that never came up, or was taken away, amounts to.
func (n *Node) apply(ctx context.Context, s stackKind.Stack, vmUUID string, action string, payload any) (string, error) {
	switch action {
	case stackKind.ActionCreate, stackKind.ActionApply:
		return n.applier.Up(ctx, s)
	case stackKind.ActionStart:
		if n.absent(ctx, s, vmUUID) {
			return n.applier.Up(ctx, s)
		}

		return n.applier.Start(ctx, s)
	case stackKind.ActionStop:
		return n.applier.Stop(ctx, s)
	case stackKind.ActionRestart:
		if n.absent(ctx, s, vmUUID) {
			return n.applier.Up(ctx, s)
		}

		return n.applier.Restart(ctx, s)
	case stackKind.ActionDelete:
		remove, _ := payload.(stackKind.DeletePayload)

		return n.applier.Down(ctx, s, remove.RemoveVolumes)
	}

	return "", fmt.Errorf("%w: a stack cannot be %s", kind.ErrUnknownAction, action)
}

// absent reports whether a stack's VM has none of its containers. One that
// cannot be read is taken to have them.
func (n *Node) absent(ctx context.Context, s stackKind.Stack, vmUUID string) bool {
	containers, err := n.containers(ctx, s, vmUUID)

	return err == nil && len(containers) == 0
}

// after is what a command left a stack as: what its containers are doing
// now, or, when they cannot be read, what the command was to make it.
func (n *Node) after(ctx context.Context, s stackKind.Stack, vmUUID string, action string) stackKind.Status {
	target := stackKind.Running
	if action == stackKind.ActionStop {
		target = stackKind.Stopped
	}

	containers, err := n.containers(ctx, s, vmUUID)
	if err != nil || len(containers) == 0 {
		return stackKind.Status{Status: kind.Status{State: target}}
	}

	return statusOf(containers)
}

// containers are the containers of one stack in its VM.
func (n *Node) containers(ctx context.Context, s stackKind.Stack, vmUUID string) ([]docker.Container, error) {
	return n.daemons.Daemon(vmUUID).Containers(ctx, docker.ContainerFilter{All: true, Label: stackKind.LabelStack + "=" + s.Metadata.UUID})
}

// dockerVM is the uuid of a VM instance and whether it is a Docker VM: one
// its engine says is one, as its image does, or one a stack's command named.
func (n *Node) dockerVM(instance vm.Instance) (string, bool) {
	if instance.Labels[vm.LabelPurpose] != vm.PurposeVM {
		return "", false
	}

	uuid := instance.Labels[vm.LabelVM]
	if len(uuid) == 0 {
		uuid = instance.ID
	}

	if instance.Labels[vmKind.LabelDocker] == "true" {
		return uuid, true
	}

	n.lock.Lock()
	defer n.lock.Unlock()

	return uuid, n.dockerVMs[uuid]
}

// begin says a command is being carried out on a stack, in its VM.
func (n *Node) begin(stackUUID string, vmUUID string, state kind.State) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.underway[stackUUID] = underway{state: state, vm: vmUUID}
	n.dockerVMs[vmUUID] = true
}

// end says the command being carried out on a stack is done.
func (n *Node) end(stackUUID string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	delete(n.underway, stackUUID)
}

// said keeps what the last command on a stack printed.
func (n *Node) said(stackUUID string, output string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.outputs[stackUUID] = output
}

// forget lets go of what is kept of a stack that is gone.
func (n *Node) forget(stackUUID string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	delete(n.outputs, stackUUID)
}

// failed is what a command that failed left a stack as: failed, with what
// was printed before it did.
func failed(output string, err error) (kind.Outcome[stackKind.Status], error) {
	status := stackKind.Status{Status: kind.Status{State: stackKind.Failed}, Output: output}

	return kind.Outcome[stackKind.Status]{Status: status, Output: output}, err
}

// byStack is containers by the stack they are labelled as.
func byStack(containers []docker.Container) map[string][]docker.Container {
	grouped := make(map[string][]docker.Container)

	for _, c := range containers {
		if uuid := c.Labels[stackKind.LabelStack]; len(uuid) > 0 {
			grouped[uuid] = append(grouped[uuid], c)
		}
	}

	return grouped
}

// statusOf is what a stack is doing, read off its containers: each of its
// services by the containers compose made for it, and the stack by the
// services that are to be running, which its containers' labels name.
func statusOf(containers []docker.Container) stackKind.Status {
	services := make(map[string][]docker.Container)

	var expected []string

	for _, c := range containers {
		services[c.Service] = append(services[c.Service], c)

		if named := c.Labels[stackKind.LabelServices]; len(named) > 0 && expected == nil {
			expected = strings.Split(named, ",")
		}
	}

	// a stack labelled before its services were is held to the ones it has.
	if expected == nil {
		for name, of := range services {
			if !slices.ContainsFunc(of, completed) {
				expected = append(expected, name)
			}
		}
	}

	names := slices.Clone(expected)
	for name := range services {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	slices.Sort(names)

	status := stackKind.Status{Services: make([]stackKind.ServiceStatus, 0, len(names))}
	running, some := 0, false

	for _, name := range names {
		toRun := slices.Contains(expected, name)

		service := serviceOf(name, services[name], toRun)
		status.Services = append(status.Services, service)

		if !toRun {
			continue
		}

		switch service.State {
		case stackKind.ServiceRunning:
			running++
			some = true
		case stackKind.ServiceDegraded:
			some = true
		}
	}

	switch {
	case len(expected) > 0 && running == len(expected):
		status.State = stackKind.Running
	case !some:
		status.State = stackKind.Stopped
	default:
		status.State = stackKind.Degraded
	}

	return status
}

// serviceOf is one service of a stack, by its containers. One that is to be
// running is stopped when none of them runs; one that is not, that runs once
// and ends, is completed when each of them did and said it went well.
func serviceOf(name string, containers []docker.Container, toRun bool) stackKind.ServiceStatus {
	service := stackKind.ServiceStatus{Name: name}

	slices.SortFunc(containers, func(a, b docker.Container) int { return strings.Compare(a.Name, b.Name) })

	running := 0
	for _, c := range containers {
		service.Containers = append(service.Containers, stackKind.ContainerStatus{ID: c.ID, Name: c.Name, State: c.State})

		if c.State == "running" {
			running++
		}
	}

	switch {
	case len(containers) == 0:
		service.State = stackKind.ServiceMissing
	case running == len(containers):
		service.State = stackKind.ServiceRunning
	case running > 0:
		service.State = stackKind.ServiceDegraded
	case !toRun && !slices.ContainsFunc(containers, func(c docker.Container) bool { return !completed(c) }):
		service.State = stackKind.ServiceCompleted
	default:
		service.State = stackKind.ServiceStopped
	}

	return service
}

// completed reports whether a container ran to its end and said it went
// well, as one that runs once does.
func completed(c docker.Container) bool {
	if c.State != "exited" {
		return false
	}

	code := exited.FindStringSubmatch(c.Status)

	return len(code) == 2 && code[1] == "0"
}

// lastOf is the last limit bytes of output, starting at a whole character.
func lastOf(output string, limit int) string {
	if len(output) <= limit {
		return output
	}

	output = output[len(output)-limit:]
	for len(output) > 0 && !utf8.RuneStart(output[0]) {
		output = output[1:]
	}

	return output
}
