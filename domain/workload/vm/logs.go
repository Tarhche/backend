package vm

// LogStore keeps what VMs' tasks wrote, as vmhost's own copy of it.
//
// The agent inside a machine numbers what its task writes and keeps it in a
// ring, which is lost with the machine and numbered from one again on every
// boot. vmhost reads it as it is written and keeps every line here, numbered
// after every line the VM wrote before (LogLine), so that a VM's output
// outlives its machine, survives the VM being started again, and is read from
// a file rather than from the guest — which is what the orchestrator does
// several times a second.
//
// It is kept beside the VM's record, and forgotten with it (StateStore.Remove).
type LogStore interface {
	// Writer opens a VM's output for more lines to be kept in it. Only the
	// one keeping a running VM writes: there is one writer to a VM at a time.
	Writer(id string) (LogWriter, error)

	// Reader reads a VM's output from its beginning, and keeps reading it as
	// it grows. Reading a VM that has kept nothing yet reads nothing.
	Reader(id string) LogReader
}

// LogWriter keeps more of one VM's output. It is safe to use from several
// goroutines at once: what a task wrote is read both as it comes and once
// more when it ends, and lines read twice are kept once.
type LogWriter interface {
	// Last is the number of the last line kept, across every boot.
	Last() uint64

	// Add keeps a line. One numbered no later than the last line kept is not
	// kept again, which is what makes reading the agent twice harmless. A
	// gap in the numbers is lines the agent let go of before anybody read
	// them, and is kept as a line saying so.
	Add(line LogLine) error

	// Changed is closed once another line is kept.
	Changed() <-chan struct{}

	Close() error
}

// LogReader reads one VM's output as it grows.
type LogReader interface {
	// Next hands every line kept since it was last asked to emit, in order,
	// and stops at the first line emit refuses. A line still being written is
	// left for the next time.
	Next(emit func(LogLine) error) error
}
