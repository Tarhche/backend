package microsandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// record is what the engine keeps of one instance, beside what microsandbox
// keeps of its sandbox.
//
// microsandbox's own record of a sandbox cannot answer for an instance: a
// restore drops the labels, the environment and the init it was made with, and
// the ports a sandbox publishes are never reported back (#1676). So the spec an
// instance was last given, the host ports it was given and what became of its
// main process are kept here, and are the truth for all of them. What the
// sandbox is doing right now is microsandbox's to say.
type record struct {
	Spec vm.Spec `json:"spec"`

	// Image is what the instance booted from: its spec's, or the vmhost's own
	// for a Docker VM.
	Image string `json:"image"`

	// HostPorts is the host port each guest port was given. It is kept for as
	// long as the instance is, so a restart, a restore or a reconfigure keeps
	// the ports its orchestrator dials.
	HostPorts map[port.Port]port.Port `json:"host_ports,omitempty"`

	// Creating marks an instance whose sandbox is being made. One still marked
	// when the vmhost starts again was never made, and is let go of.
	Creating bool `json:"creating,omitempty"`

	// Restored marks an instance whose sandbox came from a snapshot. It lost
	// the init it was made with (#1676), so a Docker VM's dockerd runs under
	// the supervisor that ensure starts rather than as the guest's PID 1.
	Restored bool `json:"restored,omitempty"`

	// MainRunning marks an instance whose main process was started and has not
	// been seen to end. One still marked when the vmhost starts again lost its
	// main process with the vmhost.
	MainRunning bool `json:"main_running,omitempty"`

	// Exit is how its main process ended, once it has.
	Exit *exit `json:"exit,omitempty"`

	// Failure is why an instance is in no state to run: its sandbox could not
	// be made again from a snapshot, or it is gone.
	Failure string `json:"failure,omitempty"`

	StartedAt time.Time `json:"started_at"`
}

// exit is how a main process ended.
type exit struct {
	// Code is its exit code. A process killed by a signal, or lost without
	// saying how it ended, has a negative one.
	Code int `json:"code"`

	// Reason says why, when the process itself could not: it never started,
	// or the vmhost lost it.
	Reason string `json:"reason,omitempty"`

	At time.Time `json:"at"`
}

// clone is a copy of r that shares nothing with it.
func (r *record) clone() *record {
	c := *r
	c.Spec = cloneSpec(r.Spec)
	c.HostPorts = maps.Clone(r.HostPorts)

	if r.Exit != nil {
		e := *r.Exit
		c.Exit = &e
	}

	return &c
}

func cloneSpec(spec vm.Spec) vm.Spec {
	spec.Ports = slices.Clone(spec.Ports)
	spec.Labels = maps.Clone(spec.Labels)
	spec.Entrypoint = slices.Clone(spec.Entrypoint)
	spec.Command = slices.Clone(spec.Command)
	spec.Env = slices.Clone(spec.Env)

	return spec
}

// maxID is the longest name microsandbox gives a sandbox.
const maxID = 128

// validID reports whether id can name an instance: it names its sandbox and
// its record's file, so it is kept to what is safe as either.
func validID(id string) error {
	if len(id) == 0 || len(id) > maxID {
		return fmt.Errorf("an instance's id is 1 to %d characters, not %d", maxID, len(id))
	}

	for n, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case n > 0 && (c == '-' || c == '_' || c == '.'):
		default:
			return fmt.Errorf("an instance's id is letters, digits, '-', '_' and '.', and starts with a letter or a digit: %q", id)
		}
	}

	return nil
}

// store keeps records, one file each, in a directory of their own.
//
// A record is written whole to a file beside it and renamed over the old one,
// so a vmhost that dies halfway leaves the record it had rather than half of
// a new one.
type store struct {
	dir string
}

const recordSuffix = ".json"

func newStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	return &store{dir: dir}, nil
}

// load is every record there is, by the id of its instance.
func (s *store) load() (map[string]*record, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	records := make(map[string]*record, len(entries))

	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), recordSuffix)
		if !ok || entry.IsDir() || validID(id) != nil {
			continue
		}

		content, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var r record
		if err := json.Unmarshal(content, &r); err != nil {
			return nil, fmt.Errorf("the record of %q cannot be read: %w", id, err)
		}

		r.Spec.ID = id
		records[id] = &r
	}

	return records, nil
}

// save writes the record of instance id in place of the one there was.
func (s *store) save(id string, r *record) error {
	content, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	file, err := os.CreateTemp(s.dir, "."+id+".*.tmp")
	if err != nil {
		return err
	}

	written := file.Name()
	defer os.Remove(written)

	if _, err := file.Write(content); err != nil {
		_ = file.Close()

		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()

		return err
	}

	if err := file.Close(); err != nil {
		return err
	}

	if err := os.Rename(written, s.path(id)); err != nil {
		return err
	}

	return s.syncDir()
}

// remove lets go of the record of instance id. One that is not there is gone
// already.
func (s *store) remove(id string) error {
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return s.syncDir()
}

func (s *store) path(id string) string {
	return filepath.Join(s.dir, id+recordSuffix)
}

// syncDir makes a rename or a removal in the directory outlast a crash.
func (s *store) syncDir() error {
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()

	return dir.Sync()
}
