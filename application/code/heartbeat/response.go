package heartbeat

type Response struct {
	Name string `json:"name"`
	Logs []byte `json:"logs"`

	// Error is why the code never ran, when it never did. Code that ran says
	// what it has to say through its output, however badly it went.
	Error string `json:"error,omitempty"`
}
