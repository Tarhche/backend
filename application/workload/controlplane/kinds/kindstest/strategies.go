package kindstest

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Fans is the fan's control-plane strategy. It admits a fan with blades,
// placed on Node, wanted running; asks for whatever makes a fan what it is
// expected to be; and renames one in place.
type Fans struct {
	lock sync.Mutex

	// Node is where fans are placed; none leaves them on no node.
	Node string

	// Slugs are the slugs fans are given, in turn, the last one again once
	// they run out; none gives each fan its name's.
	Slugs []string

	// Failure, when set, is what admitting, reconciling and applying fail
	// with.
	Failure error

	// Intents, when set, is what Reconcile asks for in place of the fan's
	// own decisions.
	Intents func(r Fan) []kind.Intent

	admitted int
}

var _ kind.ControlPlane[Spec, Status] = &Fans{}

func (f *Fans) Admit(_ context.Context, asked Fan) (Fan, domain.ValidationErrors, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.Failure != nil {
		return Fan{}, nil, f.Failure
	}

	if asked.Spec.Blades <= 0 {
		return Fan{}, domain.ValidationErrors{"blades": "required_field"}, nil
	}

	asked.Metadata.Slug = asked.Metadata.Name + "-fan"
	if len(f.Slugs) > 0 {
		asked.Metadata.Slug = f.Slugs[min(f.admitted, len(f.Slugs)-1)]
	}

	f.admitted++

	asked.Metadata.Node = f.Node

	if len(asked.Spec.House) > 0 {
		if _, in := asked.Metadata.Owner(Parent); !in {
			asked.Metadata.Owners = append(asked.Metadata.Owners, kind.Reference{Kind: Parent, UUID: asked.Spec.House})
		}
	}

	asked.Status.State = Pending
	asked.Status.Expected = Running

	return asked, nil, nil
}

// Reconcile makes one that is not there, starts one that is not running and
// stops one that is not wanted running.
func (f *Fans) Reconcile(_ context.Context, r Fan) ([]kind.Intent, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.Failure != nil {
		return nil, f.Failure
	}

	if f.Intents != nil {
		return f.Intents(r), nil
	}

	switch state, expected := r.Status.State, r.Status.Expected; {
	case expected == Running && (state == Pending || state == kind.Missing):
		return []kind.Intent{{Action: "create", Reason: "it is not on its node"}}, nil
	case expected == Running && (state == Stopped || state == kind.Failed):
		return []kind.Intent{{Action: "start", Payload: StartPayload{Speed: 1}, Reason: "it is not running"}}, nil
	case expected == Stopped && state == Running:
		return []kind.Intent{{Action: "stop", Reason: "it is not wanted running"}}, nil
	}

	return nil, nil
}

func (f *Fans) Apply(_ context.Context, r Fan, action string, payload any) (Fan, domain.ValidationErrors, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.Failure != nil {
		return Fan{}, nil, f.Failure
	}

	if action != "rename" {
		return Fan{}, nil, fmt.Errorf("a fan cannot be %s in the control plane", action)
	}

	rename := payload.(RenamePayload)
	if rename.Name == "taken" {
		return Fan{}, domain.ValidationErrors{"name": "invalid_value"}, nil
	}

	r.Metadata.Name = rename.Name
	r.Status.Renames++

	return r, nil, nil
}

// Node is the fan's node strategy: the fans on one node, kept in a map. A
// fan made or started runs at the speed it was asked for, or at one.
type Node struct {
	lock sync.Mutex

	fans map[string]Fan

	// Failure, when set, is what every command fails with, and Blind what
	// reading its fans does.
	Failure error
	Blind   error

	// Unseen are the houses it cannot look inside: the fans in them are
	// neither reported nor said to be gone.
	Unseen []string

	executed []string
}

var _ kind.Node[Spec, Status] = &Node{}

func (n *Node) Execute(_ context.Context, r Fan, action string, payload any) (kind.Outcome[Status], error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	n.executed = append(n.executed, action+" "+r.Metadata.UUID)

	if n.Failure != nil {
		return kind.Outcome[Status]{Output: "it would not " + action}, n.Failure
	}

	if n.fans == nil {
		n.fans = make(map[string]Fan)
	}

	switch action {
	case "create", "start":
		speed := 1
		if start, ok := payload.(StartPayload); ok && start.Speed > 0 {
			speed = start.Speed
		}

		r.Status.State = Running
		r.Status.Speed = speed
	case "stop":
		r.Status.State = Stopped
		r.Status.Speed = 0
	case "delete":
		delete(n.fans, r.Metadata.UUID)

		return kind.Outcome[Status]{Status: Status{Status: kind.Status{State: kind.Deleted}}, Output: "deleted"}, nil
	default:
		return kind.Outcome[Status]{}, fmt.Errorf("a fan cannot be %s on its node", action)
	}

	r.Status.Renames = 0
	n.fans[r.Metadata.UUID] = r

	return kind.Outcome[Status]{Status: Status{Status: kind.Status{State: r.Status.State}, Speed: r.Status.Speed}, Output: action + "ed"}, nil
}

func (n *Node) Query(_ context.Context, r Fan, action string, payload any) (any, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if _, held := n.fans[r.Metadata.UUID]; !held {
		return nil, fmt.Errorf("%w: no fan %q here", domain.ErrNotExists, r.Metadata.UUID)
	}

	logs := payload.(LogsPayload)

	return []string{fmt.Sprintf("the last %d lines of %s", logs.Tail, r.Metadata.Name)}, nil
}

func (n *Node) State(context.Context) (kind.Report[Status], error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.Blind != nil {
		return kind.Report[Status]{}, n.Blind
	}

	report := kind.Report[Status]{Instances: []kind.Observed[Status]{}, Unseen: slices.Clone(n.Unseen)}

	for _, fan := range n.fans {
		if house, in := fan.Metadata.Owner(Parent); in && slices.Contains(n.Unseen, house.UUID) {
			continue
		}

		report.Instances = append(report.Instances, kind.Observed[Status]{
			UUID:   fan.Metadata.UUID,
			Owners: fan.Metadata.Owners,
			Status: Status{Status: kind.Status{State: fan.Status.State}, Speed: fan.Status.Speed},
		})
	}

	slices.SortFunc(report.Instances, func(a, b kind.Observed[Status]) int {
		return cmp.Compare(a.UUID, b.UUID)
	})

	return report, nil
}

// Hold puts a fan on the node as it is, as though it had been made there.
func (n *Node) Hold(r Fan) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.fans == nil {
		n.fans = make(map[string]Fan)
	}

	n.fans[r.Metadata.UUID] = r
}

// Lose takes a fan off the node, as though it had fallen over and gone.
func (n *Node) Lose(uuid string) {
	n.lock.Lock()
	defer n.lock.Unlock()

	delete(n.fans, uuid)
}

// Executed is every command carried out, as its action and its fan's uuid.
func (n *Node) Executed() []string {
	n.lock.Lock()
	defer n.lock.Unlock()

	return slices.Clone(n.executed)
}

// Registry is a control plane's registry with the fan registered in it,
// through the binding every kind is.
func Registry(fans *Fans) *kind.Registry[kind.ControlPlaneBinding] {
	registry := kind.NewRegistry[kind.ControlPlaneBinding]()

	if err := registry.Register(kind.BindControlPlane[Spec, Status](Descriptor(), fans)); err != nil {
		panic(err)
	}

	return registry
}

// NodeRegistry is a node's registry with the fan registered in it.
func NodeRegistry(node *Node) *kind.Registry[kind.NodeBinding] {
	registry := kind.NewRegistry[kind.NodeBinding]()

	if err := registry.Register(kind.BindNode[Spec, Status](Descriptor(), node)); err != nil {
		panic(err)
	}

	return registry
}

// Permissions are the permissions a fan's actions are asked under, as the
// roles page would list them.
type Permissions []permission.Permission

var _ permission.Repository = Permissions{}

// FanPermissions are every permission a fan's actions are asked under.
func FanPermissions() Permissions {
	var all Permissions

	d := Descriptor()

	for _, verb := range []string{"manage", "update", "delete", "show", "logs"} {
		admin, self := d.Permissions(verb)

		all = append(all,
			permission.Permission{Name: verb + " anybody's fans", Value: admin},
			permission.Permission{Name: verb + " one's own fans", Value: self},
		)
	}

	return all
}

func (p Permissions) GetAll(context.Context) []permission.Permission {
	return p
}

func (p Permissions) Get(_ context.Context, values []string) ([]permission.Permission, error) {
	var found []permission.Permission
	for _, each := range p {
		if slices.Contains(values, each.Value) {
			found = append(found, each)
		}
	}

	if len(found) == 0 {
		return nil, errors.New("none of them is a permission")
	}

	return found, nil
}
