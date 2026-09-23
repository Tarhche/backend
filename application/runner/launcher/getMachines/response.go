package getMachines

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
	Machines         []machine.Machine       `json:"machines"`
}
