package kind

import (
	"encoding/json"
	"slices"
	"time"
	"unicode/utf8"
)

// The subjects every kind's commands and results travel on, over JetStream.
// They replace the requests and results each workload type had its own of.
const (
	ActOnResourceName   = "workloadActOnResource"
	ResourceActedOnName = "workloadResourceActedOn"
)

// MaxOutput is the most of an action's output a ResourceActedOn carries, in
// bytes, counted from the end: the last lines are the ones that say what
// went wrong.
const MaxOutput = 16 << 10

// ActOnResource asks the node holding a resource to carry out one of its
// kind's command actions.
//
// Every node hears every command and carries out only those addressed to it,
// as it always has. A command carries the resource as the control plane
// recorded it, so a node needs no database to act on it.
type ActOnResource struct {
	// ID tells this command from every other, the same one sent again
	// included, and comes back on its ResourceActedOn: whoever waits for what
	// came of a command waits for its ID.
	ID string `json:"id"`

	Kind   string `json:"kind"`
	UUID   string `json:"uuid"`
	Action string `json:"action"`

	// Node is the node it is addressed to.
	Node string `json:"node"`

	// Attempt is which try at the action this is, counted from zero, so the
	// control plane can tell a late result from the answer to the latest.
	Attempt int `json:"attempt,omitempty"`

	// Payload is the action's own, as its codec reads it.
	Payload json.RawMessage `json:"payload,omitempty"`

	// Resource is the manifest as the control plane recorded it. Its spec
	// and status stay raw until the kind's own strategy reads them.
	Resource Raw `json:"resource"`
}

// ResourceActedOn is what came of an ActOnResource.
//
// A command that could not be carried out is a result too, rather than an
// error: carried out again, it would fail the same way, so it is said once
// and not redelivered.
type ResourceActedOn struct {
	// ID is the command's.
	ID string `json:"id"`

	Kind    string `json:"kind"`
	UUID    string `json:"uuid"`
	Action  string `json:"action"`
	Node    string `json:"node"`
	Attempt int    `json:"attempt,omitempty"`

	OK bool `json:"ok"`

	// Refused says it failed because its node refused it as it was asked
	// (ErrRefused): nothing was done, and the resource is as its Status says
	// it is, not as the command would have left it.
	Refused bool `json:"refused,omitempty"`

	// Status is the kind's status as the action left the resource. A failed
	// action leaves one only when its strategy said what it left.
	Status json.RawMessage `json:"status,omitempty"`

	// Reason is why it failed, in the words of whatever failed it.
	Reason string `json:"reason,omitempty"`

	// Output is what the action printed, compose's say: the last MaxOutput
	// bytes of it.
	Output string `json:"output,omitempty"`

	At time.Time `json:"at,omitzero"`
}

// tail is the last MaxOutput bytes of output, starting at a whole character.
func tail(output string) string {
	if len(output) <= MaxOutput {
		return output
	}

	output = output[len(output)-MaxOutput:]

	for i := 0; i < len(output) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(output[i]) {
			return output[i:]
		}
	}

	return output
}

// Observed is one instance as the node holding it sees it, which is what a
// kind's state action says of each of them.
type Observed[Status any] struct {
	// Kind is filled in by the kind's binding: a strategy need not say what
	// it is.
	Kind string `json:"kind,omitempty"`

	// UUID is the resource the instance is, and empty for one nobody keeps a
	// record of: a container made from a VM's terminal, or one of a stack's.
	UUID string `json:"uuid,omitempty"`

	// Owners are what it belongs to, as the node sees it: the VM a container
	// is in, the stack that made it.
	Owners []Reference `json:"owners,omitempty"`

	Status Status `json:"status"`
}

// Observation is an Observed as it travels, its status not read as its
// kind's yet.
type Observation = Observed[json.RawMessage]

// Report is everything of one kind a node holds, as its state action found
// it: what its node's heartbeats say, an instance in each, and what a query
// for one resource's state is answered from.
//
// What it does not list is not on the node, which is how a query for a
// resource its node lost answers that it is missing. For a kind whose
// resources live inside a parent, that holds only inside the parents the node
// read: a Docker VM whose dockerd did not answer is Unseen, and says nothing
// about the containers in it, either way; and one the node did not look inside
// at all, because it is not running there, is neither read nor unseen, and
// what lives in it waits on it. None of that travels in a heartbeat, which
// carries one instance and nothing of what its node left out.
type Report[Status any] struct {
	Instances []Observed[Status] `json:"instances"`

	// Read are the parents, by uuid, the node looked inside this time: what
	// lives in them and is not listed is not there.
	Read []string `json:"read,omitempty"`

	// Unseen are the parents, by uuid, the node could not look inside this
	// time.
	Unseen []string `json:"unseen,omitempty"`
}

// Find is the instance that is the resource uuid names, if it is listed.
func (r Report[Status]) Find(uuid string) (Observed[Status], bool) {
	for _, instance := range r.Instances {
		if len(instance.UUID) > 0 && instance.UUID == uuid {
			return instance, true
		}
	}

	return Observed[Status]{}, false
}

// Missing reports whether the resource uuid names, living inside the parent
// of that uuid or inside none, is not on the node: the report could see where
// it would be, and does not list it. Inside a parent, that is only so of one
// the report read.
func (r Report[Status]) Missing(uuid string, parent string) bool {
	if len(parent) > 0 && !slices.Contains(r.Read, parent) {
		return false
	}

	_, listed := r.Find(uuid)

	return !listed
}

// Unread reports whether the node did not look inside a parent at all: it
// neither read it nor failed to, because the parent is not running there, or
// is not there. What lives in it cannot be seen, and waits on it.
func (r Report[Status]) Unread(parent string) bool {
	return len(parent) > 0 && !slices.Contains(r.Read, parent) && !slices.Contains(r.Unseen, parent)
}
