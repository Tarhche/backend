package attachContainer

// the subjects a client opens and feeds a terminal on.
const (
	// AttachName opens a terminal in a container. The reply is a stream: the
	// command's output, chunk by chunk, until it ends.
	AttachName = "runnerContainerAttach"

	// InputName carries what the person typed, and the size of the window they
	// are typing into, to a terminal already open. It names that terminal with
	// the request's stream id, so it is not a question and gets no answer.
	InputName = "runnerContainerAttachInput"
)
