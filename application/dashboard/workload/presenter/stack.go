package presenter

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

// NoteVMNotRunning is what a stack's detail says when it has no containers to
// show because its VM is not running, rather than because it has none.
const NoteVMNotRunning = "vm_not_running"

// Stack is one stack in a Docker VM, as the dashboard shows it: the services,
// networks and volumes its file says, which its graph made there.
type Stack struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`

	// Slug is what it is known by inside its VM: what its graph makes is
	// named under it, <slug>_<name>, and its containers are shown with it as
	// their stack.
	Slug string `json:"slug"`

	OwnerUUID string `json:"owner_uuid"`
	Owner     *Owner `json:"owner,omitempty"`

	// VMUUID is the Docker VM it is deployed into, and VMName what that VM is
	// called now.
	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`

	// Compose is its file, the YAML as it was given. A listing leaves it out:
	// it is as long as somebody wrote it, and a listing is read to choose
	// one.
	Compose string `json:"compose,omitempty"`

	// State is what its graph last said it is, what its containers make it
	// once it is made; ExpectedState is what it was asked to be, running or
	// stopped.
	State         string `json:"state"`
	ExpectedState string `json:"expected_state,omitempty"`

	// Reason is why it is in its state: what it waits on, or what its plan
	// failed with.
	Reason string `json:"reason,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// StackDetail is a stack with the containers its graph made of its services,
// read from its VM as they are now.
type StackDetail struct {
	Stack

	Containers []Container `json:"containers"`

	// Note says why there are no containers when it is not that the stack has
	// none: NoteVMNotRunning.
	Note string `json:"note,omitempty"`
}

// NewStack presents one stack, with its file.
func NewStack(s stack.Stack, owners Owners) Stack {
	return Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		Slug:          s.Slug,
		OwnerUUID:     s.OwnerUUID,
		Owner:         owners.Of(s.OwnerUUID),
		VMUUID:        s.VMUUID,
		VMName:        s.VMName,
		Compose:       s.Compose,
		State:         stackState(s.State),
		ExpectedState: stackState(s.ExpectedState),
		Reason:        s.Reason,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     when(s.UpdatedAt),
	}
}

// NewStacks presents a listing of stacks, without their files.
func NewStacks(stacks []stack.Stack, owners Owners) []Stack {
	items := make([]Stack, len(stacks))
	for i := range stacks {
		items[i] = NewStack(stacks[i], owners)
		items[i].Compose = ""
	}

	return items
}

func NewStackDetail(detail workloadControlPlane.StackDetail, owners Owners) StackDetail {
	presented := StackDetail{
		Stack:      NewStack(detail.Stack, owners),
		Containers: NewContainers(detail.Containers),
	}

	// they are this stack's, which is the one stack nobody has to look up.
	for i := range presented.Containers {
		presented.Containers[i].Stack = detail.Slug
		presented.Containers[i].StackUUID = detail.UUID
	}

	if detail.VMNotRunning {
		presented.Note = NoteVMNotRunning
	}

	return presented
}

func stackState(s stack.State) string {
	if s == 0 {
		return ""
	}

	return s.String()
}
