package kind

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"
)

// State is where a resource is in its life, in a word: "running",
// "degraded". Each kind's machine says which words it has. The framework
// knows only the three below, because it moves resources into them itself.
type State string

const (
	// Failed is a resource that could not be made what it was asked to be,
	// or whose node stopped speaking for it. Every kind has it: the framework
	// fails a resource whose command failed for good, and one whose node is
	// lost.
	Failed State = "failed"

	// Deleted is what deleting a resource desires. One that reaches it is
	// gone, record and all, so it is the one state nothing leaves. Every kind
	// has it.
	Deleted State = "deleted"

	// Missing is what a node's report says of a resource it could have
	// listed and did not: the node no longer holds it. It is an observation
	// first, and a state only for a kind whose machine has it.
	Missing State = "missing"

	// Waiting is what is observed of a resource that lives inside a parent
	// its node did not look inside, because the parent is not running: none
	// of it can be seen, and it waits on its parent, whose state its reason
	// says. It is an observation first, and a state only for a kind whose
	// machine has it.
	Waiting State = "waiting"
)

func (s State) String() string {
	return string(s)
}

// Status is what every kind's status starts with.
//
// A kind's own status type embeds it and adds what is particular to it: a
// VM's usage, a stack's services, a snapshot's size. Embedded, its fields are
// the first of one flat JSON object, so the framework reads and writes this
// part of any kind's status without knowing the rest.
type Status struct {
	// State is what it is doing, as last observed. Expected is what it was
	// asked to be doing: what the last command that desired anything desired.
	// Reconciling is closing the gap between the two.
	State    State `json:"state"`
	Expected State `json:"expected,omitempty"`

	// Reason is why it is failed or waiting, when it is.
	Reason string `json:"reason,omitempty"`

	// Since is when it entered State: how long an in-flight state has been
	// waited on, and how long a failed one has rested before it is tried
	// again.
	Since time.Time `json:"since,omitzero"`

	// ObservedAt is when its node, or a command's result, last said what it
	// is doing.
	ObservedAt time.Time `json:"observed_at,omitzero"`
}

// Stated is a kind's status type: one that embeds Status, whose Common is
// then the part every kind shares. A kind's strategies can only be bound when
// its status is one, which is what holds every kind to starting with Status.
type Stated interface {
	Common() *Status
}

var _ Stated = &Status{}

// Common is the status every kind shares. A kind's own status has it by
// embedding Status.
func (s *Status) Common() *Status {
	return s
}

// Reference names one resource of some kind.
type Reference struct {
	Kind string `json:"kind"`
	UUID string `json:"uuid"`
}

// Metadata is what every resource has, whatever its kind.
type Metadata struct {
	UUID string `json:"uuid,omitempty"`
	Name string `json:"name,omitempty"`

	// Slug is the name it is reached under, unique within its kind: a VM's
	// hostname label, a stack's compose project. A kind that is reached under
	// no name leaves it empty.
	Slug string `json:"slug,omitempty"`

	// OwnerUUID is the person it belongs to, or the guest for the code
	// runner's runs.
	OwnerUUID string `json:"owner_uuid,omitempty"`

	Labels map[string]string `json:"labels,omitempty"`

	// Owners are the resources it belongs to: the VM a stack is deployed in,
	// the stack a container was made by. A kind with a parent has its parent
	// among them.
	Owners []Reference `json:"owners,omitempty"`

	// Node is the node holding it, for a kind held on one: where its
	// commands are addressed, and whose heartbeats speak for it.
	Node string `json:"node,omitempty"`

	// Lifetime is how long it is kept, in nanoseconds on the wire, as every
	// duration between the workload's services is. Zero keeps it until it is
	// deleted; anything else deletes it at ExpiresAt.
	Lifetime  time.Duration `json:"lifetime,omitempty"`
	ExpiresAt time.Time     `json:"expires_at,omitzero"`

	CreatedAt time.Time `json:"created_at,omitzero"`
	UpdatedAt time.Time `json:"updated_at,omitzero"`
}

// Expired reports whether a resource has outlived the lifetime it was given.
// One kept until it is deleted never expires.
func (m Metadata) Expired(now time.Time) bool {
	if m.Lifetime <= 0 || m.ExpiresAt.IsZero() {
		return false
	}

	return !now.Before(m.ExpiresAt)
}

// Owner is the resource of the named kind this one belongs to, if it belongs
// to one: its parent, given its kind's Parent.
func (m Metadata) Owner(kindName string) (Reference, bool) {
	for _, owner := range m.Owners {
		if owner.Kind == kindName {
			return owner, true
		}
	}

	return Reference{}, false
}

// clone is a copy of m that shares nothing with it, so that what travels and
// what it was made from can each be changed without the other.
func (m Metadata) clone() Metadata {
	m.Labels = maps.Clone(m.Labels)
	m.Owners = slices.Clone(m.Owners)

	return m
}

// Resource is one resource of a kind, in the shape every kind shares: in Go,
// in messages and in the API.
type Resource[Spec, Status any] struct {
	Kind     string   `json:"kind"`
	Metadata Metadata `json:"metadata"`
	Spec     Spec     `json:"spec"`
	Status   Status   `json:"status"`
}

// Raw is a resource whose kind is not known where it is: its spec and its
// status as JSON. It is what the generic parts of the services hand around,
// the registries, the messages and the repository, while a kind's strategies
// only ever see their own types. Decode and Encode go between the two.
type Raw = Resource[json.RawMessage, json.RawMessage]

// Decode reads a raw resource as its kind's own types. A spec or a status
// that is not there is the zero value.
func Decode[Spec, Status any](raw Raw) (Resource[Spec, Status], error) {
	typed := Resource[Spec, Status]{Kind: raw.Kind, Metadata: raw.Metadata.clone()}

	if err := unmarshal(raw.Spec, &typed.Spec); err != nil {
		return Resource[Spec, Status]{}, fmt.Errorf("the %s's spec cannot be read: %w", raw.Kind, err)
	}

	if err := unmarshal(raw.Status, &typed.Status); err != nil {
		return Resource[Spec, Status]{}, fmt.Errorf("the %s's status cannot be read: %w", raw.Kind, err)
	}

	return typed, nil
}

// Encode is a resource as it travels and is kept: its spec and its status as
// JSON.
func Encode[Spec, Status any](r Resource[Spec, Status]) (Raw, error) {
	spec, err := json.Marshal(r.Spec)
	if err != nil {
		return Raw{}, fmt.Errorf("the %s's spec cannot be written: %w", r.Kind, err)
	}

	status, err := json.Marshal(r.Status)
	if err != nil {
		return Raw{}, fmt.Errorf("the %s's status cannot be written: %w", r.Kind, err)
	}

	return Raw{Kind: r.Kind, Metadata: r.Metadata.clone(), Spec: spec, Status: status}, nil
}

// unmarshal reads raw into value, leaving value as it is when there is
// nothing to read.
func unmarshal(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		return nil
	}

	return json.Unmarshal(raw, value)
}
