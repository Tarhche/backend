package admitResource

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Response is the resource admitted, or why it was not.
type Response struct {
	ValidationErrors domain.ValidationErrors

	// Resource is the resource as it is kept now.
	Resource kind.Raw

	// Command is the first command it was sent, when its kind asked for one,
	// and Result what came of it, when it was waited for and came in time.
	Command *kind.ActOnResource
	Result  *kind.ResourceActedOn
}
