// Package vm is the vm kind's control-plane strategy: what a VM is admitted
// as, what it is asked for when what it is doing is not what it was asked to
// be, and what is changed of it in the control plane.
//
// A VM is a machine or a Docker VM as its image says, read with the Docker
// image the control plane is given (vm.KindOf), and nothing else says it: its
// bounds, its owner's Docker VMs, what may live in it and the snapshots it
// may be restored from all go by its image.
//
// A VM is admitted with what it asked for checked and what it left out filled
// in: the default image for a machine that names none, a network open both
// ways, and, when it is made from a snapshot of its owner's that the snapshot
// kind says it can be made from, that snapshot's image and disk. A Docker VM,
// one asked to boot the Docker image's repository, boots the Docker image
// itself and nothing else. It is
// held to what one VM may be given and to what its owner's VMs may be given
// between them, given a slug nothing else holds, and placed on the node
// with the most room for it. One no node has room for is kept failed, as
// no_capacity, and given up on until somebody starts it, which places it
// then; what is refused is said under the fields the dashboard asks with.
//
// What it is asked for after that is decided from what its node last said of
// it:
//
//	expected  observed                          asked
//	stopped   running                           stop
//	running   created                           create
//	running   stopped, which its node lost too  start, which makes it again
//	running   failed                            start (with the loop's backoff)
//	any       running or stopped, its config    reconfigure
//	          not what its node last applied
//
// A VM whose node fell silent is failed as node_lost, one that outlived its
// lifetime is deleted, and one expected deleted is asked for its delete, by
// the reconcile loop, which does that for every kind alike.
//
// Its name and its lifetime are changed in the control plane (ActionUpdate),
// and so are its ports, network and resources, held to its bounds, its
// owner's quota and its node's room: that its node has not applied them is
// what has it reconfigured, at once. A restore is held, before it is sent, to
// what the snapshot kind says of its snapshot (snapshotKind.Restores): of its
// owner's, ready, taken of a VM of its kind, no larger than its disk and from
// its node's engine; and the VM to being on a node that can be asked.
//
// Beside its records, a listing of anybody's VMs has the code runner's runs,
// its extras, and a listing of VMs may be narrowed to the Docker VMs or to the
// machines, as their images say (kind.Narrower).
package vm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// ReasonNoCapacity is why a VM no node had room for is failed.
const ReasonNoCapacity = "no_capacity"

// Images are what VMs boot from: a machine that names none, and every Docker
// VM, which is what its image makes it.
type Images struct {
	// Machine is a machine's when it names none.
	Machine string

	// Docker is every Docker VM's: a docker-in-docker image, the same one the
	// nodes are given. A VM is a Docker VM when its image is this one, or
	// another tag of it (vm.KindOf).
	Docker string
}

// Dependencies are what the strategy reads and holds VMs to.
type Dependencies struct {
	Records *records.Records

	// Snapshots say whether a snapshot can be restored onto a VM, or a VM
	// made from one, which is the snapshot kind's to say.
	Snapshots snapshotKind.Restores

	Nodes     node.Repository
	Quota     *quota.Quota
	Placement *placement.Placement

	// Slugs say which slugs are held already, by a resource of any kind that
	// has one: a slug is one resource's, whatever its kind, and a VM's is
	// what the ingress serves its ports under, as a task's is.
	Slugs []slugs.Taken

	Images Images

	// Extras are what a listing of anybody's VMs has beside their records:
	// the code runner's runs. None is nothing.
	Extras kind.Extras

	// Now is the time the strategy goes by; nothing is the time now.
	Now func() time.Time
}

// VMs is the vm kind's control-plane strategy.
type VMs struct {
	Dependencies
}

var (
	_ kind.ControlPlane[vmKind.Spec, vmKind.Status] = &VMs{}
	_ kind.Preparer[vmKind.Spec, vmKind.Status]     = &VMs{}
	_ kind.Extender                                 = &VMs{}
	_ kind.Narrower                                 = &VMs{}
)

func New(d Dependencies) *VMs {
	if d.Now == nil {
		d.Now = time.Now
	}

	return &VMs{Dependencies: d}
}

// Admit takes in a VM its owner asked for: checked, its defaults filled in,
// held to its bounds and its owner's quota, given a slug and placed.
func (s *VMs) Admit(ctx context.Context, asked vmKind.VM) (vmKind.VM, domain.ValidationErrors, error) {
	if invalid := validate(asked); len(invalid) > 0 {
		return vmKind.VM{}, invalid, nil
	}

	spec := asked.Spec
	spec.Ports = vmKind.Normalized(spec.Ports)
	spec.Network = opened(spec.Network)

	if spec.Source != nil && len(strings.TrimSpace(spec.Source.Snapshot)) == 0 {
		spec.Source = nil
	}

	invalid := make(domain.ValidationErrors)

	if spec.Source != nil {
		refused, err := s.fromSnapshot(ctx, asked.Metadata.OwnerUUID, &spec)
		if err != nil || len(refused) > 0 {
			return vmKind.VM{}, refused, err
		}
	} else {
		s.imaged(&spec, invalid)
	}

	limits := s.Quota.Limits()
	maps.Copy(invalid, limits.Bounds("", s.kindOf(spec), spec.Resources))
	maps.Copy(invalid, limits.Lifetime("", asked.Metadata.Lifetime))

	if len(invalid) > 0 {
		return vmKind.VM{}, invalid, nil
	}

	held, err := s.Quota.Check(ctx, "", asked.Metadata.OwnerUUID, spec.Resources, "")
	if err != nil || len(held) > 0 {
		return vmKind.VM{}, held, err
	}

	name := strings.TrimSpace(asked.Metadata.Name)

	slug, err := slugs.Generate(ctx, name, s.Slugs...)
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	labels := maps.Clone(asked.Metadata.Labels)
	delete(labels, vmKind.LabelManagedBy)

	admitted := vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			Name:      name,
			Slug:      slug,
			OwnerUUID: asked.Metadata.OwnerUUID,
			Labels:    labels,
			Lifetime:  asked.Metadata.Lifetime,
		},
		Spec:   spec,
		Status: vmKind.Status{Status: kind.Status{State: vmKind.Created, Expected: vmKind.Running}},
	}

	chosen, err := s.Placement.Pick(ctx, spec.Resources)

	switch {
	case errors.Is(err, vm.ErrNoCapacity):
		// failed, and given up on, rather than asked for again and again:
		// whoever wants it starts it once there is room, which places it.
		admitted.Status.State = vmKind.Failed
		admitted.Status.Expected = vmKind.Failed
		admitted.Status.Reason = ReasonNoCapacity
	case err != nil:
		return vmKind.VM{}, nil, err
	default:
		admitted.Metadata.Node = chosen.Name
	}

	return admitted, nil, nil
}

// Reconcile is what a VM is asked for, given what it was asked to be and
// what it was last seen doing.
func (s *VMs) Reconcile(_ context.Context, v vmKind.VM) ([]kind.Intent, error) {
	switch state, expected := v.Status.State, v.Status.Expected; {
	case expected == vmKind.Stopped && state == vmKind.Running:
		return []kind.Intent{{Action: vmKind.ActionStop, Reason: "it runs while it was expected stopped"}}, nil

	case reconfigured(v):
		return []kind.Intent{{Action: vmKind.ActionReconfigure, Reason: "its ports, network or resources are not what its node last gave it"}}, nil

	case expected == vmKind.Running && state == vmKind.Created:
		return []kind.Intent{{Action: vmKind.ActionCreate, Reason: "it was admitted, and is to be made"}}, nil

	case expected == vmKind.Running && state == vmKind.Stopped:
		return []kind.Intent{{Action: vmKind.ActionStart, Reason: "it is not running, and was expected to be"}}, nil

	case expected == vmKind.Running && state == vmKind.Failed:
		return []kind.Intent{{Action: vmKind.ActionStart, Reason: "it failed while it was expected running"}}, nil
	}

	return nil, nil
}

// reconfigured reports whether a VM at rest, whose node holds it, is to be
// given the ports, network and resources its spec now has: its node last
// applied others. One never made has nothing applied, and is made with its
// spec.
func reconfigured(v vmKind.VM) bool {
	if v.Status.Applied == nil || (v.Status.State != vmKind.Running && v.Status.State != vmKind.Stopped) {
		return false
	}

	return !v.Status.Applied.Equal(v.Spec.Config())
}

// Apply carries out a VM's update: its name and lifetime, which are its
// record's alone, and its ports, network and resources, which its node is
// then asked to apply. What is wrong with it is said field by field, and
// nothing changes then.
func (s *VMs) Apply(ctx context.Context, v vmKind.VM, action string, payload any) (vmKind.VM, domain.ValidationErrors, error) {
	if action != vmKind.ActionUpdate {
		return vmKind.VM{}, nil, fmt.Errorf("%w: a vm has no %q run in the control plane", kind.ErrUnknownAction, action)
	}

	update, _ := payload.(vmKind.UpdatePayload)
	invalid := make(domain.ValidationErrors)

	if update.Name != nil {
		v.Metadata.Name = strings.TrimSpace(*update.Name)
	}

	if update.Lifetime != nil {
		v.Metadata.Lifetime = *update.Lifetime
		v.Metadata.ExpiresAt = time.Time{}

		if v.Metadata.Lifetime > 0 {
			v.Metadata.ExpiresAt = s.Now().Add(v.Metadata.Lifetime)
		}

		maps.Copy(invalid, s.Quota.Limits().Lifetime("", v.Metadata.Lifetime))
	}

	respecified, err := s.respecified(ctx, v, update, invalid)
	if err != nil {
		return vmKind.VM{}, nil, err
	}

	if len(invalid) > 0 {
		return vmKind.VM{}, invalid, nil
	}

	v.Spec = respecified

	return v, nil, nil
}

// respecified is a VM's spec with the ports, network and resources an update
// asks for. What cannot be given is written into invalid: a change to a VM
// on its way somewhere, which has been asked for something already and a
// change sent after which could reach its node before it; resources past what
// one VM may be given or its owner's quota, or its node's room; and a disk
// that would shrink, since a disk is not taken back from under what is written
// on it.
func (s *VMs) respecified(ctx context.Context, v vmKind.VM, update vmKind.UpdatePayload, invalid domain.ValidationErrors) (vmKind.Spec, error) {
	spec := v.Spec

	if update.Ports != nil {
		spec.Ports = vmKind.Normalized(*update.Ports)
	}

	if update.Network != nil {
		if len(update.Network.Ingress) > 0 {
			spec.Network.Ingress = update.Network.Ingress
		}

		if len(update.Network.Egress) > 0 {
			spec.Network.Egress = update.Network.Egress
		}
	}

	if update.Resources != nil {
		spec.Resources = *update.Resources
	}

	if spec.Config().Equal(v.Spec.Config()) {
		return spec, nil
	}

	if vmKind.Machine().IsInFlight(v.Status.State) {
		invalid["vm"] = "invalid_state_transition"

		return spec, nil
	}

	if spec.Resources == v.Spec.Resources {
		return spec, nil
	}

	refused := s.Quota.Limits().Bounds("", s.kindOf(spec), spec.Resources)
	if spec.Resources.Disk < v.Spec.Resources.Disk {
		refused["resources.disk"] = "disk_cannot_shrink"
	}

	if len(refused) > 0 {
		maps.Copy(invalid, refused)

		return spec, nil
	}

	held, err := s.Quota.Check(ctx, "", v.Metadata.OwnerUUID, spec.Resources, v.Metadata.UUID)
	if err != nil {
		return vmKind.Spec{}, err
	}

	if len(held) > 0 {
		maps.Copy(invalid, held)

		return spec, nil
	}

	fits, err := s.Placement.Fits(ctx, v, spec.Resources)
	if err != nil {
		return vmKind.Spec{}, err
	}

	if !fits {
		invalid["resources"] = ReasonNoCapacity
	}

	return spec, nil
}

// Prepare readies a VM's commands before they are sent: a restore is held to
// its snapshot, and a VM no node had room for is placed before it is started.
func (s *VMs) Prepare(ctx context.Context, v vmKind.VM, action string, payload any) (vmKind.VM, domain.ValidationErrors, error) {
	switch action {
	case vmKind.ActionRestore:
		restore, _ := payload.(vmKind.RestorePayload)

		refused, err := s.restorable(ctx, v, restore.SnapshotUUID)

		return v, refused, err

	case vmKind.ActionStart, vmKind.ActionRestart, vmKind.ActionCreate:
		if len(v.Metadata.Node) > 0 {
			return v, nil, nil
		}

		chosen, err := s.Placement.Pick(ctx, v.Spec.Resources)

		switch {
		case errors.Is(err, vm.ErrNoCapacity):
			return vmKind.VM{}, domain.ValidationErrors{"vm": ReasonNoCapacity}, nil
		case err != nil:
			return vmKind.VM{}, nil, err
		}

		v.Metadata.Node = chosen.Name
	}

	return v, nil, nil
}

// restorable is why a VM cannot be restored from a snapshot, or nothing when
// it can: the snapshot kind has to say the snapshot can be restored onto the
// VM, held to the engine its node runs when its node has said which, and the
// VM has to be on a node that can be asked.
func (s *VMs) restorable(ctx context.Context, v vmKind.VM, snapshotUUID string) (domain.ValidationErrors, error) {
	engine, err := s.engineOf(ctx, v.Metadata.Node)
	if err != nil {
		return nil, err
	}

	_, refused, err := s.Snapshots.Restorable(ctx, snapshotUUID, snapshotKind.Target{
		OwnerUUID: v.Metadata.OwnerUUID,
		Image:     v.Spec.Image,
		Disk:      v.Spec.Resources.Disk,
		Engine:    engine,
	})
	if err != nil || len(refused) > 0 {
		return refusedFrom(refused, "snapshot_uuid"), err
	}

	alive, err := s.Placement.Alive(ctx, v.Metadata.Node)
	if err != nil {
		return nil, err
	}

	// the disk is replaced where it is, so the VM has to be on a node that
	// can be asked.
	if !alive {
		return domain.ValidationErrors{"vm": "invalid_state_transition"}, nil
	}

	return nil, nil
}

// engineOf is the engine a node runs, as its heartbeat last said, without
// its version: nothing for a node that has not said, or is not known.
func (s *VMs) engineOf(ctx context.Context, nodeName string) (string, error) {
	if len(nodeName) == 0 {
		return "", nil
	}

	n, err := s.Nodes.GetOne(ctx, nodeName)
	if errors.Is(err, domain.ErrNotExists) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return n.Capacity.Engine, nil
}

// refusedFrom is a snapshot kind's refusal under field, or nothing for none.
func refusedFrom(refused string, field string) domain.ValidationErrors {
	if len(refused) == 0 {
		return nil
	}

	return domain.ValidationErrors{field: refused}
}

// Extras are the code runner's runs, which a listing of anybody's VMs has
// beside their records.
func (s *VMs) Extras() kind.Extras {
	return s.Dependencies.Extras
}

// Narrows reports whether word is what a VM can be: a machine or docker.
func (s *VMs) Narrows(word string) bool {
	return vm.Kind(word).IsValid()
}

// Is reports whether r is a VM of the kind word names, machine or docker, as
// its image says. One of the code runner's runs is a machine, booted from the
// runner's image.
func (s *VMs) Is(r kind.Raw, word string) (bool, error) {
	v, err := kind.Decode[vmKind.Spec, vmKind.Status](r)
	if err != nil {
		return false, err
	}

	return s.kindOf(v.Spec) == vm.Kind(word), nil
}

// kindOf is what a VM of spec is, a machine or a Docker VM, as its image
// says.
func (s *VMs) kindOf(spec vmKind.Spec) vm.Kind {
	return vm.KindOf(spec.Image, s.Images.Docker)
}

// fromSnapshot makes spec from a snapshot of ownerUUID's that the snapshot
// kind says a VM can be made from: its image, and so what it is, is the
// snapshot's, and its disk is at least the snapshot's. One asked to boot an
// image of another kind than the snapshot was taken of, where naming none is
// asking for a machine, is refused under kind, the field the dashboard asks
// what a VM is under.
func (s *VMs) fromSnapshot(ctx context.Context, ownerUUID string, spec *vmKind.Spec) (domain.ValidationErrors, error) {
	taken, refused, err := s.Snapshots.Restorable(ctx, spec.Source.Snapshot, snapshotKind.Target{OwnerUUID: ownerUUID, Image: spec.Image})

	switch {
	case err != nil:
		return nil, err
	case refused == snapshotKind.RefusedKind:
		return refusedFrom(refused, "kind"), nil
	case len(refused) > 0:
		return refusedFrom(refused, "snapshot_uuid"), nil
	}

	spec.Image = taken.Image
	spec.Resources.Disk = max(spec.Resources.Disk, taken.Disk)

	return nil, nil
}

// imaged gives spec its image. One that names the Docker image's repository
// is a Docker VM, and dockerd and the way it is started are the Docker
// image's, so a Docker VM boots from nothing else; a machine boots from the
// default image when it names none.
func (s *VMs) imaged(spec *vmKind.Spec, invalid domain.ValidationErrors) {
	if s.kindOf(*spec) == vm.KindDocker {
		if spec.Image != s.Images.Docker {
			invalid["image"] = "invalid_image"
		}

		return
	}

	if len(spec.Image) == 0 {
		spec.Image = s.Images.Machine
	}
}

// validate is what is wrong with a VM as it was asked for, that the request
// alone tells: what it is held to beside, the snapshot it names, its bounds
// and its owner's quota, is the strategy's to check.
func validate(asked vmKind.VM) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	if code, ok := vmKind.ValidateName(asked.Metadata.Name); !ok {
		invalid["name"] = code
	}

	if code, ok := vmKind.ValidatePorts(asked.Spec.Ports); !ok {
		invalid["ports"] = code
	}

	if access := asked.Spec.Network.Ingress; len(access) > 0 && !access.IsValid() {
		invalid["network.ingress"] = "invalid_access"
	}

	if access := asked.Spec.Network.Egress; len(access) > 0 && !access.IsValid() {
		invalid["network.egress"] = "invalid_access"
	}

	if asked.Metadata.Lifetime < 0 {
		invalid["lifetime_seconds"] = "invalid_lifetime"
	}

	return invalid
}

// opened is a network with a way left empty allowed, which is what a VM
// somebody made to reach and be reached is usually for.
func opened(network vmKind.Network) vmKind.Network {
	if len(network.Ingress) == 0 {
		network.Ingress = vm.AccessAllow
	}

	if len(network.Egress) == 0 {
		network.Egress = vm.AccessAllow
	}

	return network
}
