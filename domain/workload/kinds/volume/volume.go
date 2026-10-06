// Package volume is the volume kind: a docker volume in a Docker VM, stored
// and reconciled, declared once for every service that runs it
// (domain/workload/kind).
//
// It is a building block of its Docker VM (domain/workload/kinds/blocks): it
// lives in the VM, goes with it, and is reset to what a restored disk holds.
// The platform labels the volume it makes as this one, so what the VM's
// dockerd holds is matched to its record exactly.
//
// Its spec is its name and its driver, neither of which changes. What is
// decided of it:
//
//	expected  observed                       asked
//	present   pending or missing, VM running  create
//
// A volume made again after it went missing is empty: whatever was in it is
// gone. It is labelled with when it was made again (LabelRecreated), and says
// so as its reason for as long as it is there.
package volume

import (
	"fmt"
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
	Name   = "volume"
	Plural = "volumes"

	// Parent is the kind a volume lives in: a Docker VM.
	Parent = blocks.Parent
)

// A volume's states: those of a building block that is either there or not
// (blocks.Presence).
const (
	Pending  = blocks.Pending
	Creating = blocks.Creating
	Present  = blocks.Present
	Removing = blocks.Removing

	Waiting = kind.Waiting
	Missing = kind.Missing
	Failed  = kind.Failed
	Deleted = kind.Deleted
)

// A volume's actions.
const (
	// ActionCreate makes a volume that is not there. It is the workload's
	// own to ask for.
	ActionCreate = blocks.ActionCreate

	// ActionDelete removes a volume (DeletePayload).
	ActionDelete = blocks.ActionDelete

	ActionState = blocks.ActionState
)

// LabelRecreated is when a volume was made again after it went missing, in
// RFC 3339: what was in it before then is gone.
const LabelRecreated = "workload.volume.recreated"

// Spec is a volume as it is asked for.
type Spec struct {
	// Name is the only id a volume has.
	Name string `json:"name"`

	// Driver is local, which is also what empty is.
	Driver string `json:"driver,omitempty"`

	// Labels are what it is labelled with beside the platform's own.
	Labels map[string]string `json:"labels,omitempty"`
}

// Docker is the volume spec as docker is asked to make it, labelled as the
// platform's, as the resource uuid names, and with labels beside.
func (s Spec) Docker(uuid string, labels map[string]string) docker.VolumeSpec {
	platform := blocks.Labels(Name, uuid)
	for key, value := range labels {
		platform[key] = value
	}

	merged := make(map[string]string, len(s.Labels)+len(platform))
	for key, value := range s.Labels {
		merged[key] = value
	}

	for key, value := range platform {
		merged[key] = value
	}

	return docker.VolumeSpec{Name: s.Name, Driver: s.Driver, Labels: merged}
}

// Status is what a volume is: its state, and what its VM's dockerd last said
// of it.
type Status struct {
	kind.Status

	// Docker is the volume as its VM's dockerd last had it, and nothing until
	// it is made.
	Docker *Docker `json:"docker,omitempty"`

	// Failure is what the last command on it failed with, in the codes every
	// side knows; an empty one is a command that did not fail since.
	Failure *noderequest.Error `json:"failure,omitempty"`
}

// Docker is a volume as its VM's dockerd has it.
type Docker struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Mountpoint string            `json:"mountpoint"`
	Labels     map[string]string `json:"labels,omitempty"`

	// InUse says a container mounts it.
	InUse bool `json:"in_use"`

	CreatedAt time.Time `json:"created_at,omitzero"`
}

// DockerOf is a volume as docker has it, as a status has it.
func DockerOf(v docker.Volume) *Docker {
	return &Docker{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint, Labels: v.Labels, InUse: v.InUse, CreatedAt: v.CreatedAt}
}

// Volume is it as docker has it.
func (d *Docker) Volume() docker.Volume {
	if d == nil {
		return docker.Volume{}
	}

	return docker.Volume{Name: d.Name, Driver: d.Driver, Mountpoint: d.Mountpoint, Labels: d.Labels, InUse: d.InUse, CreatedAt: d.CreatedAt}
}

// ReasonOf is why a volume is what it is, read off its labels: one made again
// after it went missing is empty, and says so.
func ReasonOf(labels map[string]string) string {
	at, err := time.Parse(time.RFC3339, labels[LabelRecreated])
	if err != nil {
		return ""
	}

	return fmt.Sprintf("it went missing and was made again, empty, at %s: what was in it is gone", at.UTC().Format(time.RFC3339))
}

// DeletePayload is how a volume is removed.
type DeletePayload struct {
	// Force removes it even when docker would rather keep it.
	Force bool `json:"force,omitempty"`
}

var _ domain.Validatable = &DeletePayload{}

// Validate says nothing is wrong: either way is a delete.
func (p *DeletePayload) Validate() domain.ValidationErrors {
	return nil
}

// Volume is a volume, as its strategies are handed one.
type Volume = kind.Resource[Spec, Status]

// VMOf is the uuid of the Docker VM a volume lives in: its parent.
func VMOf(v Volume) string {
	if parent, ok := v.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return ""
}

// Descriptor is the volume kind, a new one every time.
func Descriptor() kind.Descriptor {
	return kind.Descriptor{
		Name:        Name,
		Plural:      Plural,
		PermittedAs: blocks.PermittedAs,
		StateBy:     kind.OnNode,
		Parent:      Parent,
		OnParent:    blocks.OnParent(),
		Machine:     Machine(),
		Actions: []kind.Action{
			{Name: ActionCreate, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Pending, Missing}, Internal: true, Payload: kind.NoPayload},
			{Name: ActionDelete, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.Payload[DeletePayload]()},
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a volume's states and the moves between them: those of a
// building block that is either there or not.
func Machine() kind.Machine {
	return blocks.Presence()
}

// InFlightOf is the state a volume is in while action is carried out on it.
func InFlightOf(action string) kind.State {
	return blocks.PresenceInFlightOf(action)
}
