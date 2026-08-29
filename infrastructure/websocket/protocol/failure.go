package protocol

import "github.com/khanzadimahdi/testproject/domain"

// ErrorOnProcessingMessage is the translation key a client is answered with when
// its request failed for a reason that belongs in the logs rather than the wire.
const ErrorOnProcessingMessage = "error_on_processing_the_request"

// FailureResponse is sent when a request never reaches the queue.
type FailureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}
