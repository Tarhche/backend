// Package requirementstest is the kinds a graph is made of, wired the way the
// control plane's own are, over the VMs and building blocks blockstest keeps,
// beside a node that carries out every command it is sent at once and says
// so, as a node does: what the tests of each kind's graph.Kind are written
// against.
package requirementstest

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	controlPlaneContainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/container"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	controlPlaneNetworks "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/network"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements"
	controlPlaneVMs "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	controlPlaneVolumes "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/volume"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Patience is how long a command is waited on, unless a test says otherwise:
// far longer than one its node answers at once takes.
const Patience = 10 * time.Second

// Workload is blockstest's VMs and building blocks, the kinds a graph is
// made of over them, and the node holding all of it.
type Workload struct {
	*blockstest.Workload

	// Node is the node holding every VM and what lives in them, which
	// carries out what it is sent.
	Node *Node

	// Requirements are what the kinds make, ask and remove records through,
	// and Kinds the vm, container, network and volume kinds as a graph needs
	// each, by their names.
	Requirements *requirements.Records
	Kinds        graph.Kinds
}

// New is a workload with one node, alive and roomy, and the VMs options say,
// whose commands are carried out at once.
func New(options ...vmtest.Option) *Workload {
	w := &Workload{Workload: blockstest.New(options...)}

	logger := slog.New(slog.DiscardHandler)

	// whoever waits for what came of a command is told by the handler that
	// takes it onto its record, as in the control plane.
	waiting := waiters.New()
	w.Node = &Node{results: recordResult.NewResourceActedOnHandler(w.Registry, w.Resources, waiting, logger, nil)}

	dispatcher := dispatch.New(w.Resources, w.Node, waiting, nil, dispatch.PollEvery(time.Millisecond))
	admission := admitResource.NewUseCase(w.Registry, w.Resources, dispatcher, logger)

	w.Requirements = requirements.New(requirements.Dependencies{
		Registry:   w.Registry,
		Resources:  w.Resources,
		Dispatcher: dispatcher,
		Admission:  admission,
		Deletion:   deleteResource.NewUseCase(w.Registry, w.Resources, dispatcher),
		Patience:   Patience,
	})

	// a Docker VM a graph needs is made as any Docker VM made for what lives
	// in it is.
	chooser := dockervm.NewChooser(w.Workload.Records, w.Resources, admission, w.VMs, vmtest.DockerDefaults)

	w.Kinds = graph.Kinds{
		vmKind.Name:        controlPlaneVMs.NewGraph(chooser, w.Requirements),
		containerKind.Name: controlPlaneContainers.NewGraph(w.Containers, w.Requirements),
		networkKind.Name:   controlPlaneNetworks.NewGraph(w.Networks, w.Requirements),
		volumeKind.Name:    controlPlaneVolumes.NewGraph(w.Volumes, w.Requirements),
	}

	return w
}

// KeepWaiting keeps a resource of any kind as it is, waiting on its command
// for action, under id, sent a moment ago.
func (w *Workload) KeepWaiting(r kind.Raw, action string, id string) {
	if _, err := w.Memory.Create(context.Background(), resource.Record{Raw: r, Pending: &resource.Pending{Action: action, IDs: []string{id}, SentAt: time.Now()}}); err != nil {
		panic(err)
	}
}

// Node is the node holding every VM and what lives in them: what it is sent
// it carries out at once and says so, as a node does, through the handler the
// control plane hears what came of its commands with, unless a test has it
// say otherwise, or nothing (Answer).
type Node struct {
	results domain.MessageHandler

	lock   sync.Mutex
	sent   []kind.ActOnResource
	answer func(command kind.ActOnResource) (kind.ResourceActedOn, bool)
}

var _ domain.Producer = &Node{}

// Answer has the node say of each command it is sent from now on what answer
// says, or nothing when it says nothing.
func (n *Node) Answer(answer func(command kind.ActOnResource) (kind.ResourceActedOn, bool)) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.answer = answer
}

// Sent is every command the node was sent, in order.
func (n *Node) Sent() []kind.ActOnResource {
	n.lock.Lock()
	defer n.lock.Unlock()

	return append([]kind.ActOnResource(nil), n.sent...)
}

// Say has the node say what came of a command, as it does once it has
// carried out one it said nothing of when it was sent.
func (n *Node) Say(ctx context.Context, result kind.ResourceActedOn) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return n.results.Handle(ctx, encoded)
}

func (n *Node) Produce(ctx context.Context, subject string, payload []byte) error {
	if subject != kind.ActOnResourceName {
		return nil
	}

	var command kind.ActOnResource
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}

	n.lock.Lock()
	n.sent = append(n.sent, command)
	answer := n.answer
	n.lock.Unlock()

	if answer == nil {
		answer = Carry
	}

	result, said := answer(command)
	if !said {
		return nil
	}

	return n.Say(context.WithoutCancel(ctx), result)
}

// Carry is what a node says of a command it carried out: a VM made or
// started runs, as its spec asks; a container made or started runs, as docker
// made it; a network and a volume made are present, as docker has them; and
// what was deleted is gone.
func Carry(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
	result := answered(command)
	result.OK = true

	if command.Action == "delete" {
		return result, true
	}

	var status any

	switch command.Kind {
	case vmKind.Name:
		v, _ := kind.Decode[vmKind.Spec, vmKind.Status](command.Resource)
		config := v.Spec.Config()

		status = vmKind.Status{Status: kind.Status{State: vmKind.Running}, Slug: v.Metadata.Slug, Applied: &config, StartedAt: result.At}
	case containerKind.Name:
		c, _ := kind.Decode[containerKind.Spec, containerKind.Status](command.Resource)

		status = containerKind.Status{
			Status:  kind.Status{State: containerKind.Running},
			Docker:  &containerKind.Docker{ID: "c-" + command.UUID, Name: cmp.Or(c.Spec.Name, "quirky_"+command.UUID), Image: c.Spec.Image, State: "running", Status: "Up Less than a second"},
			Failure: &noderequest.Error{},
		}
	case networkKind.Name:
		n, _ := kind.Decode[networkKind.Spec, networkKind.Status](command.Resource)

		status = networkKind.Status{
			Status:  kind.Status{State: networkKind.Present},
			Docker:  &networkKind.Docker{ID: "n-" + command.UUID, Name: n.Spec.Name, Driver: "bridge", Scope: "local", Containers: []string{}},
			Failure: &noderequest.Error{},
		}
	case volumeKind.Name:
		v, _ := kind.Decode[volumeKind.Spec, volumeKind.Status](command.Resource)

		status = volumeKind.Status{
			Status:  kind.Status{State: volumeKind.Present},
			Docker:  &volumeKind.Docker{Name: v.Spec.Name, Driver: "local"},
			Failure: &noderequest.Error{},
		}
	}

	result.Status, _ = json.Marshal(status)

	return result, true
}

// Fail is what a node says of a command it could not carry out, for
// reason: what it was asked of failed, saying why in the codes every side
// knows, as a building block's node strategy says it.
func Fail(command kind.ActOnResource, reason string) kind.ResourceActedOn {
	result := answered(command)
	result.Reason = reason

	status := map[string]any{"state": kind.Failed, "reason": reason}
	if command.Kind != vmKind.Name {
		status["failure"] = noderequest.Error{Code: noderequest.CodeInvalid, Message: reason}
	}

	result.Status, _ = json.Marshal(status)

	return result
}

// CommandOf is the command a record waits on, as its node was sent it: its
// first try.
func CommandOf(r resource.Record) kind.ActOnResource {
	return kind.ActOnResource{ID: r.Pending.IDs[0], Kind: r.Kind, UUID: r.Metadata.UUID, Action: r.Pending.Action, Node: r.Metadata.Node, Resource: r.Raw}
}

// answered is what a node says of a command before it says how it went.
func answered(command kind.ActOnResource) kind.ResourceActedOn {
	return kind.ResourceActedOn{ID: command.ID, Kind: command.Kind, UUID: command.UUID, Action: command.Action, Node: command.Node, At: time.Now()}
}
