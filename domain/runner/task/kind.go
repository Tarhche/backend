package task

// Kind tells a one-shot task apart from a long-running one. It decides how the
// runner treats the container's lifetime: a job is expected to exit and have
// its whole log shipped back in the heartbeat, while a service is expected to
// keep running, expose ports and stream its log as it is produced.
type Kind string

const (
	// KindJob runs until its command exits. The code runner produces these.
	KindJob Kind = "job"

	// KindService runs until it is stopped or deleted.
	KindService Kind = "service"
)

// DefaultKind is what a task that names no kind runs as, which keeps every
// existing producer of a task working unchanged.
const DefaultKind = KindJob

// IsValid reports whether k is one of the known kinds.
func (k Kind) IsValid() bool {
	return k == KindJob || k == KindService
}

func (k Kind) String() string {
	return string(k)
}
