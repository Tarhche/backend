package firecracker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

const (
	recordName  = "machine.json"
	scratchName = "scratch.ext4"
	outputName  = "output.log"
)

// record is everything the orchestrator keeps about one of its machines. What
// a task is was written on its container, as labels, when docker ran it; a
// machine keeps it here instead, which is how a node says what it is holding
// without asking anything that keeps records.
type record struct {
	// Execution is what the task was asked to be, as it was handed over.
	Execution task.Execution `json:"execution"`

	// Process is what the machine runs, as the image and the task say it
	// together.
	Process guest.Process `json:"process"`

	Hostname string `json:"hostname"`

	// Image is the image's root the machine boots, shared with every machine
	// that runs it, and Scratch the disk of its own it keeps its changes on.
	// A read-only task has none.
	Image   string `json:"image"`
	Scratch string `json:"scratch,omitempty"`

	VCPUs     int     `json:"vcpus"`
	CPUQuota  float64 `json:"cpu_quota,omitempty"`
	MemoryMiB int     `json:"memory_mib"`

	// Interfaces are the machine's network devices while it runs. They are
	// given when it starts and taken back when it stops, since what a network
	// hands out is only good while the network is.
	Interfaces []iface `json:"interfaces,omitempty"`

	Status       task.Status `json:"status"`
	ExitCode     int         `json:"exit_code"`
	RestartCount uint        `json:"restart_count"`
	CreatedAt    time.Time   `json:"created_at"`
	StartedAt    time.Time   `json:"started_at,omitzero"`
	FinishedAt   time.Time   `json:"finished_at,omitzero"`

	// Generation is which run of the task inside the machine is the current
	// one: a task started again in its machine is a new run of it.
	Generation uint64 `json:"generation"`

	// LogBase is what the agent's numbering of this boot's output starts
	// after. A machine that boots again numbers its output from one again, so
	// what it wrote is kept numbered after everything it wrote before.
	LogBase uint64 `json:"log_base"`

	// Stopped says the task was stopped on purpose, which no restart policy
	// undoes.
	Stopped bool `json:"stopped"`
}

// iface is one of a machine's network devices.
type iface struct {
	Network string `json:"network"`

	// Address is its own, in CIDR form, and MAC what the machine finds it by.
	Address string `json:"address"`
	MAC     string `json:"mac"`

	// Gateway is set on the one device a machine routes out through.
	Gateway string `json:"gateway,omitempty"`

	// Aliases are the names its neighbours on the network reach it by.
	Aliases []string `json:"aliases,omitempty"`
}

// running reports whether the machine's task is up, or on its way up again.
func (r record) running() bool {
	return r.Status == task.StatusRunning || r.Status == task.StatusRestarting
}

// execution is the record as the rest of the runner sees a run.
func (r record) execution() task.Execution {
	e := r.Execution

	e.Status = r.Status
	e.ExitCode = r.ExitCode
	e.RestartCount = r.RestartCount
	e.CreatedAt = r.CreatedAt
	e.StartedAt = r.StartedAt
	e.Endpoints = nil

	// a task is reached through its machine's own network, so it is
	// reachable while it runs on one, on every port it exposes.
	if r.Status == task.StatusRunning && len(r.Interfaces) > 0 {
		endpoints := make([]port.Port, 0, len(e.ExposedPorts))
		for p := range e.ExposedPorts {
			endpoints = append(endpoints, p)
		}

		slices.Sort(endpoints)
		e.Endpoints = endpoints
	}

	return e
}

// address is the machine's address on a network, without its prefix.
func (r record) address(network string) (string, bool) {
	for _, i := range r.Interfaces {
		if i.Network == network {
			ip, _, _ := cutPrefix(i.Address)

			return ip, true
		}
	}

	return "", false
}

// store keeps an orchestrator's machine records: one file each, beside the
// machine's disks and output, and all of them in memory, so that asking what a
// node holds reads no file at all.
type store struct {
	dir string

	lock    sync.RWMutex
	records map[string]record
}

// openStore reads back every record kept in dir.
func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	s := &store{dir: dir, records: make(map[string]record)}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		encoded, err := os.ReadFile(filepath.Join(dir, entry.Name(), recordName))
		if err != nil {
			continue
		}

		var r record
		if err := json.Unmarshal(encoded, &r); err != nil {
			return nil, fmt.Errorf("the record of machine %s cannot be read: %w", entry.Name(), err)
		}

		s.records[r.Execution.ID] = r
	}

	return s, nil
}

// machineDir is where one machine's record, disks and output are kept.
func (s *store) machineDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *store) get(id string) (record, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	r, found := s.records[id]

	return r, found
}

// all is every record, oldest first.
func (s *store) all() []record {
	s.lock.RLock()
	defer s.lock.RUnlock()

	records := make([]record, 0, len(s.records))
	for _, r := range s.records {
		records = append(records, r)
	}

	slices.SortFunc(records, func(a record, b record) int {
		return a.CreatedAt.Compare(b.CreatedAt)
	})

	return records
}

// put keeps a record, written anew in full so that it is never read half
// written.
func (s *store) put(r record) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.write(r)
}

// update changes one record under the store's lock, so that two changes to it
// never undo each other.
func (s *store) update(id string, change func(*record)) (record, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	r, found := s.records[id]
	if !found {
		return record{}, errNoMachine
	}

	change(&r)

	return r, s.write(r)
}

func (s *store) write(r record) error {
	dir := s.machineDir(r.Execution.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	encoded, err := json.Marshal(r)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(dir, ".record-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())

	if _, err := temporary.Write(encoded); err != nil {
		temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	if err := os.Rename(temporary.Name(), filepath.Join(dir, recordName)); err != nil {
		return err
	}

	s.records[r.Execution.ID] = r

	return nil
}

// remove lets go of a record, and of everything kept beside it.
func (s *store) remove(id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.records, id)

	if err := os.RemoveAll(s.machineDir(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
