package queryResource

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Response is the answer to a query, or why there is none.
type Response struct {
	ValidationErrors domain.ValidationErrors

	// NodeError is why there is no answer from a node: the resource cannot
	// be asked in the state it is in, or its node refused or did not answer.
	NodeError *noderequest.Error

	// Result is the answer, as the kind gave it, and Truncated says it was
	// cut to fit in a reply.
	Result    json.RawMessage
	Truncated bool
}
