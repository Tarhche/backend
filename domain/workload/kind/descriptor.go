package kind

import "slices"

// Executor is the service an action runs in, and the one a kind's state is
// known in.
type Executor string

const (
	// OnControlPlane runs in the control plane, on the resource's record:
	// renaming a snapshot, deleting its archive.
	OnControlPlane Executor = "controlplane"

	// OnNode runs on the node holding the resource, as a command or a query
	// addressed to it.
	OnNode Executor = "node"
)

// IsValid reports whether e is a service an action can run in.
func (e Executor) IsValid() bool {
	return e == OnControlPlane || e == OnNode
}

// Mode is how an action is asked and answered.
type Mode string

const (
	// ModeCommand is asked and answered later: it changes something, and
	// what came of it comes back as a Result.
	ModeCommand Mode = "command"

	// ModeQuery is asked and answered at once, and changes nothing.
	ModeQuery Mode = "query"

	// ModeStream is opened and kept open: a terminal. It reaches the node
	// holding the resource through the ingress.
	ModeStream Mode = "stream"
)

// IsValid reports whether m is a way of asking for an action.
func (m Mode) IsValid() bool {
	return m == ModeCommand || m == ModeQuery || m == ModeStream
}

// Timeout names how long a node may take over a command, when that may be
// longer than the control plane's patience with one it has heard nothing of:
// an image pulled first, or a VM's disk streamed to or from the bucket. Each
// service sizes the names from its own settings, the same ones: a node gives
// the command that long, and the control plane waits that long, and then its
// patience, before it asks again for what may still be being done.
type Timeout string

const (
	// TimeoutPull is a command that may pull an image first: the wait for its
	// Docker VM's dockerd, and the pull (WORKLOAD_DOCKER_READY_TIMEOUT and
	// WORKLOAD_DOCKER_PULL_TIMEOUT), or a code-runner task's runner image.
	TimeoutPull Timeout = "pull"

	// TimeoutTransfer is a command that streams a VM's disk to or from the
	// bucket: a snapshot taken, a VM restored or made from one.
	TimeoutTransfer Timeout = "transfer"
)

// IsValid reports whether t names how long a command may take: one of the
// names, or none, which is the control plane's patience.
func (t Timeout) IsValid() bool {
	return len(t) == 0 || t == TimeoutPull || t == TimeoutTransfer
}

// Cascade is what becomes of a resource when something happens to its
// parent.
type Cascade string

const (
	// CascadeDelete goes where the parent goes: its record is deleted with
	// it. A Docker VM's containers go with the VM.
	CascadeDelete Cascade = "delete"

	// CascadeKeep outlives what happens to the parent, and is reconciled as
	// it was. A snapshot outlives the VM it was taken of.
	CascadeKeep Cascade = "keep"

	// CascadeReset is reset to what the parent holds afterwards: dropped
	// when the parent no longer has it, and adopted when the parent has one
	// nobody kept a record of. A restored Docker VM's containers are what its
	// restored disk holds, so reconciling does not make again what the restore
	// just took away.
	CascadeReset Cascade = "reset"
)

// ParentRules are what deleting a resource's parent does to it, and what
// restoring the parent from a snapshot does. A kind with a parent has both.
type ParentRules struct {
	Delete  Cascade `json:"delete"`
	Restore Cascade `json:"restore"`
}

// Action is one thing a resource of a kind can be asked.
type Action struct {
	// Name is the action's word: "start", "apply", "state", "attach". It is
	// what its routes and its node requests are named by.
	Name string `json:"name"`

	Runs Executor `json:"runs"`
	Mode Mode     `json:"mode"`

	// AllowedIn are the states it may be asked in. None is any.
	AllowedIn []State `json:"allowed_in,omitempty"`

	// Desires is the state a command asks the resource to reach, which
	// becomes its Expected: start desires running, delete desires deleted. A
	// command that changes nothing about where the resource is going desires
	// nothing, and neither does a query or a stream.
	Desires State `json:"desires,omitempty"`

	// Permission is the verb of the permissions it is asked under:
	// workload.<plural>.<verb> over anybody's resources, and its self. twin
	// over one's own. Several actions may share one, as start, stop and
	// restart share manage.
	Permission string `json:"permission,omitempty"`

	// Internal is an action only the workload asks for itself, such as the
	// create a reconcile sends a node that has lost a VM. Nobody is asked a
	// permission for it, and no route serves it.
	Internal bool `json:"internal,omitempty"`

	// Waits says that, asked of a resource in flight, it is not refused but
	// waits its turn: what it desires is written down as what the resource is
	// expected to be, and the reconcile loop asks for it once the resource
	// has got where it was going. A VM asked to stop while it starts stops
	// once it runs, and one asked to start while it stops starts once it has
	// stopped. Only a command that desires a state has anything to wait for,
	// and nothing waits on a resource on its way to being deleted.
	Waits bool `json:"waits,omitempty"`

	// Restores says that, carried out, it gives the resource what a snapshot
	// held, as a VM's restore gives the VM a snapshot's disk: what lives in
	// the resource is then whatever it holds afterwards, and is reset to it,
	// as the kinds that live in it say (ParentRules.Restore).
	Restores bool `json:"restores,omitempty"`

	// Timeout names how long its node may take over a command that can take
	// longer than the control plane's patience. None is that patience, which
	// is enough for a command that changes nothing slow.
	Timeout Timeout `json:"timeout,omitempty"`

	// Public says that a stream is opened for anybody who asks, signed in or
	// not. Whom it is opened for is still its node strategy's to say, from
	// the resource itself: a code-runner snippet's terminal is everybody's, as
	// the page it runs on is, while a task of somebody's would be theirs
	// alone. Only a stream is public; every other action is asked by
	// somebody.
	Public bool `json:"public,omitempty"`

	// Payload reads and checks what the action is asked with. NoPayload is
	// the codec of one asked with nothing.
	Payload Codec `json:"-"`
}

// Descriptor is everything a kind declares about itself: its names, where
// its state is known, its parent, its machine and its actions. It is the same
// value in every service a kind is registered in, and the API serves it, so
// the dashboard offers exactly the actions a resource's state allows.
type Descriptor struct {
	// Name is the kind's word: "vm", "stack". It names its node requests
	// and its messages.
	Name string `json:"name"`

	// Plural names its routes and its permissions: "vms", "stacks".
	Plural string `json:"plural"`

	// StateBy is where what a resource of the kind is doing is known: on the
	// node holding it, which reports it in every heartbeat, or in the control
	// plane, on its record, for a kind that is a series of operations rather
	// than something a node keeps running, such as a snapshot.
	StateBy Executor `json:"state_by"`

	// Parent is the kind a resource of this one lives inside, such as the VM
	// a container runs in, or nothing. OnParent says what becomes of it when
	// its parent is deleted or restored.
	Parent   string      `json:"parent,omitempty"`
	OnParent ParentRules `json:"on_parent,omitzero"`

	// Endpoints says its resources' ports are served through the ingress,
	// under their slugs, as a VM's and a code-runner task's are.
	Endpoints bool `json:"endpoints,omitempty"`

	// PermissionsOf is the plural of the kind whose permissions its actions
	// are asked under, when they are another kind's rather than its own: the
	// code runner's tasks are shown, stopped and deleted among anybody's VMs,
	// under the VMs' permissions, and an image, a network and a volume are
	// asked under the containers', workload.containers.<verb>, as they always
	// were. Nothing is its own plural.
	PermissionsOf string `json:"permissions_of,omitempty"`

	Machine Machine  `json:"machine"`
	Actions []Action `json:"actions"`
}

// Action is the kind's action of that name, if it has one.
func (d Descriptor) Action(name string) (Action, bool) {
	for _, action := range d.Actions {
		if action.Name == name {
			return action, true
		}
	}

	return Action{}, false
}

// Allows reports whether an action may be asked of a resource in state s: an
// action the kind has, asked in a state of its machine that the action
// allows. Nothing is asked of a deleted resource, which is gone.
func (d Descriptor) Allows(action string, s State) bool {
	a, found := d.Action(action)
	if !found || !d.Machine.Has(s) || s == Deleted {
		return false
	}

	return len(a.AllowedIn) == 0 || slices.Contains(a.AllowedIn, s)
}

// Permissions are the two permissions verb is granted under for the kind's
// resources, as domain/permission names them: over anybody's,
// workload.<plural>.<verb>, and over one's own, self.workload.<plural>.<verb>,
// the plural being the one its permissions are of (PermissionsOf). An
// action's are its Permission's; an internal one has none.
func (d Descriptor) Permissions(verb string) (admin string, self string) {
	if len(verb) == 0 {
		return "", ""
	}

	admin = "workload." + d.permissionsPlural() + "." + verb

	return admin, "self." + admin
}

// permissionsPlural is the plural the kind's permissions are named by: its
// own, unless they are another kind's.
func (d Descriptor) permissionsPlural() string {
	if len(d.PermissionsOf) > 0 {
		return d.PermissionsOf
	}

	return d.Plural
}
