package microsandbox

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// MemoryRecords keeps run records in memory.
type MemoryRecords struct {
	mu      sync.Mutex
	records map[string]runs.Record
	fail    error
	saves   int
}

var _ runs.Records = &MemoryRecords{}

// NewMemoryRecords holds records, as a service that went away left them.
func NewMemoryRecords(records ...runs.Record) *MemoryRecords {
	m := &MemoryRecords{records: make(map[string]runs.Record)}

	for _, record := range records {
		m.records[record.ID] = record
	}

	return m
}

// Fail is what every save fails with from then on, until it is given nil.
func (m *MemoryRecords) Fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.fail = err
}

func (m *MemoryRecords) Save(record runs.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.fail != nil {
		return m.fail
	}

	m.saves++
	m.records[record.ID] = record

	return nil
}

func (m *MemoryRecords) Load() ([]runs.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	records := slices.Collect(maps.Values(m.records))

	slices.SortFunc(records, func(a, b runs.Record) int { return a.CreatedAt.Compare(b.CreatedAt) })

	return records, nil
}

func (m *MemoryRecords) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.records, id)

	return nil
}

// Get is a run's record as it was last saved.
func (m *MemoryRecords) Get(id string) (runs.Record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, found := m.records[id]

	return record, found
}

// Len is how many records are kept.
func (m *MemoryRecords) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.records)
}

// MemoryJournal keeps run journals in memory.
type MemoryJournal struct {
	mu    sync.Mutex
	lines map[string][]api.LogLine
}

var _ runs.Journal = &MemoryJournal{}

func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{lines: make(map[string][]api.LogLine)}
}

func (m *MemoryJournal) Append(id string, lines []api.LogLine) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lines[id] = append(m.lines[id], lines...)

	return nil
}

func (m *MemoryJournal) Reader(id string, since time.Time) (runs.JournalReader, error) {
	return &memoryReader{journal: m, id: id, since: since}, nil
}

func (m *MemoryJournal) Last(id string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lines := m.lines[id]
	if len(lines) == 0 {
		return time.Time{}, nil
	}

	return lines[len(lines)-1].At, nil
}

func (m *MemoryJournal) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.lines, id)

	return nil
}

// Lines are a run's lines, in order.
func (m *MemoryJournal) Lines(id string) []api.LogLine {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.lines[id])
}

// Has is whether a run has a journal.
func (m *MemoryJournal) Has(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, found := m.lines[id]

	return found
}

type memoryReader struct {
	journal *MemoryJournal
	id      string
	since   time.Time
	next    int
}

func (r *memoryReader) Next() (api.LogLine, error) {
	r.journal.mu.Lock()
	defer r.journal.mu.Unlock()

	lines := r.journal.lines[r.id]

	for r.next < len(lines) {
		line := lines[r.next]
		r.next++

		if !line.At.Before(r.since) {
			return line, nil
		}
	}

	return api.LogLine{}, io.EOF
}

func (r *memoryReader) Close() error {
	return nil
}

// MemoryHostPorts hands out host ports from a range, in order, without
// binding anything.
type MemoryHostPorts struct {
	mu       sync.Mutex
	next     uint16
	last     uint16
	held     map[string][]uint16
	released []uint16
}

var _ runs.HostPorts = &MemoryHostPorts{}

func NewMemoryHostPorts(first, last uint16) *MemoryHostPorts {
	return &MemoryHostPorts{next: first, last: last, held: make(map[string][]uint16)}
}

// Allocate hands out the next ports of the range that no run holds.
func (m *MemoryHostPorts) Allocate(id string, count int) ([]uint16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	held := make(map[uint16]bool)
	for _, ports := range m.held {
		for _, port := range ports {
			held[port] = true
		}
	}

	ports := make([]uint16, 0, count)

	for ; len(ports) < count && m.next <= m.last; m.next++ {
		if !held[m.next] {
			ports = append(ports, m.next)
		}
	}

	if len(ports) < count {
		return nil, fmt.Errorf("only %d host ports are left", len(ports))
	}

	m.held[id] = append(m.held[id], ports...)

	return ports, nil
}

func (m *MemoryHostPorts) Hold(id string, ports []uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.held[id] = append(m.held[id], ports...)
}

func (m *MemoryHostPorts) Release(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.released = append(m.released, m.held[id]...)
	delete(m.held, id)
}

// Held are the ports a run holds.
func (m *MemoryHostPorts) Held(id string) []uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.held[id])
}

// Released are the ports let go of, in order.
func (m *MemoryHostPorts) Released() []uint16 {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.released)
}
