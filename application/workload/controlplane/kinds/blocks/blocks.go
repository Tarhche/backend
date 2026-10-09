// Package blocks is what the control-plane strategies of the building blocks
// of a Docker VM share: the container, image, network and volume kinds',
// beside it in kinds/container, kinds/image, kinds/network and kinds/volume.
//
// What the platform made in a Docker VM is a record of its kind, stored and
// reconciled as any resource is. What a VM's dockerd holds besides, made by a
// stack or from the VM's terminal, is reported every beat as well, each in a
// heartbeat of its own, and each building block's strategy takes what its
// kind's heartbeats say of it (Blocks.Witnessed) and shows it beside its
// records as its extras: a stack's, as part of the stack, and nobody's, as
// unmanaged. A kind whose objects what lives beside them can keep says what
// keeps them (Keeper): an image is the container's that uses it, or the
// stack's. None of it is stored, and none of it is ever reconciled: each
// control plane keeps what the heartbeats it hears say (Sightings), until they
// stop saying it for long enough, and it can be acted on and read, through the
// node holding it, as a record can.
//
// What is found in a Docker VM labelled as the platform's with no record of
// its own, in the first look into a VM whose disk was just restored from a
// snapshot, is taken in again, as its kind says it is read off what was
// observed of it (Kind.Adopt): the restored disk is what the VM holds now,
// and it is what its stored building blocks are reset to. Any other time, it
// is shown as nobody's, which is what a record deleted a moment before its
// node said so would otherwise come back as.
//
// Inside a Docker VM, a building block is named by its Docker id or its name,
// as the dashboard has always named them, and the strategy resolves that to
// the record, or to what nobody keeps a record of (Blocks.Resolve).
package blocks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/dockervm"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// adoptWithin is how long after a Docker VM's disk was restored what is
	// first found in it labelled as the platform's is taken in again: the
	// restored VM's dockerd has to come up first, which nested takes minutes.
	adoptWithin = 15 * time.Minute

	// prefixAtLeast is how much of a Docker id names one, as docker reads a
	// short id: at least this much, and the only id beginning so.
	prefixAtLeast = 4
)

// Kind is what the shared code is told of one of the building blocks.
type Kind interface {
	// Adopt is what an object labelled as the platform's, found with no
	// record in a Docker VM whose disk was just restored, is taken in as,
	// read off what was observed of it, or false when it cannot be told.
	Adopt(observed kind.Observation) (Adoption, bool)

	// Named reports whether name names r inside its VM otherwise than by
	// its uuid, its Docker id or its name: a container by the name it was
	// asked for with, an image by any reference to it.
	Named(r kind.Raw, name string) bool

	// Refuse is why something of the kind nobody keeps a record of is not
	// asked for action as it is, as its node would refuse it, or nothing: a
	// container that runs is not removed but by force.
	Refuse(ctx context.Context, r kind.Raw, action string, payload any) error
}

// Keeper is a building block that what lives beside it in its Docker VM can
// keep with no record of its own: an image, which the containers made from it
// keep.
type Keeper interface {
	// Keepers are what keeps each of the kind's objects on a node that has
	// no record of its own, by its uuid, as LabelManagedBy says it. What
	// nothing keeps is not among them.
	Keepers(ctx context.Context, nodeName string) (map[string]string, error)
}

// Adoption is what an object found with no record is taken in as.
type Adoption struct {
	Name     string
	Spec     json.RawMessage
	Expected kind.State
}

// Dependencies are what the shared code reads and asks.
type Dependencies struct {
	// Resources are the records of every kind, deleting what lives in one
	// with it.
	Resources resource.Repository

	// VMs are the Docker VMs, the vm kind's records.
	VMs *records.Records

	// Chooser chooses the Docker VM a building block goes into.
	Chooser *dockervm.Chooser

	// Requester asks the nodes: what nobody keeps a record of is asked of
	// its node at once.
	Requester noderequest.Requester

	// Sightings are what the nodes reported that nobody keeps a record of,
	// shared by every building block.
	Sightings *Sightings

	Logger *slog.Logger

	// Now is the time the strategies go by; nothing is the time now.
	Now func() time.Time
}

// Blocks is what one building block's control-plane strategy shares with the
// others: its extras, what it hears its reports say, and how it names what
// lives in a VM.
type Blocks struct {
	Dependencies

	descriptor kind.Descriptor
	kind       Kind

	lock sync.Mutex

	// adopted is the first look into each Docker VM since its disk was last
	// restored, by the VM: what was found in it labelled as the platform's
	// was taken in again then.
	adopted map[string]look

	// keeping is what keeps what each node holds of the kind that has no
	// record of its own, by the node, as the last beat it was worked out for
	// found.
	keeping map[string]keepersAt
}

// look is the first look into a Docker VM since its disk was restored: the
// beat whose heartbeats said what it held then.
type look struct {
	restoredAt time.Time
	at         time.Time
}

// keepersAt are what keeps what of a kind a node holds that has no record of
// its own, by its uuid, as worked out for the beat at a moment.
type keepersAt struct {
	at      time.Time
	keepers map[string]string
}

var (
	_ kind.Extras   = &Blocks{}
	_ kind.Extender = &Blocks{}
	_ kind.Witness  = &Blocks{}
	_ kind.Resolver = &Blocks{}
)

// New is what the building block d describes shares, told of it as k says.
func New(d kind.Descriptor, k Kind, dependencies Dependencies) *Blocks {
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}

	if dependencies.Logger == nil {
		dependencies.Logger = slog.New(slog.DiscardHandler)
	}

	return &Blocks{Dependencies: dependencies, descriptor: d, kind: k, adopted: make(map[string]look), keeping: make(map[string]keepersAt)}
}

// Descriptor is the kind's.
func (b *Blocks) Descriptor() kind.Descriptor {
	return b.descriptor
}

// Extras are what nobody keeps a record of: its own.
func (b *Blocks) Extras() kind.Extras {
	return b
}

// All is everything of the kind a node reported lately that nobody keeps a
// record of, newest first.
func (b *Blocks) All(context.Context) ([]kind.Raw, error) {
	return b.Sightings.All(b.descriptor.Name), nil
}

// One is what nobody keeps a record of that uuid names, or
// domain.ErrNotExists.
func (b *Blocks) One(_ context.Context, uuid string) (kind.Raw, error) {
	r, seen := b.Sightings.One(b.descriptor.Name, uuid)
	if !seen {
		return kind.Raw{}, domain.ErrNotExists
	}

	return r, nil
}

// Act asks the node holding something nobody keeps a record of for one of
// the kind's commands, at once, and is what it left it as, or gone. What the
// state it is in does not allow is refused as a record's is, and so is what
// the kind refuses; what its node refused is the node's refusal, in the codes
// every side knows.
func (b *Blocks) Act(ctx context.Context, r kind.Raw, action string, payload []byte) (kind.Raw, bool, domain.ValidationErrors, error) {
	a, found := b.descriptor.Action(action)
	if !found || a.Internal || a.Mode != kind.ModeCommand || a.Runs != kind.OnNode {
		return kind.Raw{}, false, nil, fmt.Errorf("%w: a %s has no command %q", kind.ErrUnknownAction, b.descriptor.Name, action)
	}

	if !b.descriptor.Allows(action, ObservedOf(r.Status).State) {
		return kind.Raw{}, false, domain.ValidationErrors{"action": "invalid_state_transition"}, nil
	}

	value, invalid, err := a.Payload.Decode(payload)
	if err != nil || len(invalid) > 0 {
		return kind.Raw{}, false, invalid, err
	}

	if err := b.kind.Refuse(ctx, r, action, value); err != nil {
		return kind.Raw{}, false, nil, err
	}

	compacted, err := dispatch.Compact(payload)
	if err != nil {
		return kind.Raw{}, false, nil, err
	}

	result, err := b.ask(ctx, r, action, compacted)
	if err != nil {
		return kind.Raw{}, false, nil, err
	}

	if !result.OK {
		return kind.Raw{}, false, nil, Refusal(result.Status, result.Reason)
	}

	if a.Desires == kind.Deleted {
		b.Sightings.Changed(b.descriptor.Name, r.Metadata.UUID, nil)

		return kind.Raw{}, true, nil, nil
	}

	after := r
	if len(result.Status) > 0 {
		after.Status = result.Status
	}

	b.Sightings.Changed(b.descriptor.Name, r.Metadata.UUID, &after)

	return after, false, nil, nil
}

// Query asks the node holding something nobody keeps a record of one of the
// kind's queries, and is its answer. One the state it is in does not allow,
// and what its node refused, are the node's refusal.
func (b *Blocks) Query(ctx context.Context, r kind.Raw, action string, payload []byte) ([]byte, domain.ValidationErrors, error) {
	a, found := b.descriptor.Action(action)
	if !found || a.Mode != kind.ModeQuery || a.Runs != kind.OnNode {
		return nil, nil, fmt.Errorf("%w: a %s has no query %q", kind.ErrUnknownAction, b.descriptor.Name, action)
	}

	if state := ObservedOf(r.Status).State; !b.descriptor.Allows(action, state) {
		return nil, nil, &noderequest.Error{Code: noderequest.CodeNotRunning, Message: fmt.Sprintf("a %s that is %s cannot be asked for its %s", b.descriptor.Name, state, action)}
	}

	if _, invalid, err := a.Payload.Decode(payload); err != nil || len(invalid) > 0 {
		return nil, invalid, err
	}

	compacted, err := dispatch.Compact(payload)
	if err != nil {
		return nil, nil, err
	}

	reply, err := b.request(ctx, r, action, compacted)
	if err != nil {
		return nil, nil, err
	}

	return reply.Result, nil, nil
}

// ask has the node holding r carry out a command on it at once, and is what
// came of it.
func (b *Blocks) ask(ctx context.Context, r kind.Raw, action string, payload json.RawMessage) (kind.ResourceActedOn, error) {
	reply, err := b.request(ctx, r, action, payload)
	if err != nil {
		return kind.ResourceActedOn{}, err
	}

	var result kind.ResourceActedOn
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		return kind.ResourceActedOn{}, &noderequest.Error{Code: noderequest.CodeInternal, Message: "the node answered with something that is not what came of the command"}
	}

	return result, nil
}

// request asks the node holding r one of the kind's actions, and is its
// reply, or why there is none, as the node's refusal.
func (b *Blocks) request(ctx context.Context, r kind.Raw, action string, payload json.RawMessage) (noderequest.Reply, error) {
	if len(r.Metadata.Node) == 0 {
		return noderequest.Reply{}, &noderequest.Error{Code: noderequest.CodeNotRunning, Message: fmt.Sprintf("the %s is on no node", b.descriptor.Name)}
	}

	asked, err := kind.Query{Kind: b.descriptor.Name, UUID: r.Metadata.UUID, Action: action, Payload: payload, Resource: r}.Request()
	if err != nil {
		return noderequest.Reply{}, err
	}

	reply, err := b.Requester.Request(ctx, r.Metadata.Node, asked)

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return noderequest.Reply{}, &noderequest.Error{Code: noderequest.CodeTimeout, Message: fmt.Sprintf("the node holding the %s did not answer in time", b.descriptor.Name)}
	case err != nil:
		return noderequest.Reply{}, &noderequest.Error{Code: noderequest.CodeInternal, Message: fmt.Sprintf("the node holding the %s is not answering", b.descriptor.Name)}
	}

	var refused *noderequest.Error
	if errors.As(reply.Err(), &refused) {
		return noderequest.Reply{}, refused
	}

	return reply, nil
}

// Witnessed takes in what a node's heartbeat at a moment said of a thing of
// the kind in a Docker VM: one a record speaks for is shown as the record
// alone, and one nobody keeps a record of is shown as it was seen from now on,
// until a later heartbeat says otherwise, or until none has said it for long
// enough (Sightings). What it found labelled as the platform's with no record,
// in the first look into a VM whose disk was just restored, is taken in again
// instead.
func (b *Blocks) Witnessed(ctx context.Context, nodeName string, instance kind.Observation, kept bool, at time.Time) error {
	if kept {
		b.Sightings.Forget(b.descriptor.Name, instance.UUID)

		return nil
	}

	parent, in := ownerOf(instance.Owners, blockKinds.Parent)
	if !in {
		return nil
	}

	v, err := b.VMs.GetOne(ctx, parent.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		// a VM nobody keeps a record of, on its way out.
		return nil
	case err != nil:
		return err
	case v.Metadata.Node != nodeName:
		// its record says it is elsewhere: that node's to speak for.
		return nil
	}

	var failed error

	if len(instance.UUID) > 0 && b.adoptable(v, at) {
		adopted, err := b.adopt(ctx, nodeName, v, instance, at)
		if adopted {
			b.Sightings.Forget(b.descriptor.Name, instance.UUID)

			return nil
		}

		failed = err
	}

	keepers, err := b.keepers(ctx, nodeName, at)
	if err != nil {
		return errors.Join(failed, err)
	}

	b.Sightings.Saw(b.descriptor.Name, Seen{Node: nodeName, At: at, Manifest: b.sighted(nodeName, v, instance, keepers)})

	return failed
}

// adoptable reports whether what is found in v labelled as the platform's,
// with no record, is taken in again at a moment: in the first look into it
// since its disk was restored, not long ago, which is every heartbeat of the
// beat that first said what it held.
func (b *Blocks) adoptable(v vmKind.VM, at time.Time) bool {
	restoredAt := v.Status.RestoredAt
	if restoredAt.IsZero() || at.Before(restoredAt) || at.Sub(restoredAt) > adoptWithin {
		return false
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	first, looked := b.adopted[v.Metadata.UUID]
	if !looked || !first.restoredAt.Equal(restoredAt) {
		b.adopted[v.Metadata.UUID] = look{restoredAt: restoredAt, at: at}

		return true
	}

	return first.at.Equal(at)
}

// adopt takes in what was found labelled as the platform's in a restored VM,
// as the kind reads it off what was observed of it, and reports whether it
// did. One another control plane took in first is taken in.
func (b *Blocks) adopt(ctx context.Context, nodeName string, v vmKind.VM, instance kind.Observation, at time.Time) (bool, error) {
	adoption, ok := b.kind.Adopt(instance)
	if !ok {
		return false, nil
	}

	common, err := resource.Common(instance.Status)
	if err != nil {
		return false, err
	}

	common.Expected = adoption.Expected
	common.Since = at
	common.ObservedAt = at

	status, err := resource.WithCommon(instance.Status, common)
	if err != nil {
		return false, err
	}

	now := b.Now()

	_, err = b.Resources.Create(ctx, resource.Record{Raw: kind.Raw{
		Kind: b.descriptor.Name,
		Metadata: kind.Metadata{
			UUID:      instance.UUID,
			Name:      adoption.Name,
			OwnerUUID: v.Metadata.OwnerUUID,
			Owners:    []kind.Reference{{Kind: blockKinds.Parent, UUID: v.Metadata.UUID}},
			Node:      nodeName,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Spec:   adoption.Spec,
		Status: status,
	}})

	switch {
	case err == nil:
		b.Logger.InfoContext(ctx, "took in what a restored vm holds labelled as the platform's", "kind", b.descriptor.Name, "uuid", instance.UUID, "vm", v.Metadata.UUID)

		return true, nil
	case errors.Is(err, domain.ErrAlreadyExists):
		return true, nil
	}

	return false, err
}

// keepers are what keeps what of the kind on a node has no record of its own,
// by uuid, when the kind says what lives beside it can keep it: worked out
// once a beat, for every heartbeat of it.
func (b *Blocks) keepers(ctx context.Context, nodeName string, at time.Time) (map[string]string, error) {
	keeper, keeps := b.kind.(Keeper)
	if !keeps {
		return nil, nil
	}

	b.lock.Lock()
	worked, known := b.keeping[nodeName]
	b.lock.Unlock()

	if known && worked.at.Equal(at) {
		return worked.keepers, nil
	}

	keepers, err := keeper.Keepers(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	b.lock.Lock()
	b.keeping[nodeName] = keepersAt{at: at, keepers: keepers}
	b.lock.Unlock()

	return keepers, nil
}

// sighted is the manifest of what nobody keeps a record of, as a node saw
// it in v: a stack's, what keepers say keeps it, or nobody's, its VM's owner's,
// under a uuid its Docker id gives it when no label does.
func (b *Blocks) sighted(nodeName string, v vmKind.VM, instance kind.Observation, keepers map[string]string) kind.Raw {
	observed := ObservedOf(instance.Status)

	uuid := instance.UUID
	if len(uuid) == 0 {
		uuid = blockKinds.Derived(b.descriptor.Name, v.Metadata.UUID, cmpOr(observed.Docker.ID, observed.Docker.Name))
	}

	managedBy := blockKinds.ManagedByNobody

	_, stacked := ownerOf(instance.Owners, stackKind.Name)

	switch keeper := keepers[uuid]; {
	case stacked, len(observed.Docker.Labels[docker.LabelComposeProject]) > 0:
		managedBy = blockKinds.ManagedByStack
	case len(keeper) > 0:
		managedBy = keeper
	}

	return kind.Raw{
		Kind: b.descriptor.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      cmpOr(observed.Docker.Name, observed.Docker.Reference),
			OwnerUUID: v.Metadata.OwnerUUID,
			Labels:    map[string]string{blockKinds.LabelManagedBy: managedBy},
			Owners:    slices.Clone(instance.Owners),
			Node:      nodeName,
			CreatedAt: observed.Docker.CreatedAt,
		},
		Status: instance.Status,
	}
}

// Resolve is the uuid of what name names inside a Docker VM: one of the
// kind's records, or something nobody keeps a record of, by its uuid, its
// Docker id, its name or whatever else the kind names it by, or the one Docker
// id beginning so.
func (b *Blocks) Resolve(ctx context.Context, parent kind.Reference, name string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) == 0 {
		return "", domain.ErrNotExists
	}

	recorded, _, err := b.Resources.GetAll(ctx, b.descriptor.Name, resource.Filter{Parent: parent}, 0, 0)
	if err != nil {
		return "", err
	}

	candidates := make([]kind.Raw, 0, len(recorded))
	for i := range recorded {
		candidates = append(candidates, recorded[i].Raw)
	}

	candidates = append(candidates, b.Sightings.In(b.descriptor.Name, parent.UUID)...)

	// what names one surely, before what may name several.
	for _, named := range []func(r kind.Raw, name string) bool{
		func(r kind.Raw, name string) bool { return r.Metadata.UUID == name },
		func(r kind.Raw, name string) bool {
			id := ObservedOf(r.Status).Docker.ID
			return len(id) > 0 && (id == name || strings.TrimPrefix(id, "sha256:") == name)
		},
		func(r kind.Raw, name string) bool { return ObservedOf(r.Status).Docker.Name == name },
		b.kind.Named,
	} {
		for _, r := range candidates {
			if named(r, name) {
				return r.Metadata.UUID, nil
			}
		}
	}

	if len(name) < prefixAtLeast {
		return "", domain.ErrNotExists
	}

	var prefixed []string

	for _, r := range candidates {
		id := strings.TrimPrefix(ObservedOf(r.Status).Docker.ID, "sha256:")
		if strings.HasPrefix(id, strings.TrimPrefix(name, "sha256:")) && !slices.Contains(prefixed, r.Metadata.UUID) {
			prefixed = append(prefixed, r.Metadata.UUID)
		}
	}

	if len(prefixed) == 1 {
		return prefixed[0], nil
	}

	return "", domain.ErrNotExists
}

// Into is the Docker VM a building block owned by ownerUUID is to live in,
// the one parent names, or why it cannot: one that is not theirs, not a
// Docker VM, or not running or on its way up.
func (b *Blocks) Into(ctx context.Context, ownerUUID string, parent string) (vmKind.VM, domain.ValidationErrors, error) {
	if len(parent) == 0 {
		return vmKind.VM{}, domain.ValidationErrors{"vm": "required_field"}, nil
	}

	chosen, refused, err := b.Chooser.Choose(ctx, ownerUUID, dockervm.Choice{UUID: parent})
	if err != nil || len(refused) > 0 {
		return vmKind.VM{}, refused, err
	}

	if !vmKind.Up(chosen.VM) {
		return vmKind.VM{}, domain.ValidationErrors{"vm": "not_running"}, nil
	}

	return chosen.VM, nil, nil
}

// Present is what a building block that is either there or not is asked
// for, given what it was asked to be and what it was last seen being: made,
// when it is not there and its VM runs.
func (b *Blocks) Present(ctx context.Context, state kind.State, expected kind.State, vmUUID string) ([]kind.Intent, error) {
	if expected != blockKinds.Present || (state != blockKinds.Pending && state != kind.Missing) {
		return nil, nil
	}

	running, err := b.Running(ctx, vmUUID)
	if err != nil || !running {
		return nil, err
	}

	return []kind.Intent{{Action: blockKinds.ActionCreate, Reason: "it is not in its vm, which runs"}}, nil
}

// Running reports whether the Docker VM uuid names runs: what lives in one
// that does not waits on it, and one that is gone takes what lived in it
// with it.
func (b *Blocks) Running(ctx context.Context, vmUUID string) (bool, error) {
	v, err := b.VMs.GetOne(ctx, vmUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return v.Status.State == vmKind.Running, nil
}

// Refused is a refusal in the words a node would refuse with, which is what
// the dashboard shows of a request docker would have turned down.
func Refused(format string, args ...any) error {
	return &noderequest.Error{Code: noderequest.CodeInvalid, Message: fmt.Sprintf(format, args...)}
}

// ownerOf is what of the named kind owners has, if it has one.
func ownerOf(owners []kind.Reference, kindName string) (kind.Reference, bool) {
	for _, owner := range owners {
		if owner.Kind == kindName {
			return owner, true
		}
	}

	return kind.Reference{}, false
}

func cmpOr(values ...string) string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}

	return ""
}
