package runs

import (
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// Record is what the supervisor keeps of a run, so that a service that
// restarts knows what it was holding and what each run was asked to be.
//
// It is the run as a client sees it, less what is worked out on the way (its
// endpoints), plus the few facts only the supervisor needs: the host ports the
// run was given, whether its sandbox has been made, and why it last stopped.
type Record struct {
	ID   string      `json:"id"`
	Spec api.RunSpec `json:"spec"`

	State        api.State `json:"state"`
	ExitCode     int       `json:"exit_code"`
	Error        string    `json:"error,omitempty"`
	RestartCount uint      `json:"restart_count"`

	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`

	// HostPorts are where the run's published ports are reached, picked at its
	// first boot and kept until it is deleted: microsandbox fixes a sandbox's
	// ports when it makes the sandbox, so a run that boots again has to be
	// given the same ones.
	HostPorts []api.Endpoint `json:"host_ports,omitempty"`

	// Sandbox is whether the run's sandbox has been made. A run whose sandbox
	// has not been, or has gone, is given a new one at its next start, on
	// the same host ports.
	Sandbox bool `json:"sandbox,omitempty"`

	// Generation counts the sandboxes the run was given in place of one that
	// microsandbox would not start again (ErrStuck), and names its sandbox.
	Generation uint `json:"generation,omitempty"`

	// StoppedByRequest is a run that somebody stopped, killed or restarted
	// rather than one whose main process ended by itself. Its restart policy
	// does not bring it back, and neither does this service starting again.
	StoppedByRequest bool `json:"stopped_by_request,omitempty"`

	// Resume is a run this service stopped because it was shutting down.
	// Its restart policy decides whether it is started again when the
	// service comes back, as it would have for a run the service found
	// running.
	Resume bool `json:"resume,omitempty"`
}

// Run is the record as a client sees it. A run's endpoints are its host ports,
// and there only while something is listening on them.
func (r Record) Run() api.Run {
	run := api.Run{
		ID:           r.ID,
		RunSpec:      cloneSpec(r.Spec),
		State:        r.State,
		ExitCode:     r.ExitCode,
		Error:        r.Error,
		RestartCount: r.RestartCount,
		CreatedAt:    r.CreatedAt,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
	}

	if r.State == api.StateRunning || r.State == api.StateStopping {
		run.Endpoints = slices.Clone(r.HostPorts)
	}

	return run
}

// clone is a copy of the record that shares nothing with it, so it can be
// handed to a store, or to a caller, while the run carries on changing.
func (r Record) clone() Record {
	clone := r
	clone.Spec = cloneSpec(r.Spec)
	clone.HostPorts = slices.Clone(r.HostPorts)

	return clone
}

func cloneSpec(spec api.RunSpec) api.RunSpec {
	spec.Entrypoint = slices.Clone(spec.Entrypoint)
	spec.Command = slices.Clone(spec.Command)
	spec.Environment = slices.Clone(spec.Environment)
	spec.Ports = slices.Clone(spec.Ports)

	return spec
}

// Records keeps one record for each run, so that what the service holds
// outlives the service. Each is written whole and atomically: a service that
// dies halfway through saving one finds the record as it was before, never
// half of each.
type Records interface {
	Save(record Record) error

	// Load is every record kept.
	Load() ([]Record, error)

	// Delete forgets a run's record. One that is not there is no error.
	Delete(id string) error
}

// Journal keeps what each run's main process wrote, across every time the run
// was started, so that its log outlives both the VM and the service.
//
// The supervisor stamps each line before appending it, and stamps are
// strictly increasing within a run: that is what lets a reader resume from
// the last line it kept.
type Journal interface {
	// Append adds lines to the end of a run's journal, in order.
	Append(id string, lines []api.LogLine) error

	// Reader reads a run's journal from its first line at or after since.
	Reader(id string, since time.Time) (JournalReader, error)

	// Last is the stamp of the newest line in a run's journal, and zero when
	// it has none.
	Last(id string) (time.Time, error)

	// Delete forgets a run's journal. One that is not there is no error.
	Delete(id string) error
}

// JournalReader reads one run's journal in order.
type JournalReader interface {
	// Next is the next line. At the end of what has been written so far it
	// is io.EOF, which is not final: once more is appended, Next carries on
	// from where it stopped.
	Next() (api.LogLine, error)

	Close() error
}

// HostPorts hands out the host ports that runs' published ports are reached
// at, so that no two runs are given the same one.
type HostPorts interface {
	// Allocate picks count free ports for a run.
	Allocate(id string, count int) ([]uint16, error)

	// Hold takes ports a run already has, as its record says, so that a
	// service that restarts does not hand them to another run.
	Hold(id string, ports []uint16)

	// Release lets a run's ports go. They are not handed out again straight
	// away, in case a listener of the sandbox they belonged to outlives it.
	Release(id string)
}
