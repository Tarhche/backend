// Package blocks is what the building blocks of a Docker VM share: the
// container, image, network and volume kinds, each declared in a package of
// its own beside this one (domain/workload/kind).
//
// A building block is stored and reconciled like any other kind: the control
// plane keeps it with what it is expected to be, its VM's node reports what
// that VM's dockerd holds, and the generic loop closes the gap. What makes
// them building blocks is where they live and how they are known:
//
//   - they live in a Docker VM, their Parent: they go with it when it is
//     deleted, and are reset to what its disk holds when it is restored from
//     a snapshot (OnParent), so that reconciling does not make again what the
//     restore took away;
//   - what the platform makes in a VM's dockerd carries its labels, managed
//     and the uuid of the resource it is (Labels), so that what a node sees
//     is matched to its record exactly rather than guessed by name;
//   - what a VM's dockerd holds that the platform did not make is reported as
//     well, and kept by whoever made it: a stack, whose compose project it is
//     part of and which keeps it as one with the rest, or nobody, when it was
//     made from the VM's terminal, in which case it is shown as unmanaged and
//     never reconciled (OwnershipOf);
//   - and they are asked under the containers' permissions, which the
//     dashboard has always asked them under (PermissionsOf).
package blocks

import (
	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

const (
	// Parent is the kind a building block lives in: a VM, which is always a
	// Docker VM.
	Parent = "vm"

	// PermissionsOf is the plural every building block's permissions are
	// named by: workload.containers.<verb>.
	PermissionsOf = "containers"
)

// OnParent is what becomes of a building block when its Docker VM is deleted,
// which takes its record with it, or restored from a snapshot, which resets
// it to what the restored disk holds: dropped when the disk does not have it,
// and taken in again when the disk has it labelled as the platform's.
func OnParent() kind.ParentRules {
	return kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}
}

// The labels the platform puts on what it makes in a Docker VM's dockerd.
const (
	// LabelManaged marks what the platform made, with "true".
	LabelManaged = "workload.managed"

	// labelPrefix and a kind's name are the label that carries the uuid of
	// the resource an object is: workload.container.
	labelPrefix = "workload."
)

// The labels of the manifest of what a Docker VM's dockerd holds that nobody
// keeps a record of: what keeps it instead.
const (
	// LabelManagedBy says what keeps what has no record of its own.
	LabelManagedBy = "workload.managed-by"

	// ManagedByStack is what a stack made, as part of its compose project.
	ManagedByStack = "stack"

	// ManagedByNobody is what nobody keeps: what was made from the VM's
	// terminal. It is shown as unmanaged, and is never reconciled.
	ManagedByNobody = "nobody"
)

// Label is the label that carries the uuid of the resource of the named kind
// an object is.
func Label(kindName string) string {
	return labelPrefix + kindName
}

// Labels are what the platform labels an object it makes with: managed, and
// the uuid of the resource of the named kind it is.
func Labels(kindName string, uuid string) map[string]string {
	return map[string]string{LabelManaged: "true", Label(kindName): uuid}
}

// Identity is the uuid of the resource of the named kind an object labelled
// so is, or nothing for one the platform did not make.
func Identity(kindName string, labels map[string]string) string {
	if labels[LabelManaged] != "true" {
		return ""
	}

	return labels[Label(kindName)]
}

// Ownership is who keeps what a Docker VM's dockerd holds.
type Ownership string

const (
	// Managed is what the platform made, which a record keeps.
	Managed Ownership = "managed"

	// Stacked is what a stack made, which the stack keeps as one with the
	// rest of its compose project: it is part of the stack, and is not
	// stored one by one.
	Stacked Ownership = "stack"

	// Unmanaged is what nobody keeps: what was made from the VM's terminal.
	Unmanaged Ownership = "unmanaged"
)

// OwnershipOf is who keeps an object of the named kind, by its labels: the
// platform when it labelled it as one of its resources, a stack when it
// carries the stack's label or a compose project's, and nobody otherwise.
func OwnershipOf(kindName string, labels map[string]string) Ownership {
	switch {
	case len(Identity(kindName, labels)) > 0:
		return Managed
	case len(labels[stackKind.LabelStack]) > 0, len(labels[docker.LabelComposeProject]) > 0:
		return Stacked
	}

	return Unmanaged
}

// Owners are what an object of the named kind in a Docker VM belongs to: the
// VM, and the stack its labels name, when they name one.
func Owners(vmUUID string, labels map[string]string) []kind.Reference {
	owners := []kind.Reference{{Kind: Parent, UUID: vmUUID}}

	if stack := labels[stackKind.LabelStack]; len(stack) > 0 {
		owners = append(owners, kind.Reference{Kind: stackKind.Name, UUID: stack})
	}

	return owners
}

// namespace is what Derived works its uuids out in.
var namespace = uuid.Must(uuid.FromString("4f7f0c5e-1c63-5b5e-9a57-0d3a7c1b2e64"))

// Derived is the uuid a building block of the named kind in the Docker VM
// parent is known by when nothing labelled it with one, worked out the same
// wherever it is: an image, which nothing can label once it is pulled, by
// the reference it is pulled as; and what a stack or a VM's terminal made, by
// its Docker id.
func Derived(kindName string, parent string, key string) string {
	return uuid.NewV5(namespace, kindName+"\x00"+parent+"\x00"+key).String()
}

// The states of a building block that is either there or not, a network or a
// volume. Waiting, Missing, Failed and Deleted are the framework's own.
const (
	// Pending is one asked for and not made yet.
	Pending kind.State = "pending"

	// Creating is one being made.
	Creating kind.State = "creating"

	// Present is one its VM's dockerd has.
	Present kind.State = "present"

	// Removing is one being removed. Its record goes once it is.
	Removing kind.State = "removing"
)

// The actions every building block that is either there or not has.
const (
	// ActionCreate makes one that is not there. It is the workload's own to
	// ask for.
	ActionCreate = "create"

	ActionDelete = "delete"
	ActionState  = "state"
)

// Presence is the machine of a building block that is either there or not,
// a network or a volume: made, from pending or from missing, it is present;
// deleted, it is removing until its VM's dockerd has none of it. At rest it is
// what its node says: present, missing when its VM's dockerd has none of it,
// and waiting while its VM is not running.
func Presence() kind.Machine {
	transitions := []kind.Transition{
		{From: Pending, On: kind.OnAction(ActionCreate), To: Creating},
		{From: kind.Missing, On: kind.OnAction(ActionCreate), To: Creating},
		{From: Creating, On: kind.OnObserved(Present), To: Present},
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Removing},
		{From: Removing, On: kind.OnObserved(kind.Missing), To: kind.Deleted},
		{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
	}

	for _, state := range []kind.State{Pending, Present, kind.Waiting, kind.Failed} {
		transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(kind.Missing), To: kind.Missing})
	}

	for _, state := range []kind.State{Pending, Present, kind.Missing, kind.Failed} {
		transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(kind.Waiting), To: kind.Waiting})
	}

	return kind.Machine{
		Initial:     Pending,
		States:      []kind.State{Pending, Creating, Present, Removing, kind.Waiting, kind.Missing, kind.Failed, kind.Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{kind.Failed, kind.Deleted},
		InFlight:    []kind.State{Creating, Removing},
	}
}
