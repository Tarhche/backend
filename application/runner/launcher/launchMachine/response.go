package launchMachine

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
	Machine          machine.Machine         `json:"machine"`
}
