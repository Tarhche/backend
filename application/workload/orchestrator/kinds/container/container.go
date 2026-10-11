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
// (blocks.Reader): the platform's by the resource each is, a stack's among
// them, in the kind's words as their restart policies say them, and the rest,
// nobody's, as they are, for the control plane to show beside its records.
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
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
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
			return connected(ctx, daemon, id, connect)
		})
	case containerKind.ActionDisconnect:
		disconnect, _ := payload.(containerKind.DisconnectPayload)

		return n.on(ctx, daemon, c, func(id string) error {
			return disconnected(ctx, daemon, id, disconnect)
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
		Owners: blockKinds.Owners(vmUUID),
		Status: containerKind.Status{
			Status: kind.Status{State: containerKind.StateOf(c.State, c.RestartPolicy)},
			Docker: containerKind.DockerOf(c),
		},
	}
}

// create makes a container that is not there, pulling its image first when
// its VM does not hold it, labelled as the resource it is, with its spec
// beside, and as its stack's service when it is one (labelsOf). One that is
// to be stopped, a stopped stack's service, is made and never started, so
// that it does not run a moment, and its answer says it is stopped, as its
// node does. One that is there already, a create asked again, is what was
// asked for, and is left as it is, but for being started when it is to be
// running, and stopped when it is to be stopped.
func (n *Node) create(ctx context.Context, daemon docker.Daemon, c containerKind.Container) (kind.Outcome[containerKind.Status], error) {
	found, there, err := find(ctx, daemon, c)
	if err != nil {
		return failed(err)
	}

	if !there {
		labels, err := labelsOf(c)
		if err != nil {
			return failed(err)
		}

		spec := c.Spec.Docker(c.Metadata.UUID, labels)
		spec.Stopped = c.Status.Expected == containerKind.Stopped

		if found, err = daemon.CreateContainer(ctx, spec); err != nil {
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

// labelsOf are what a container is labelled with beside what labels it as
// the resource it is: its spec, and, of a stack's, which stack's and which
// of its services it is, as its record says, which only a stack's graph says
// of a container its stack owns.
func labelsOf(c containerKind.Container) (map[string]string, error) {
	spec, err := containerKind.SpecLabel(c.Spec)
	if err != nil {
		return nil, err
	}

	labels := map[string]string{containerKind.LabelSpec: spec}

	for _, key := range []string{stackKind.LabelStack, stackKind.LabelService} {
		if value := c.Metadata.Labels[key]; len(value) > 0 {
			labels[key] = value
		}
	}

	return labels, nil
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

// connected puts the container id names on a network, by its name or its
// Docker id, as connect asks, under the aliases it asks for. One its VM's
// dockerd has on a network of somebody's making already is taken off it, and
// put back on it under those aliases: docker keeps the aliases a container
// was put on a network with, refusing to put a running one on it again, and
// doing nothing for one that is not running, while its spec names the
// network under the aliases asked from then on. What the control plane puts
// on a network docker has it on, but its record does not say so, put on it
// from its VM's terminal say, or by a try of the same connect whose result
// was not heard, is on it as asked so, at the cost of a moment off it. One
// put back on that dockerd then refuses is off it, and the command fails:
// the network its spec names, it is put on it again later, as any container
// off a network its spec names is.
//
// A network every dockerd has (networkKind.Default) takes no aliases, and a
// container on it is on it as it was. Of that, and of a network a container
// was taken off, or not found on, before the connect: one the network is
// there for, and that its VM's dockerd has on it once it answered, which
// docker refuses to put on a network it is on already, is on it, as asked,
// and no failure: what the control plane puts on a network a disconnect
// asked a moment before had not taken it off yet, or which its record had
// not heard it was put on. What dockerd answers of a network or a container
// it does not have, of one not on the network, or when it cannot be asked
// which, fails the command, as it did.
func connected(ctx context.Context, daemon docker.Daemon, id string, connect containerKind.ConnectPayload) error {
	if name, on, read := attachment(ctx, daemon, id, connect.Network); read && on && !networkKind.Default(name) {
		if err := disconnected(ctx, daemon, id, containerKind.DisconnectPayload{Network: name}); err != nil {
			return err
		}
	}

	err := daemon.ConnectNetwork(ctx, connect.Network, id, connect.Aliases)
	if err == nil || errors.Is(err, domain.ErrNotExists) {
		return err
	}

	on, read := onNetwork(ctx, daemon, id, connect.Network)
	if !read || !on {
		return err
	}

	return nil
}

// disconnected takes the container id names off a network, by its name or its
// Docker id, as disconnect asks. One the network is there for, and that its
// VM's dockerd does not have on it once it answered, which docker refuses to
// take off a network it is not on, is off it, as asked, and no failure: what
// the control plane takes off a network its spec names, which the container
// was not put on yet, or could not be put back on. What dockerd answers of a
// network or a container it does not have, of one it still has on the
// network, or when it cannot be asked which, fails the command, as it did.
func disconnected(ctx context.Context, daemon docker.Daemon, id string, disconnect containerKind.DisconnectPayload) error {
	err := daemon.DisconnectNetwork(ctx, disconnect.Network, id, disconnect.Force)
	if err == nil || errors.Is(err, domain.ErrNotExists) {
		return err
	}

	on, read := onNetwork(ctx, daemon, id, disconnect.Network)
	if !read || on {
		return err
	}

	return nil
}

// onNetwork reports whether the container id names is on a network, by its
// name or its Docker id, as its VM's dockerd has the two now, and whether it
// could be told: dockerd has both, and answered.
func onNetwork(ctx context.Context, daemon docker.Daemon, id string, network string) (bool, bool) {
	_, on, read := attachment(ctx, daemon, id, network)

	return on, read
}

// attachment is the name of a network, by its name or its Docker id, as its
// VM's dockerd has it, and whether the container id names is on it, as
// onNetwork reports it.
func attachment(ctx context.Context, daemon docker.Daemon, id string, network string) (string, bool, bool) {
	found, err := daemon.Container(ctx, id)
	if err != nil {
		return "", false, false
	}

	networks, err := daemon.Networks(ctx)
	if err != nil {
		return "", false, false
	}

	at := slices.IndexFunc(networks, func(n docker.Network) bool { return n.Name == network || n.ID == network })
	if at < 0 {
		return "", false, false
	}

	name := networks[at].Name

	return name, slices.Contains(found.Networks, name), true
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
