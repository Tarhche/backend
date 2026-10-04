package runs

import "syscall"

// Why a run ended, when its exit code cannot say. These are Run.Error's
// values, besides the message of a main process that never started.
const (
	// ReasonKilled is a main process a signal ended that the supervisor did
	// not send: the guest ran out of memory, or something inside it killed
	// it.
	ReasonKilled = "killed"

	// ReasonVMLost is a main process whose VM went away under it.
	ReasonVMLost = "vm_lost"

	// ReasonServiceRestarted is a main process that ended because this
	// service went away, or shut down, while it ran.
	ReasonServiceRestarted = "service_restarted"
)

// Docker's numbers for a process it could not start, and for one SIGKILL
// ended.
const (
	exitNotFound      = 127
	exitNotExecutable = 126
	exitCannotStart   = 1
	exitKilled        = 128 + int(sigKill)
)

// ending is how a main process ended, numbered as docker numbers it, which is
// what task.EvaluateState reads.
type ending struct {
	code   int
	reason string
}

// endingOf works out how a main process ended from its last event and from
// what the supervisor did to it.
//
// Microsandbox reports a process a signal ended as -1, without saying which
// signal, but the supervisor knows what it sent: sent is the last signal it
// sent, and zero when it sent none. forced is a VM the supervisor stopped under
// a main process that would not end.
//
//   - A process that exited with N ended with N.
//   - One the stop signal ended ended with 128 plus the signal, which is 143
//     for SIGTERM; one SIGKILL ended, with 137.
//   - One a signal ended that the supervisor did not send ended with 137,
//     and was killed: the guest's own OOM killer, most likely.
//   - One that never started ended with 127 when it was not found, 126 when it
//     could not be executed, and 1 otherwise.
//   - One whose stream ended without an exit ended with 137, its VM lost,
//     unless the supervisor stopped the VM itself.
func endingOf(last Event, ended bool, sent syscall.Signal, forced bool) ending {
	if !ended {
		last = Event{Kind: EventLost}
	}

	switch last.Kind {
	case EventExited:
		if last.ExitCode >= 0 {
			return ending{code: last.ExitCode}
		}

		if sent != 0 {
			return ending{code: 128 + int(sent)}
		}

		return ending{code: exitKilled, reason: ReasonKilled}
	case EventFailed:
		return ending{code: spawnFailureCode(last.Errno), reason: spawnFailureMessage(last)}
	default:
		if forced || sent == sigKill {
			return ending{code: exitKilled}
		}

		return ending{code: exitKilled, reason: ReasonVMLost}
	}
}

// spawnFailureCode is docker's exit code for a program that could not be
// started, by the error that stopped it.
func spawnFailureCode(errno string) int {
	switch errno {
	case "ENOENT":
		return exitNotFound
	case "EACCES", "ENOEXEC":
		return exitNotExecutable
	default:
		return exitCannotStart
	}
}

// spawnFailureMessage is what a run says about a main process that never
// started, which is the reason a task that could not run is given.
func spawnFailureMessage(event Event) string {
	switch {
	case len(event.Errno) > 0 && len(event.Message) > 0:
		return event.Errno + ": " + event.Message
	case len(event.Message) > 0:
		return event.Message
	case len(event.Errno) > 0:
		return event.Errno
	default:
		return "the main process could not be started"
	}
}
