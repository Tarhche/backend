// Package manager stands in for the runner, as the rest of the application
// sees it.
package manager

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type MockClient struct {
	mock.Mock
}

var _ runnerManager.Client = &MockClient{}

func (m *MockClient) Containers(ctx context.Context, ownerUUID string, page uint) (runnerManager.Page[task.Task], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(runnerManager.Page[task.Task]), args.Error(1)
}

func (m *MockClient) Container(ctx context.Context, uuid string) (task.Task, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) ContainerOf(ctx context.Context, ownerUUID string, uuid string) (task.Task, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) RunContainer(ctx context.Context, spec runnerManager.ContainerSpec, ownerUUID string) (task.Task, error) {
	args := m.Called(ctx, spec, ownerUUID)

	return args.Get(0).(task.Task), args.Error(1)
}

func (m *MockClient) StopContainer(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) KillContainer(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) RestartContainer(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) DeleteContainer(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) ContainerLogs(ctx context.Context, uuid string, after time.Time, limit uint) ([]container.Log, error) {
	args := m.Called(ctx, uuid, after, limit)

	return args.Get(0).([]container.Log), args.Error(1)
}

func (m *MockClient) WatchContainers(ctx context.Context) (runnerManager.ContainerStream, error) {
	args := m.Called(ctx)

	stream, _ := args.Get(0).(runnerManager.ContainerStream)

	return stream, args.Error(1)
}

func (m *MockClient) FollowContainerLogs(ctx context.Context, uuid string, after time.Time) (runnerManager.LogStream, error) {
	args := m.Called(ctx, uuid, after)

	stream, _ := args.Get(0).(runnerManager.LogStream)

	return stream, args.Error(1)
}

func (m *MockClient) AttachContainer(ctx context.Context, uuid string, command []string) (runnerManager.Attachment, error) {
	args := m.Called(ctx, uuid, command)

	attachment, _ := args.Get(0).(runnerManager.Attachment)

	return attachment, args.Error(1)
}

func (m *MockClient) Stacks(ctx context.Context, ownerUUID string, page uint) (runnerManager.Page[runnerManager.Stack], error) {
	args := m.Called(ctx, ownerUUID, page)

	return args.Get(0).(runnerManager.Page[runnerManager.Stack]), args.Error(1)
}

func (m *MockClient) WatchStacks(ctx context.Context) (runnerManager.StackStream, error) {
	args := m.Called(ctx)

	stream, _ := args.Get(0).(runnerManager.StackStream)

	return stream, args.Error(1)
}

func (m *MockClient) Stack(ctx context.Context, uuid string) (runnerManager.Stack, error) {
	args := m.Called(ctx, uuid)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) StackOf(ctx context.Context, ownerUUID string, uuid string) (runnerManager.Stack, error) {
	args := m.Mock.Called(ctx, ownerUUID, uuid)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) RunStack(ctx context.Context, spec runnerManager.StackSpec, ownerUUID string) (runnerManager.Stack, error) {
	args := m.Called(ctx, spec, ownerUUID)

	return args.Get(0).(runnerManager.Stack), args.Error(1)
}

func (m *MockClient) StopStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) KillStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) RestartStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockClient) DeleteStack(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

// FakeAttachment is a command running inside a container, without a container.
// Writing to it records what was typed; the output it hands back is whatever it
// was given, and closing it ends the read the way a real session does.
type FakeAttachment struct {
	output chan []byte

	lock    sync.Mutex
	typed   []byte
	resizes [][2]uint
	closed  bool

	pending []byte
}

var _ runnerManager.Attachment = &FakeAttachment{}

func NewFakeAttachment() *FakeAttachment {
	return &FakeAttachment{output: make(chan []byte, 16)}
}

// Emit gives the reader something to read, as a command writing would.
func (a *FakeAttachment) Emit(data string) {
	a.output <- []byte(data)
}

func (a *FakeAttachment) Read(p []byte) (int, error) {
	if len(a.pending) == 0 {
		data, ok := <-a.output
		if !ok {
			return 0, io.EOF
		}

		a.pending = data
	}

	n := copy(p, a.pending)
	a.pending = a.pending[n:]

	return n, nil
}

func (a *FakeAttachment) Write(p []byte) (int, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.closed {
		return 0, io.ErrClosedPipe
	}

	a.typed = append(a.typed, p...)

	return len(p), nil
}

func (a *FakeAttachment) Resize(_ context.Context, rows uint, cols uint) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.resizes = append(a.resizes, [2]uint{rows, cols})

	return nil
}

func (a *FakeAttachment) Close() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if !a.closed {
		a.closed = true
		close(a.output)
	}

	return nil
}

// Typed is everything written to the command.
func (a *FakeAttachment) Typed() string {
	a.lock.Lock()
	defer a.lock.Unlock()

	return string(a.typed)
}

// Resizes is every window size the command was told about.
func (a *FakeAttachment) Resizes() [][2]uint {
	a.lock.Lock()
	defer a.lock.Unlock()

	return append([][2]uint(nil), a.resizes...)
}

// IsClosed reports whether the session has been ended.
func (a *FakeAttachment) IsClosed() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.closed
}

// FakeContainerStream is what happens to the containers, without a runner.
type FakeContainerStream struct {
	changes chan runnerManager.ContainerChange

	lock   sync.Mutex
	closed bool
}

var _ runnerManager.ContainerStream = &FakeContainerStream{}

func NewFakeContainerStream() *FakeContainerStream {
	return &FakeContainerStream{changes: make(chan runnerManager.ContainerChange, 16)}
}

// Emit gives the reader a change, as a container becoming something else would.
func (s *FakeContainerStream) Emit(change runnerManager.ContainerChange) {
	s.changes <- change
}

func (s *FakeContainerStream) Next(ctx context.Context) (runnerManager.ContainerChange, error) {
	select {
	case change, ok := <-s.changes:
		if !ok {
			return runnerManager.ContainerChange{}, io.EOF
		}

		return change, nil
	case <-ctx.Done():
		return runnerManager.ContainerChange{}, io.EOF
	}
}

func (s *FakeContainerStream) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.closed {
		s.closed = true
		close(s.changes)
	}

	return nil
}

func (s *FakeContainerStream) IsClosed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}

// FakeStackStream is what happens to the stacks, without a runner.
type FakeStackStream struct {
	changes chan runnerManager.StackChange

	lock   sync.Mutex
	closed bool
}

var _ runnerManager.StackStream = &FakeStackStream{}

func NewFakeStackStream() *FakeStackStream {
	return &FakeStackStream{changes: make(chan runnerManager.StackChange, 16)}
}

// Emit gives the reader a change, as a stack becoming something else would.
func (s *FakeStackStream) Emit(change runnerManager.StackChange) {
	s.changes <- change
}

func (s *FakeStackStream) Next(ctx context.Context) (runnerManager.StackChange, error) {
	select {
	case change, ok := <-s.changes:
		if !ok {
			return runnerManager.StackChange{}, io.EOF
		}

		return change, nil
	case <-ctx.Done():
		return runnerManager.StackChange{}, io.EOF
	}
}

func (s *FakeStackStream) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.closed {
		s.closed = true
		close(s.changes)
	}

	return nil
}

func (s *FakeStackStream) IsClosed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}

// FakeLogStream is a container's output, without a container.
type FakeLogStream struct {
	lines chan container.Log

	lock   sync.Mutex
	closed bool
}

var _ runnerManager.LogStream = &FakeLogStream{}

func NewFakeLogStream() *FakeLogStream {
	return &FakeLogStream{lines: make(chan container.Log, 16)}
}

// Emit gives the reader a line, as a container writing one would.
func (s *FakeLogStream) Emit(log container.Log) {
	s.lines <- log
}

func (s *FakeLogStream) Next(ctx context.Context) (container.Log, error) {
	select {
	case line, ok := <-s.lines:
		if !ok {
			return container.Log{}, io.EOF
		}

		return line, nil
	case <-ctx.Done():
		return container.Log{}, io.EOF
	}
}

func (s *FakeLogStream) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if !s.closed {
		s.closed = true
		close(s.lines)
	}

	return nil
}

func (s *FakeLogStream) IsClosed() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.closed
}
