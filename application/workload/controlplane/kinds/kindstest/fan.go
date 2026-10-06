// Package kindstest is a kind for the control plane's generic code to be
// tested with, registered through the same bindings a real kind is: a fan,
// which lives in a house, is made, started, stopped and deleted on its node,
// renamed in the control plane, and asked for its state and its log.
//
// It is small enough to read whole, and has one of everything the generic
// code handles: an internal command, a command with a payload, one run in the
// control plane, queries with and without a payload, a parent, and the states
// the framework moves resources into itself. Nothing outside tests registers
// it.
package kindstest

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

const (
	// Kind is the fan's name, and Plural its routes'.
	Kind   = "fan"
	Plural = "fans"

	// Parent is the kind a fan lives in. Houses are not a kind anything
	// registers: a fan only names one.
	Parent = "house"
)

// The fan's states, beside the framework's own: kind.Failed, kind.Missing and
// kind.Deleted.
const (
	Pending  kind.State = "pending"
	Starting kind.State = "starting"
	Running  kind.State = "running"
	Stopping kind.State = "stopping"
	Stopped  kind.State = "stopped"
	Deleting kind.State = "deleting"
)

// Spec is what a fan is asked for as.
type Spec struct {
	Blades int `json:"blades"`

	// House is the house it is put in, if any: it becomes its parent.
	House string `json:"house,omitempty"`
}

// Status is what a fan is doing.
type Status struct {
	kind.Status

	// Speed is how fast its node says it turns.
	Speed int `json:"speed"`

	// Renames is how many times it was renamed, which only the control plane
	// knows: its node never mentions it.
	Renames int `json:"renames,omitempty"`
}

// StartPayload is what a fan is started with.
type StartPayload struct {
	Speed int `json:"speed"`
}

func (p StartPayload) Validate() domain.ValidationErrors {
	if p.Speed < 0 || p.Speed > 3 {
		return domain.ValidationErrors{"speed": "invalid_value"}
	}

	return nil
}

// RenamePayload is what a fan is renamed to.
type RenamePayload struct {
	Name string `json:"name"`
}

func (p RenamePayload) Validate() domain.ValidationErrors {
	if len(p.Name) == 0 {
		return domain.ValidationErrors{"name": "required_field"}
	}

	return nil
}

// LogsPayload narrows what is read of a fan's log.
type LogsPayload struct {
	Since time.Time `json:"since"`
	Tail  uint      `json:"tail"`
}

func (p LogsPayload) Validate() domain.ValidationErrors {
	if p.Tail > 1000 {
		return domain.ValidationErrors{"tail": "invalid_value"}
	}

	return nil
}

// Descriptor is the fan's, a new one every time, so a test can change it
// without changing it for another.
func Descriptor() kind.Descriptor {
	return kind.Descriptor{
		Name:     Kind,
		Plural:   Plural,
		StateBy:  kind.OnNode,
		Parent:   Parent,
		OnParent: kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep},
		Machine:  Machine(),
		Actions: []kind.Action{
			{Name: "create", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Pending, kind.Missing}, Desires: Running, Internal: true, Payload: kind.NoPayload},
			{Name: "start", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Stopped, kind.Failed}, Desires: Running, Permission: "manage", Payload: kind.Payload[StartPayload]()},
			{Name: "stop", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{Running}, Desires: Stopped, Permission: "manage", Payload: kind.NoPayload},
			{Name: "rename", Runs: kind.OnControlPlane, Mode: kind.ModeCommand, Permission: "update", Payload: kind.Payload[RenamePayload]()},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
			{Name: "logs", Runs: kind.OnNode, Mode: kind.ModeQuery, AllowedIn: []kind.State{Running}, Permission: "logs", Payload: kind.Payload[LogsPayload]()},
		},
	}
}

// Machine is the fan's states and the moves between them.
func Machine() kind.Machine {
	return kind.Machine{
		Initial: Pending,
		States:  []kind.State{Pending, Starting, Running, Stopping, Stopped, kind.Failed, kind.Missing, Deleting, kind.Deleted},
		Transitions: []kind.Transition{
			{From: Pending, On: kind.OnAction("create"), To: Starting},
			{From: kind.Missing, On: kind.OnAction("create"), To: Starting},
			{From: Starting, On: kind.OnObserved(Running), To: Running},
			{From: Stopped, On: kind.OnAction("start"), To: Starting},
			{From: kind.Failed, On: kind.OnAction("start"), To: Starting},
			{From: Running, On: kind.OnAction("stop"), To: Stopping},
			{From: Stopping, On: kind.OnObserved(Stopped), To: Stopped},
			{From: Running, On: kind.OnObserved(kind.Missing), To: kind.Missing},
			{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
			{From: kind.Any, On: kind.OnAction("delete"), To: Deleting},
			{From: Deleting, On: kind.OnObserved(kind.Missing), To: kind.Deleted},
			{From: Deleting, On: kind.OnObserved(kind.Deleted), To: kind.Deleted},
		},
		Terminal: []kind.State{Stopped, kind.Failed, kind.Deleted},
		InFlight: []kind.State{Pending, Starting, Stopping, Deleting},
	}
}

// Fan is a fan as the strategies are handed one.
type Fan = kind.Resource[Spec, Status]
