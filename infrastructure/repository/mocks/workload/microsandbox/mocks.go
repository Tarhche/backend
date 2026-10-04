package microsandbox

import (
	"context"
	"io"
	"syscall"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// MockSandboxes stands in for microsandbox.
type MockSandboxes struct {
	mock.Mock
}

var _ runs.Sandboxes = &MockSandboxes{}

func (m *MockSandboxes) Check(ctx context.Context) (runs.Versions, error) {
	args := m.Called(ctx)

	return args.Get(0).(runs.Versions), args.Error(1)
}

func (m *MockSandboxes) Pull(ctx context.Context, reference string) error {
	return m.Called(ctx, reference).Error(0)
}

func (m *MockSandboxes) Image(ctx context.Context, reference string) (runs.ImageConfig, bool, error) {
	args := m.Called(ctx, reference)

	return args.Get(0).(runs.ImageConfig), args.Bool(1), args.Error(2)
}

func (m *MockSandboxes) Create(ctx context.Context, spec runs.SandboxSpec) (runs.Sandbox, error) {
	args := m.Called(ctx, spec)

	sandbox, _ := args.Get(0).(runs.Sandbox)

	return sandbox, args.Error(1)
}

func (m *MockSandboxes) Start(ctx context.Context, name string) (runs.Sandbox, error) {
	args := m.Called(ctx, name)

	sandbox, _ := args.Get(0).(runs.Sandbox)

	return sandbox, args.Error(1)
}

func (m *MockSandboxes) Connect(ctx context.Context, name string) (runs.Sandbox, error) {
	args := m.Called(ctx, name)

	sandbox, _ := args.Get(0).(runs.Sandbox)

	return sandbox, args.Error(1)
}

func (m *MockSandboxes) Stop(ctx context.Context, name string, timeout time.Duration) error {
	return m.Called(ctx, name, timeout).Error(0)
}

func (m *MockSandboxes) Remove(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}

func (m *MockSandboxes) List(ctx context.Context, labels map[string]string) ([]runs.SandboxInfo, error) {
	args := m.Called(ctx, labels)

	infos, _ := args.Get(0).([]runs.SandboxInfo)

	return infos, args.Error(1)
}

func (m *MockSandboxes) Metrics(ctx context.Context) (map[string]runs.Metrics, error) {
	args := m.Called(ctx)

	metrics, _ := args.Get(0).(map[string]runs.Metrics)

	return metrics, args.Error(1)
}

// MockSandbox stands in for a handle on a running sandbox.
type MockSandbox struct {
	mock.Mock
}

var _ runs.Sandbox = &MockSandbox{}

func (m *MockSandbox) Name() string {
	return m.Called().String(0)
}

func (m *MockSandbox) Exec(ctx context.Context, command runs.Command) (runs.Process, error) {
	args := m.Called(ctx, command)

	process, _ := args.Get(0).(runs.Process)

	return process, args.Error(1)
}

func (m *MockSandbox) Close() error {
	return m.Called().Error(0)
}

// MockProcess stands in for a command running in a guest.
type MockProcess struct {
	mock.Mock
}

var _ runs.Process = &MockProcess{}

func (m *MockProcess) Events() <-chan runs.Event {
	args := m.Called()

	events, _ := args.Get(0).(<-chan runs.Event)

	return events
}

func (m *MockProcess) Stdin() io.WriteCloser {
	args := m.Called()

	stdin, _ := args.Get(0).(io.WriteCloser)

	return stdin
}

func (m *MockProcess) Signal(ctx context.Context, signal syscall.Signal) error {
	return m.Called(ctx, signal).Error(0)
}

func (m *MockProcess) Resize(ctx context.Context, rows, cols uint16) error {
	return m.Called(ctx, rows, cols).Error(0)
}

func (m *MockProcess) Close() error {
	return m.Called().Error(0)
}

// MockRecords stands in for the run records.
type MockRecords struct {
	mock.Mock
}

var _ runs.Records = &MockRecords{}

func (m *MockRecords) Save(record runs.Record) error {
	return m.Called(record).Error(0)
}

func (m *MockRecords) Load() ([]runs.Record, error) {
	args := m.Called()

	records, _ := args.Get(0).([]runs.Record)

	return records, args.Error(1)
}

func (m *MockRecords) Delete(id string) error {
	return m.Called(id).Error(0)
}

// MockJournal stands in for the runs' journals.
type MockJournal struct {
	mock.Mock
}

var _ runs.Journal = &MockJournal{}

func (m *MockJournal) Append(id string, lines []api.LogLine) error {
	return m.Called(id, lines).Error(0)
}

func (m *MockJournal) Reader(id string, since time.Time) (runs.JournalReader, error) {
	args := m.Called(id, since)

	reader, _ := args.Get(0).(runs.JournalReader)

	return reader, args.Error(1)
}

func (m *MockJournal) Last(id string) (time.Time, error) {
	args := m.Called(id)

	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockJournal) Delete(id string) error {
	return m.Called(id).Error(0)
}

// MockJournalReader stands in for a reader of one run's journal.
type MockJournalReader struct {
	mock.Mock
}

var _ runs.JournalReader = &MockJournalReader{}

func (m *MockJournalReader) Next() (api.LogLine, error) {
	args := m.Called()

	return args.Get(0).(api.LogLine), args.Error(1)
}

func (m *MockJournalReader) Close() error {
	return m.Called().Error(0)
}

// MockHostPorts stands in for the host ports' allocator.
type MockHostPorts struct {
	mock.Mock
}

var _ runs.HostPorts = &MockHostPorts{}

func (m *MockHostPorts) Allocate(id string, count int) ([]uint16, error) {
	args := m.Called(id, count)

	ports, _ := args.Get(0).([]uint16)

	return ports, args.Error(1)
}

func (m *MockHostPorts) Hold(id string, ports []uint16) {
	m.Called(id, ports)
}

func (m *MockHostPorts) Release(id string) {
	m.Called(id)
}
