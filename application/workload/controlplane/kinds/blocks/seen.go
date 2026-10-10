package blocks

import (
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Observed is what every building block's status says of it that the shared
// code reads: what it is doing, what its VM's dockerd last said of it beside
// that, and what the last command on it failed with. Each kind's own status
// has these under the same names, whatever else it has. A container's state
// is read as o.Status.State, and docker's as o.Docker.State.
type Observed struct {
	kind.Status

	*Docker

	Failure *noderequest.Error `json:"failure,omitempty"`
}

// Docker is what a VM's dockerd said of a building block, of whatever kind:
// each kind says some of these.
type Docker struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Reference string `json:"reference,omitempty"`

	// State is a container's, in docker's words, beside the status's own.
	State string `json:"docker_state,omitempty"`

	// InUse says a container uses an image or a volume, and Containers are
	// the containers on a network.
	InUse      bool     `json:"in_use,omitempty"`
	Containers []string `json:"containers,omitempty"`

	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitzero"`
}

// ObservedOf reads what a building block's status says of it. A status that
// cannot be read says nothing.
func ObservedOf(status json.RawMessage) Observed {
	var observed Observed

	if len(status) > 0 {
		_ = json.Unmarshal(status, &observed)
	}

	if observed.Docker == nil {
		observed.Docker = &Docker{}
	}

	return observed
}

// Refusal is what a failed command's status says it failed with, in the
// codes every side knows, or, when it says nothing, its reason.
func Refusal(status json.RawMessage, reason string) *noderequest.Error {
	if failure := ObservedOf(status).Failure; failure != nil && len(failure.Code) > 0 {
		return failure
	}

	if len(reason) == 0 {
		reason = "the command failed"
	}

	return &noderequest.Error{Code: noderequest.CodeInternal, Message: reason}
}
