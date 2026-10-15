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

	// Missing is what is observed of a resource its node no longer holds:
	// one its node has gone on beating without a word of for long enough, or
	// one a query for its state did not find there. It is an observation
	// first, and a state only for a kind whose machine has it.
	Missing State = "missing"

	// Waiting is what is observed of a resource that lives inside a parent
	// its node cannot look inside, because the parent is not running: none of
	// it can be seen, and it waits on its parent, whose state its reason says.
	// It is an observation first, and a state only for a kind whose machine
	// has it.
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
	// is doing, or when its node's silence about it last did: a resource
	// whose state is its node's is taken to be gone from the node once the
	// node goes on beating for long enough after it without a word of it.
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

// Restore is what a resource one of whose commands restores it from a
// snapshot (Action.Restores) says of its last restore, among its status: when
// its node carried it out, by the node's own clock, every restore of it later
// than the one before. A kind whose commands restore its resources embeds it
// in its status beside Status, as a VM's does, so that the framework reads it,
// and takes it onto a record, without knowing the rest: what lives in the
// resource is reset as of that moment, and a record that says an earlier one
// is yet to take that restore.
type Restore struct {
	// RestoredAt is when it was last restored, as its node says it when it
	// restores it: nothing else says it.
	RestoredAt time.Time `json:"restored_at,omitzero"`
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
	// hostname label, the name a stack's graph names what it makes under. A
	// kind that is reached under no name leaves it empty.
	Slug string `json:"slug,omitempty"`

	// OwnerUUID is the person it belongs to, or the guest for the code
	// runner's runs.
	OwnerUUID string `json:"owner_uuid,omitempty"`

	Labels map[string]string `json:"labels,omitempty"`

	// Owners are the resources that own it, those that need it through an
	// owned edge of a graph: a network's stack and the containers on it. It
	// goes with the last of them.
	Owners []Reference `json:"owners,omitempty"`

	// Refs are the resources it uses and does not own, which outlive it:
	// always the one it lives in, given its kind's Parent, whatever owns
	// what, and what it needs through a loose edge of its graph, a volume a
	// container mounts.
	Refs []Reference `json:"refs,omitempty"`

	// Node is the node holding it, for a kind held on one: where its
	// commands are addressed, and whose heartbeats speak for it.
	Node string `json:"node,omitempty"`

	// Lifetime is how long it is kept, in nanoseconds on the wire, as every
	// duration between the workload's services is. Zero keeps it until it is
	// deleted; anything else deletes it at ExpiresAt.
	Lifetime  time.Duration `json:"lifetime,omitempty"`
	ExpiresAt time.Time     `json:"expires_at,omitzero"`

	// Generation counts the specs it has had: 1 once it is admitted, and one
	// more each time its spec changes since. A record kept before
	// generations has none, which is read as its first.
	Generation int64 `json:"generation,omitempty"`

	// DeletionRequestedAt is when it was first asked to be deleted, and
	// nothing while it has not been.
	DeletionRequestedAt time.Time `json:"deletion_requested_at,omitzero"`

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
// to one: its parent, given its kind's Parent, or what owns it, a container's
// stack. It is looked for among its refs first, and then among its owners,
// where what nobody keeps a record of has its parent, as its node names it
// (Observation.Owners).
func (m Metadata) Owner(kindName string) (Reference, bool) {
	for _, belongs := range [][]Reference{m.Refs, m.Owners} {
		for _, r := range belongs {
			if r.Kind == kindName {
				return r, true
			}
		}
	}

	return Reference{}, false
}

// BelongsTo reports whether this one belongs to the resource r names: as one
// of its refs, the resource it lives in among them, or as one of its owners,
// where what nobody keeps a record of has the resource it lives in, as its
// node names it. A reference whose kind is left out is to a resource of any
// kind with its uuid.
func (m Metadata) BelongsTo(r Reference) bool {
	belongs := func(to Reference) bool {
		return to.UUID == r.UUID && (len(r.Kind) == 0 || to.Kind == r.Kind)
	}

	return slices.ContainsFunc(m.Refs, belongs) || slices.ContainsFunc(m.Owners, belongs)
}

// Owning are the resources that own this one, each once, in the order its
// owners name them. A resource none owns is the root of its graph.
func (m Metadata) Owning() []Reference {
	var owning []Reference

	for _, r := range m.Owners {
		if slices.Contains(owning, r) {
			continue
		}

		owning = append(owning, r)
	}

	return owning
}

// Belongs are the resources this one belongs to, each once: its refs, the
// resource it lives in among them, and then its owners. It is what an
// observation of it says it belongs to (Observed.Owners).
func (m Metadata) Belongs() []Reference {
	var belongs []Reference

	for _, r := range slices.Concat(m.Refs, m.Owners) {
		if !slices.Contains(belongs, r) {
			belongs = append(belongs, r)
		}
	}

	return belongs
}

// LivingIn are refs with parent first, in place of any other resource of its
// kind they name, and the rest as they were, each once: a resource lives in
// one resource, the one its kind admits it into, beside whatever else it was
// asked to use.
func LivingIn(refs []Reference, parent Reference) []Reference {
	living := []Reference{parent}

	for _, r := range refs {
		if r.Kind != parent.Kind && !slices.Contains(living, r) {
			living = append(living, r)
		}
	}

	return living
}

// clone is a copy of m that shares nothing with it, so that what travels and
// what it was made from can each be changed without the other.
func (m Metadata) clone() Metadata {
	m.Labels = maps.Clone(m.Labels)
	m.Owners = slices.Clone(m.Owners)
	m.Refs = slices.Clone(m.Refs)

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
