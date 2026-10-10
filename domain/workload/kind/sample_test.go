package kind

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// A box is the kind these tests are about: a small thing a node runs inside
// a VM, started, stopped, resized and read. It is small enough to read whole
// and has one of everything the framework handles: a parent, an internal
// command, a control-plane command, a query with a payload, a stream, and
// the states the framework moves resources into itself.

type boxSpec struct {
	Image string `json:"image"`
	Size  int    `json:"size,omitempty"`
}

type boxStatus struct {
	Status

	Uptime int `json:"uptime,omitempty"`
}

// resizePayload is valid by value, and logsPayload by pointer, as the
// repository's requests are.
type resizePayload struct {
	Size int `json:"size"`
}

func (p resizePayload) Validate() domain.ValidationErrors {
	if p.Size <= 0 {
		return domain.ValidationErrors{"size": "required_field"}
	}

	return nil
}

type logsPayload struct {
	Tail uint `json:"tail"`
}

func (p *logsPayload) Validate() domain.ValidationErrors {
	if p.Tail > 1000 {
		return domain.ValidationErrors{"tail": "too_many"}
	}

	return nil
}

const (
	boxPending  State = "pending"
	boxStarting State = "starting"
	boxRunning  State = "running"
	boxStopping State = "stopping"
	boxStopped  State = "stopped"
	boxDeleting State = "deleting"
)

// box is the box's descriptor, a new one every time, so a test can break it
// without breaking it for another.
func box() Descriptor {
	return Descriptor{
		Name:      "box",
		Plural:    "boxes",
		StateBy:   OnNode,
		Parent:    "vm",
		OnParent:  ParentRules{Delete: CascadeDelete, Restore: CascadeReset},
		Endpoints: true,
		Machine:   boxMachine(),
		Actions: []Action{
			{Name: "create", Runs: OnNode, Mode: ModeCommand, AllowedIn: []State{boxPending, Missing}, Desires: boxRunning, Internal: true, Payload: NoPayload},
			{Name: "start", Runs: OnNode, Mode: ModeCommand, AllowedIn: []State{boxStopped, Failed}, Desires: boxRunning, Permission: "manage", Payload: NoPayload},
			{Name: "stop", Runs: OnNode, Mode: ModeCommand, AllowedIn: []State{boxRunning}, Desires: boxStopped, Permission: "manage", Payload: NoPayload},
			{Name: "resize", Runs: OnControlPlane, Mode: ModeCommand, Permission: "update", Payload: Payload[resizePayload]()},
			{Name: "delete", Runs: OnNode, Mode: ModeCommand, Desires: Deleted, Permission: "delete", Payload: NoPayload},
			{Name: "state", Runs: OnNode, Mode: ModeQuery, Permission: "show", Payload: NoPayload},
			{Name: "logs", Runs: OnNode, Mode: ModeQuery, Permission: "logs", Payload: Payload[logsPayload]()},
			{Name: "attach", Runs: OnNode, Mode: ModeStream, AllowedIn: []State{boxRunning}, Permission: "attach", Payload: NoPayload},
		},
	}
}

func boxMachine() Machine {
	return Machine{
		Initial: boxPending,
		States:  []State{boxPending, boxStarting, boxRunning, boxStopping, boxStopped, Failed, Missing, boxDeleting, Deleted},
		Transitions: []Transition{
			{From: boxPending, On: OnAction("create"), To: boxStarting},
			{From: Missing, On: OnAction("create"), To: boxStarting},
			{From: boxStarting, On: OnObserved(boxRunning), To: boxRunning},
			{From: boxStopped, On: OnAction("start"), To: boxStarting},
			{From: Failed, On: OnAction("start"), To: boxStarting},
			{From: boxRunning, On: OnAction("stop"), To: boxStopping},
			{From: boxStopping, On: OnObserved(boxStopped), To: boxStopped},
			{From: boxRunning, On: OnObserved(Missing), To: Missing},
			{From: Any, On: OnObserved(Failed), To: Failed},
			{From: Any, On: OnAction("delete"), To: boxDeleting},
			{From: boxDeleting, On: OnObserved(Missing), To: Deleted},
			{From: boxDeleting, On: OnObserved(Deleted), To: Deleted},
		},
		Terminal: []State{boxStopped, Failed, Deleted},
		InFlight: []State{boxPending, boxStarting, boxStopping, boxDeleting},
	}
}

// aBox is one box, as the control plane records it.
func aBox() Resource[boxSpec, boxStatus] {
	return Resource[boxSpec, boxStatus]{
		Kind: "box",
		Metadata: Metadata{
			UUID:      "box-uuid",
			Name:      "shop",
			Slug:      "shop-abcde",
			OwnerUUID: "owner-uuid",
			Labels:    map[string]string{"team": "web"},
			Owners:    []Reference{{Kind: "vm", UUID: "vm-uuid"}},
			Node:      "node-1",
		},
		Spec:   boxSpec{Image: "nginx:alpine", Size: 2},
		Status: boxStatus{Status: Status{State: boxRunning, Expected: boxRunning}, Uptime: 42},
	}
}

// boxControlPlane is a box's control-plane strategy: it admits a box with an
// image, starts one that stopped while it was wanted running, and resizes
// one up to ten.
type boxControlPlane struct {
	failure error

	// asked is what it was last asked to admit.
	asked Resource[boxSpec, boxStatus]
}

var _ ControlPlane[boxSpec, boxStatus] = &boxControlPlane{}

func (b *boxControlPlane) Admit(_ context.Context, asked Resource[boxSpec, boxStatus]) (Resource[boxSpec, boxStatus], domain.ValidationErrors, error) {
	b.asked = asked

	if b.failure != nil {
		return Resource[boxSpec, boxStatus]{}, nil, b.failure
	}

	if len(asked.Spec.Image) == 0 {
		return Resource[boxSpec, boxStatus]{}, domain.ValidationErrors{"image": "required_field"}, nil
	}

	asked.Metadata.UUID = "box-uuid"
	asked.Metadata.Slug = asked.Metadata.Name + "-abcde"
	asked.Metadata.Node = "node-1"
	asked.Status.State = boxPending
	asked.Status.Expected = boxRunning

	return asked, nil, nil
}

func (b *boxControlPlane) Reconcile(_ context.Context, r Resource[boxSpec, boxStatus]) ([]Intent, error) {
	if b.failure != nil {
		return nil, b.failure
	}

	if r.Status.Expected == boxRunning && r.Status.State == boxStopped {
		return []Intent{{Action: "start", Reason: "it stopped while it was wanted running"}}, nil
	}

	return nil, nil
}

func (b *boxControlPlane) Apply(_ context.Context, r Resource[boxSpec, boxStatus], action string, payload any) (Resource[boxSpec, boxStatus], domain.ValidationErrors, error) {
	if action != "resize" {
		return Resource[boxSpec, boxStatus]{}, nil, fmt.Errorf("a box cannot be %sd here", action)
	}

	resize := payload.(resizePayload)
	if resize.Size > 10 {
		return Resource[boxSpec, boxStatus]{}, domain.ValidationErrors{"size": "too_big"}, nil
	}

	r.Spec.Size = resize.Size

	return r, nil, nil
}

// boxNode is a box's node strategy: it does what it is told to, and
// remembers what it was asked.
type boxNode struct {
	outcome Outcome[boxStatus]
	failure error

	report Report[boxStatus]
	blind  error

	executed []string
	payloads []any
}

var _ Node[boxSpec, boxStatus] = &boxNode{}

func (n *boxNode) Execute(_ context.Context, r Resource[boxSpec, boxStatus], action string, payload any) (Outcome[boxStatus], error) {
	n.executed = append(n.executed, action+" "+r.Metadata.UUID)
	n.payloads = append(n.payloads, payload)

	return n.outcome, n.failure
}

func (n *boxNode) Query(_ context.Context, r Resource[boxSpec, boxStatus], action string, payload any) (any, error) {
	if n.failure != nil {
		return nil, n.failure
	}

	return []string{fmt.Sprintf("%s of %s: the last %d lines", action, r.Metadata.UUID, payload.(logsPayload).Tail)}, nil
}

func (n *boxNode) State(context.Context) (Report[boxStatus], error) {
	return n.report, n.blind
}

// attachingBoxNode is a box's node strategy that serves streams too, to
// the box's owner alone, and its port.
type attachingBoxNode struct {
	boxNode

	session Session
}

var (
	_ Attacher = &attachingBoxNode{}
	_ Exposer  = &attachingBoxNode{}
)

func (n *attachingBoxNode) Attach(_ context.Context, action string, uuid string, owner string) (Session, error) {
	if uuid != "box-uuid" || owner != "owner-uuid" {
		return nil, fmt.Errorf("%w: no box %q of theirs", domain.ErrNotExists, uuid)
	}

	return n.session, nil
}

func (n *attachingBoxNode) Endpoint(_ context.Context, slug string, p port.Port) (Endpoint, error) {
	if slug != "shop-abcde" || (p != 0 && p != 8080) {
		return Endpoint{}, fmt.Errorf("%w: no port %d of %q", domain.ErrNotExists, p, slug)
	}

	return Endpoint{Port: 8080, Address: "vmhost-01:20000"}, nil
}

// boxIngress finds the one box there is.
type boxIngress struct{}

var _ Ingress = boxIngress{}

func (boxIngress) ByUUID(_ context.Context, uuid string) (Location, error) {
	if uuid != "box-uuid" {
		return Location{}, domain.ErrNotExists
	}

	return Location{UUID: uuid, Node: "node-1", Ports: []port.Port{8080}}, nil
}

func (boxIngress) BySlug(_ context.Context, slug string) (Location, error) {
	if slug != "shop-abcde" {
		return Location{}, domain.ErrNotExists
	}

	return Location{UUID: "box-uuid", Node: "node-1", Ports: []port.Port{8080}}, nil
}

// terminal is a session that has said all it will.
type terminal struct {
	output bytes.Buffer
}

var _ Session = &terminal{}

func (t *terminal) Stdin() io.WriteCloser                        { return nopWriteCloser{&t.output} }
func (t *terminal) Stdout() io.Reader                            { return &t.output }
func (t *terminal) Stderr() io.Reader                            { return &bytes.Buffer{} }
func (t *terminal) Resize(context.Context, uint, uint) error     { return nil }
func (t *terminal) Wait(context.Context) (exitCode int, _ error) { return 0, nil }
func (t *terminal) Close() error                                 { return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// boxServices are every service's registry, with a box registered in each.
func boxServices(node NodeBinding) Services {
	controlPlane := NewRegistry[ControlPlaneBinding]()
	nodes := NewRegistry[NodeBinding]()
	ingress := NewRegistry[IngressBinding]()

	for _, err := range []error{
		controlPlane.Register(BindControlPlane[boxSpec, boxStatus](box(), &boxControlPlane{})),
		nodes.Register(node),
		ingress.Register(BindIngress(box(), boxIngress{})),
	} {
		if err != nil {
			panic(err)
		}
	}

	return Services{ControlPlane: controlPlane, Node: nodes, Ingress: ingress}
}
