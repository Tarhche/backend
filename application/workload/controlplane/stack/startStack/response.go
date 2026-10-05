package startStack

import "github.com/khanzadimahdi/testproject/domain"

// Response says why the stack could not be asked, when it could not.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
