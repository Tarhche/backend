package createVM

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors

	// ID is what vmhost calls the VM it made.
	ID string
}
