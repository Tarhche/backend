package deleteStack

import "github.com/khanzadimahdi/testproject/domain"

// Response says whether the stack is gone, or why it was not deleted.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Pending says the stack is being taken down, and goes once it is.
	Pending bool `json:"pending,omitempty"`
}
