package followContainerLogs

import (
	"github.com/khanzadimahdi/testproject/domain"
	"time"
)

// Request opens a container's log stream.
type Request struct {
	ID            string    `json:"id"`
	ContainerUUID string    `json:"container_uuid"`
	AccessToken   string    `json:"access_token"`
	After         time.Time `json:"after"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.ContainerUUID) == 0 {
		validationErrors["container_uuid"] = "required_field"
	}

	if len(r.AccessToken) == 0 {
		validationErrors["access_token"] = "required_field"
	}

	return validationErrors
}
