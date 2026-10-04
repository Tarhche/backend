package api

// Error is what went wrong: a Code a client can act on, and a Message for
// people. It is the body of every failed request, inside an ErrorResponse,
// and what an exec's error frame carries.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error reads as the message, since a task that could not be run is told it
// as its reason, and as the code when there is no message.
func (e *Error) Error() string {
	if len(e.Message) == 0 {
		return e.Code
	}

	return e.Message
}

// The codes an Error carries. A client acts on not_found, which is the
// workload's own "does not exist"; the rest it passes on.
const (
	CodeInvalid      = "invalid"       // the request does not hold
	CodeNotSupported = "not_supported" // it holds, but the service cannot do it
	CodeNotFound     = "not_found"     // no such run, or no such exec
	CodeNameInUse    = "name_in_use"   // the node already has a run by that name
	CodeNotRunning   = "not_running"   // what was asked for needs a running run
	CodeCapacity     = "capacity"      // the node's memory budget cannot take the run
	CodePullFailed   = "pull_failed"   // the image could not be pulled
	CodeUnavailable  = "unavailable"   // the service is not ready yet
	CodeInternal     = "internal"      // anything else
)

// ErrorResponse is the body of every failed request.
type ErrorResponse struct {
	Error Error `json:"error"`
}
