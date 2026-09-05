package stop

import "github.com/khanzadimahdi/testproject/domain"

// Response says nothing when the container is gone, which is all there is to
// say: what the page does next is stop watching it.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
