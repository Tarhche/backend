package terminal

// the subjects a reader opens and feeds a snippet's terminal on.
const (
	// AttachName opens a terminal in the container a snippet is running in.
	// The reply is a stream: everything the shell writes, until it ends.
	AttachName = "codeTerminal"

	// InputName carries what the reader typed, and the size of the window they
	// are typing into, to a terminal already open.
	InputName = "codeTerminalInput"
)
