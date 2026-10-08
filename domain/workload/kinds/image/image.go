// Package image is the image kind: an image a Docker VM is to hold, stored
// and reconciled, declared once for every service that runs it
// (domain/workload/kind).
//
// It is a building block of its Docker VM (domain/workload/kinds/blocks). An
// image is the one building block nothing can label once it is pulled, so it
// is known by its reference instead: a record's uuid is worked out from its
// VM and its reference (UUIDOf), the same way on its node, which reports
// every reference its VM's dockerd holds under the uuid it works out for it.
//
// Its spec is its reference, which never changes. What is decided of it:
//
//	expected  observed                       asked
//	present   pending or missing, VM running  create, which pulls it
//
// An image a stored container uses is implied present, and cannot be deleted
// while that container is stored. One with no record of its own is kept by
// what uses it: the stored container's, or a stack's when one of the stack's
// containers was made from it. Only one nothing kept uses is unmanaged.
package image

import (
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

const (
	// Name is the kind's word, and Plural what its routes are named by. Its
	// permissions are the containers': workload.containers.<verb>.
	Name   = "image"
	Plural = "images"

	// Parent is the kind an image lives in: a Docker VM.
	Parent = blocks.Parent
)

// An image's states. Waiting, Missing, Failed and Deleted are the
// framework's own.
const (
	// Pending is an image asked for and not pulled yet.
	Pending kind.State = "pending"

	// Pulling is an image being pulled.
	Pulling kind.State = "pulling"

	// Present is an image its VM holds.
	Present kind.State = "present"

	// Removing is an image being removed. Its record goes once it is.
	Removing kind.State = "removing"

	Waiting = kind.Waiting
	Missing = kind.Missing
	Failed  = kind.Failed
	Deleted = kind.Deleted
)

// An image's actions.
const (
	// ActionCreate pulls an image its VM does not hold. It is the workload's
	// own to ask for.
	ActionCreate = "create"

	// ActionPull pulls an image again, for what its tag names now.
	ActionPull = "pull"

	// ActionDelete removes an image (DeletePayload).
	ActionDelete = "delete"

	ActionState = "state"
)

// Spec is an image as it is asked for: the reference it is pulled as.
type Spec struct {
	// Reference is the image, as name[:tag] or name@digest.
	Reference string `json:"reference"`
}

// Status is what an image is: its state, and what its VM's dockerd last said
// of it.
type Status struct {
	kind.Status

	// Docker is the image as its VM's dockerd last had it, and nothing until
	// it is pulled.
	Docker *Docker `json:"docker,omitempty"`

	// Failure is what the last command on it failed with, in the codes every
	// side knows; an empty one is a command that did not fail since.
	Failure *noderequest.Error `json:"failure,omitempty"`
}

// Docker is an image as its VM's dockerd has it, under one of the references
// it holds it under.
type Docker struct {
	// Reference is the one it is seen under here: one of its tags, or one of
	// its digests, or nothing for an image nothing names any more.
	Reference string `json:"reference,omitempty"`

	ID   string   `json:"id"`
	Tags []string `json:"tags"`

	// Size is in bytes.
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at,omitzero"`

	// InUse says a container was made from it.
	InUse bool `json:"in_use"`
}

// DockerOf is an image as docker has it, seen under reference.
func DockerOf(i docker.Image, reference string) *Docker {
	tags := append([]string{}, i.Tags...)

	return &Docker{Reference: reference, ID: i.ID, Tags: tags, Size: i.Size, CreatedAt: i.CreatedAt, InUse: i.InUse}
}

// Image is it as docker has it.
func (d *Docker) Image() docker.Image {
	if d == nil {
		return docker.Image{}
	}

	return docker.Image{ID: d.ID, Tags: append([]string{}, d.Tags...), Size: d.Size, CreatedAt: d.CreatedAt, InUse: d.InUse}
}

// DeletePayload is how an image is removed.
type DeletePayload struct {
	// Force removes it even while a container nobody keeps uses it. One a
	// stored container uses is never removed.
	Force bool `json:"force,omitempty"`
}

var _ domain.Validatable = &DeletePayload{}

// Validate says nothing is wrong: either way is a delete.
func (p *DeletePayload) Validate() domain.ValidationErrors {
	return nil
}

// Image is an image, as its strategies are handed one.
type Image = kind.Resource[Spec, Status]

// VMOf is the uuid of the Docker VM an image lives in: its parent.
func VMOf(i Image) string {
	if parent, ok := i.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return ""
}

// UUIDOf is the uuid of the image a Docker VM holds under a reference: one
// worked out from the two, the same on a node as in the control plane, since
// nothing can label an image once it is pulled.
func UUIDOf(vmUUID string, reference string) string {
	return blocks.Derived(Name, vmUUID, Normalized(reference))
}

// Normalized is a reference as docker lists the images it names: by a tag,
// latest when it names none, or by a digest, with docker's own registry and
// its library left out, so that nginx, nginx:latest and
// docker.io/library/nginx:latest are one image.
func Normalized(reference string) string {
	name, digest, digested := strings.Cut(strings.TrimSpace(reference), "@")

	tag := ""
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name, tag = name[:i], name[i+1:]
	}

	for _, registry := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		if trimmed, ok := strings.CutPrefix(name, registry); ok {
			name = trimmed

			break
		}
	}

	name = strings.TrimPrefix(name, "library/")

	if digested {
		return name + "@" + digest
	}

	if len(tag) == 0 {
		tag = "latest"
	}

	return name + ":" + tag
}

// References are every reference an image docker holds is known by, as a
// record of one is: its tags, and its digests.
func References(i docker.Image) []string {
	var references []string

	for _, tag := range i.Tags {
		if len(tag) > 0 && !strings.HasPrefix(tag, "<none>") {
			references = append(references, Normalized(tag))
		}
	}

	for _, digest := range i.Digests {
		if len(digest) > 0 && !strings.HasPrefix(digest, "<none>") {
			references = append(references, Normalized(digest))
		}
	}

	return references
}

// Descriptor is the image kind, a new one every time.
func Descriptor() kind.Descriptor {
	return kind.Descriptor{
		Name:          Name,
		Plural:        Plural,
		PermissionsOf: blocks.PermissionsOf,
		StateBy:       kind.OnNode,
		Parent:        Parent,
		OnParent:      blocks.OnParent(),
		Machine:       Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Pending, Missing}, Internal: true, Timeout: kind.TimeoutPull, Payload: kind.NoPayload},
			{Name: ActionPull, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Present, Missing, Failed}, Desires: Present, Permission: "manage", Timeout: kind.TimeoutPull, Payload: kind.NoPayload},
			{Name: ActionDelete, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.Payload[DeletePayload]()},
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is an image's states and the moves between them: pulled, from
// pending or missing or again, it is present; removed, it is gone. At rest it
// is what its node says: present, missing when its VM's dockerd has none of
// it, and waiting while its VM is not running.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: Pending, On: kind.OnAction(ActionCreate), To: Pulling},
		{From: Missing, On: kind.OnAction(ActionCreate), To: Pulling},
		{From: Present, On: kind.OnAction(ActionPull), To: Pulling},
		{From: Missing, On: kind.OnAction(ActionPull), To: Pulling},
		{From: Failed, On: kind.OnAction(ActionPull), To: Pulling},
		{From: Pulling, On: kind.OnObserved(Present), To: Present},
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Removing},
		{From: Removing, On: kind.OnObserved(Missing), To: Deleted},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
	}

	for _, state := range []kind.State{Pending, Present, Waiting, Failed} {
		transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(Missing), To: Missing})
	}

	for _, state := range []kind.State{Pending, Present, Missing, Failed} {
		transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(Waiting), To: Waiting})
	}

	return kind.Machine{
		Initial:     Pending,
		States:      []kind.State{Pending, Pulling, Present, Removing, Waiting, Missing, Failed, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Failed, Deleted},
		InFlight:    []kind.State{Pulling, Removing},
	}
}
