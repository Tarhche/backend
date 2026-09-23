package ensureNetwork

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
	Network          machine.Network         `json:"network"`
}
