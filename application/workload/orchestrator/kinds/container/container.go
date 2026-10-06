// Package container is the container kind's node strategy: what a node does
// to the containers in its Docker VMs, and what it says they are doing.
//
// It keeps no record of a container. What it knows of one is what a command
// carries, the container as the control plane recorded it, and what the
// dockerd of its VM says: a container is found by its Docker id, or by the
// label the platform made it with, which says which resource it is.
//
// Its state is every container of every running Docker VM on the node, read
// with the other building blocks in one listing of each sort of object per VM
// (blocks.Reader): the platform's by the resource each is, in the kind's words
// as their restart policies say them, and the rest, a stack's or nobody's, as
// they are, for the control plane to show beside its records.
package container

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Node is the container kind's node strategy.
type Node struct {
	reader *blocks.Reader

	// timeout bounds a command: waiting for its VM's dockerd, and a pull of
	// its image before it is made.
	timeout time.Duration
}

var _ kind.Node[containerKind.Spec, containerKind.Status] = &Node{}

// New is the strategy that reaches and reads Docker VMs through reader, each
// command given timeout.
func New(reader *blocks.Reader, timeout time.Duration) *Node {
	return &Node{reader: reader, timeout: timeout}
}

// Execute carries out one of a container's commands in its VM's dockerd, and
// is what the container is doing afterwards. A VM that is not running, a
// dockerd that does not come up and what dockerd refuses fail the command,
// saying so in the codes every side knows.
func (n *Node) Execute(ctx context.Context, c containerKind.Container, action string, payload any) (kind.Outcome[containerKind.Status], error) {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	daemon, err := n.reader.Reach(ctx, containerKind.VMOf(c))

	switch {
	case errors.Is(err, blocks.ErrGone) && action == containerKind.ActionDelete:
		// nothing of it is left anywhere to remove.
		return kind.Outcome[containerKind.Status]{}, nil
	case err != nil:
		return failed(err)
	}

	switch action {
	case containerKind.ActionCreate:
		return n.create(ctx, daemon, c)
	case containerKind.ActionStart:
		return n.start(ctx, daemon, c)
	case containerKind.ActionStop:
		return n.stop(ctx, daemon, c)
	case containerKind.ActionRestart:
		return n.restart(ctx, daemon, c)
	case containerKind.ActionConnect:
		connect, _ := payload.(containerKind.ConnectPayload)

		return n.on(ctx, daemon, c, func(id string) error {
			return daemon.ConnectNetwork(ctx, connect.Network, id, connect.Aliases)
		})
	case containerKind.ActionDisconnect:
		disconnect, _ := payload.(containerKind.DisconnectPayload)

		return n.on(ctx, daemon, c, func(id string) error {
			return daemon.DisconnectNetwork(ctx, disconnect.Network, id, disconnect.Force)
		})
	case containerKind.ActionDelete:
		return n.remove(ctx, daemon, c)
	}

	return failed(fmt.Errorf("%w: a container cannot be %s on its node", kind.ErrUnknownAction, action))
}

// Query reads what a container wrote, or samples what it uses.
func (n *Node) Query(ctx context.Context, c containerKind.Container, action string, payload any) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	daemon, err := n.reader.Reach(ctx, containerKind.VMOf(c))
	if err != nil {
		return nil, err
	}

	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return nil, err
	case !there:
		return nil, fmt.Errorf("%w: the container is not in its vm", domain.ErrNotExists)
	}

	switch action {
	case containerKind.ActionLogs:
		options, _ := payload.(containerKind.LogsPayload)

		return logs(ctx, daemon, found.ID, options)
	case containerKind.ActionStats:
		stats, err := daemon.ContainerStats(ctx, found.ID)
		if err != nil {
			return nil, err
		}

		return containerKind.StatsOf(stats), nil
	}

	return nil, fmt.Errorf("%w: a container has no %q", kind.ErrUnknownAction, action)
}

// State is every container of every running Docker VM on the node.
func (n *Node) State(ctx context.Context) (kind.Report[containerKind.Status], error) {
	read, err := n.reader.Read(ctx)
	if err != nil {
		return kind.Report[containerKind.Status]{}, err
	}

	report := blocks.Report[containerKind.Status](read)

	for _, vmUUID := range read.VMs() {
		for _, c := range read.Inventories[vmUUID].Containers {
			report.Instances = append(report.Instances, Observed(vmUUID, c))
		}
	}

	slices.SortFunc(report.Instances, func(a, b kind.Observed[containerKind.Status]) int {
		return cmp.Or(strings.Compare(a.UUID, b.UUID), strings.Compare(a.Status.Docker.ID, b.Status.Docker.ID))
	})

	return report, nil
}

// Observed is what is said of a container a Docker VM's dockerd listed: the
// resource it is, when the platform made it, what it belongs to, and what it
// is doing, as the restart policy it was made with says, when it is known.
func Observed(vmUUID string, c docker.Container) kind.Observed[containerKind.Status] {
	if policy, known := containerKind.PolicyOf(c.Labels); known {
		c.RestartPolicy = policy
	}

	return kind.Observed[containerKind.Status]{
		UUID:   blockKinds.Identity(containerKind.Name, c.Labels),
		Owners: blockKinds.Owners(vmUUID, c.Labels),
		Status: containerKind.Status{
			Status: kind.Status{State: containerKind.StateOf(c.State, c.RestartPolicy)},
			Docker: containerKind.DockerOf(c),
		},
	}
}

// create makes a container that is not there, pulling its image first when
// its VM does not hold it, labelled as the resource it is, with its spec
// beside. One that is there already, a create asked again, is what was asked
// for, and is left as it is, but for being started when it is to be running.
// One that is to be stopped is stopped once it is made.
func (n *Node) create(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)
	if err != nil {
		return failed(err)
	}

	if !there {
		spec, err := containerKind.SpecLabel(c.Spec)
		if err != nil {
			return failed(err)
		}

		if found, err = daemon.CreateContainer(ctx, c.Spec.Docker(c.Metadata.UUID, map[string]string{containerKind.LabelSpec: spec})); err != nil {
			return failed(err)
		}
	}

	switch running := found.State == "running"; {
	case c.Status.Expected == containerKind.Stopped && running:
		if err := daemon.StopContainer(ctx, found.ID); err != nil {
			return failed(err)
		}
	case c.Status.Expected != containerKind.Stopped && !running:
		if err := daemon.StartContainer(ctx, found.ID); err != nil {
			return failed(err)
		}
	default:
		return outcome(found), nil
	}

	return n.inspected(ctx, daemon, found.ID)
}

// start starts a container, and makes one its VM does not have.
func (n *Node) start(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return failed(err)
	case !there:
		return n.create(ctx, daemon, c)
	}

	if err := daemon.StartContainer(ctx, found.ID); err != nil {
		return failed(err)
	}

	return n.inspected(ctx, daemon, found.ID)
}

// stop stops a container. One its VM does not have is not running, which is
// what was asked.
func (n *Node) stop(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return failed(err)
	case !there:
		return kind.Outcome[containerKind.Status]{Status: containerKind.Status{Status: kind.Status{State: containerKind.Stopped}, Failure: blocks.Cleared()}}, nil
	}

	if err := daemon.StopContainer(ctx, found.ID); err != nil {
		return failed(err)
	}

	return n.inspected(ctx, daemon, found.ID)
}

// restart stops a running container and starts it again, starts one that is
// not running, and makes one its VM does not have: what somebody asking for a
// restart wants to end with.
func (n *Node) restart(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return failed(err)
	case !there:
		return n.create(ctx, daemon, c)
	case found.State == "running":
		err = daemon.RestartContainer(ctx, found.ID)
	default:
		err = daemon.StartContainer(ctx, found.ID)
	}

	if err != nil {
		return failed(err)
	}

	return n.inspected(ctx, daemon, found.ID)
}

// on does what change does to a container that is there, by its Docker id,
// and is what it is doing afterwards.
func (n *Node) on(ctx context.Context, daemon docker.Daemon, c containerKind.Container, change func(id string) error) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return failed(err)
	case !there:
		return failed(fmt.Errorf("%w: the container is not in its vm", domain.ErrNotExists))
	}

	if err := change(found.ID); err != nil {
		return failed(err)
	}

	return n.inspected(ctx, daemon, found.ID)
}

// remove removes a container, running or not: whether one that runs may be
// removed was asked before it was sent here, and a delete that reached its
// node is to leave it gone. One its VM does not have is gone already.
func (n *Node) remove(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)

	switch {
	case err != nil:
		return failed(err)
	case !there:
		return kind.Outcome[containerKind.Status]{}, nil
	}

	if err := daemon.RemoveContainer(ctx, found.ID, true); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return failed(err)
	}

	return kind.Outcome[containerKind.Status]{}, nil
}

// inspected is what a container is doing after a command.
func (n *Node) inspected(ctx context.Context, daemon docker.Daemon, id string) (kind.Outcome[containerKind.Status], error) {
	found, err := daemon.Container(ctx, id)
	if err != nil {
		return failed(err)
	}

	return outcome(found), nil
}

// find is the container a resource is in its VM's dockerd: the one its
// Docker id names, or else the one labelled as the resource, and whether there
// is one at all.
func find(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (docker.Container, bool, error) {
	if seen := c.Status.Docker; seen != nil && len(seen.ID) > 0 {
		found, err := daemon.Container(ctx, seen.ID)

		switch {
		case err == nil:
			return found, true, nil
		case !errors.Is(err, domain.ErrNotExists):
			return docker.Container{}, false, err
		}
	}

	labelled, err := daemon.Containers(ctx, docker.ContainerFilter{All: true, Label: blockKinds.Label(containerKind.Name) + "=" + c.Metadata.UUID})
	if err != nil {
		return docker.Container{}, false, err
	}

	if len(labelled) == 0 {
		return docker.Container{}, false, nil
	}

	found, err := daemon.Container(ctx, labelled[0].ID)
	if errors.Is(err, domain.ErrNotExists) {
		return docker.Container{}, false, nil
	} else if err != nil {
		return docker.Container{}, false, err
	}

	return found, true, nil
}

// outcome is what a command left a container as: what docker says it is
// doing, and no failure.
func outcome(c docker.Container) kind.Outcome[containerKind.Status] {
	if policy, known := containerKind.PolicyOf(c.Labels); known && len(c.RestartPolicy) == 0 {
		c.RestartPolicy = policy
	}

	return kind.Outcome[containerKind.Status]{Status: containerKind.Status{
		Status:  kind.Status{State: containerKind.StateOf(c.State, c.RestartPolicy)},
		Docker:  containerKind.DockerOf(c),
		Failure: blocks.Cleared(),
	}}
}

// failed is what a command that failed left a container as: failed, saying
// why in the codes every side knows.
func failed(err error) (kind.Outcome[containerKind.Status], error) {
	return kind.Outcome[containerKind.Status]{Status: containerKind.Status{
		Status:  kind.Status{State: containerKind.Failed, Reason: err.Error()},
		Failure: blocks.Failure(err),
	}}, err
}

// logs reads a container's log from its end, at most as many lines as a
// reply carries, and then as many of them as fit in one.
func logs(ctx context.Context, daemon docker.Daemon, id string, asked containerKind.LogsPayload) (containerKind.Logs, error) {
	// one line more than a reply carries is how a log that does not fit is
	// told from one that just does.
	options := docker.LogOptions{Since: asked.Since, Tail: asked.Tail}
	if options.Tail == 0 || options.Tail > noderequest.MaxLogLines {
		options.Tail = noderequest.MaxLogLines + 1
	}

	read, err := daemon.ContainerLogs(ctx, id, options)
	if err != nil {
		return containerKind.Logs{}, err
	}

	truncated := false
	if len(read) > noderequest.MaxLogLines {
		read = read[len(read)-noderequest.MaxLogLines:]
		truncated = true
	}

	lines := make([]containerKind.LogLine, len(read))
	for i, line := range read {
		lines[i] = containerKind.LogLine{At: line.At, Stream: line.Stream, Line: line.Line}
	}

	fitted, cut := reply.Last(lines)

	return containerKind.Logs{Lines: fitted, Truncated: truncated || cut}, nil
}
