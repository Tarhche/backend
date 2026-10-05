package deleteSnapshot

import "github.com/khanzadimahdi/testproject/domain"

// Response says whether the snapshot is gone, or why it was not deleted.
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Pending says the snapshot is still being taken, and goes once its node
	// has finished with it.
	Pending bool `json:"pending,omitempty"`
}
