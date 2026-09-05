package watchStacks

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// Request opens a watch on the stacks.
//
// The access token travels in the payload because a websocket handshake from a
// browser carries no Authorization header, and one connection is shared by
// every request on it: the person is established per request, not per socket.
type Request struct {
	ID          string `json:"id"`
	AccessToken string `json:"access_token"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.AccessToken) == 0 {
		validationErrors["access_token"] = "required_field"
	}

	return validationErrors
}
