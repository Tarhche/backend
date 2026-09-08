package attachContainer

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// Request opens a terminal in a container.
//
// The access token travels in the payload because a websocket handshake from a
// browser carries no Authorization header, and one connection is shared by
// every request on it: the person is established per request, not per socket.
type Request struct {
	ID            string   `json:"id"`
	ContainerUUID string   `json:"container_uuid"`
	AccessToken   string   `json:"access_token"`
	Command       []string `json:"command"`
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

// Input is what a client sends to a terminal it already has open: the keys
// somebody pressed, or the size of the window they are pressing them in.
type Input struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data []byte `json:"data"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

// the kinds of input a terminal takes.
const (
	inputKeys   = ""
	inputResize = "resize"
)
