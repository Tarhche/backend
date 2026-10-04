package api

// ExecRequest starts a command inside a running run, over RouteExec's
// websocket. It is the first frame the client sends, as text.
//
// The frames mirror the platform's own terminal websocket:
//
//  1. The client sends an ExecRequest.
//  2. The service answers with a started Control, which carries the exec's
//     ID, or with an error Control, and then closes.
//  3. The client's binary frames are the command's stdin, as they are. The
//     service's binary frames are its output, each prefixed with
//     OutputStdout or OutputStderr.
//  4. The client's text frames are Controls: resize, close_stdin and signal.
//  5. The service ends with an exit Control, which carries the exit code,
//     and closes.
//
// Closing the websocket does not stop the command, as closing a task's exec
// session does not: it only lets go of the stream. Ending it is a call of its
// own, RouteEndExec, so that a terminal whose connection dropped is ended with
// a grace period rather than killed with the connection.
type ExecRequest struct {
	Command []string `json:"command"`
	TTY     bool     `json:"tty,omitempty"`

	// Env is KEY=VALUE entries laid over the run's environment, and WorkDir
	// replaces the run's working directory when it is set. The user is the
	// run's.
	Env     []string `json:"env,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`

	// Rows and Cols are the terminal's size to begin with, when there is
	// one; a resize Control changes it.
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// Control is every text frame after the ExecRequest, in either direction.
// Type says which one it is, and only the fields that type uses are set.
type Control struct {
	Type string `json:"type"`

	// ExecID comes with started, and is what RouteEndExec ends the command
	// by.
	ExecID string `json:"exec_id,omitempty"`

	// Code comes with exit. It is a pointer so that an exit code of zero is
	// still sent.
	Code *int `json:"code,omitempty"`

	// Rows and Cols come with resize.
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`

	// Signal comes with signal, numbered as Linux numbers it, since the
	// guest is Linux whatever the client runs on.
	Signal int `json:"signal,omitempty"`

	// Error comes with error, and says why the command could not be run.
	Error *Error `json:"error,omitempty"`
}

// The types of Control. The service sends the first three and the client the
// rest.
const (
	ControlStarted    = "started"
	ControlExit       = "exit"
	ControlError      = "error"
	ControlResize     = "resize"
	ControlCloseStdin = "close_stdin"
	ControlSignal     = "signal"
)

// The service prefixes each binary frame with the stream it came from. With a
// TTY a command has only the one, and everything arrives as OutputStdout.
const (
	OutputStdout byte = 1
	OutputStderr byte = 2
)
