package requestDocker

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Request is one question for a Docker VM's dockerd, as one person's own VM
// when OwnerUUID is set.
type Request struct {
	OwnerUUID string
	VMUUID    string
	Op        noderequest.Op

	// Payload is the operation's own request, passed on as it came.
	Payload json.RawMessage
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.VMUUID) == 0 {
		validationErrors["vm_uuid"] = "required_field"
	}

	if !r.Op.IsDocker() {
		validationErrors["op"] = "invalid_value"
	}

	if len(r.Payload) > 0 && !json.Valid(r.Payload) {
		validationErrors["payload"] = "invalid_value"
	}

	return validationErrors
}
