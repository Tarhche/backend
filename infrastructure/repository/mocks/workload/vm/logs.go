package vm

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// MockLogStore stands in for what keeps VMs' output.
type MockLogStore struct {
	mock.Mock
}

var _ vm.LogStore = &MockLogStore{}

func (m *MockLogStore) Writer(id string) (vm.LogWriter, error) {
	args := m.Called(id)

	writer, _ := args.Get(0).(vm.LogWriter)

	return writer, args.Error(1)
}

func (m *MockLogStore) Reader(id string) vm.LogReader {
	reader, _ := m.Called(id).Get(0).(vm.LogReader)

	return reader
}

// MockLogWriter stands in for what keeps more of one VM's output.
type MockLogWriter struct {
	mock.Mock
}

var _ vm.LogWriter = &MockLogWriter{}

func (m *MockLogWriter) Last() uint64 {
	return m.Called().Get(0).(uint64)
}

func (m *MockLogWriter) Add(line vm.LogLine) error {
	return m.Called(line).Error(0)
}

func (m *MockLogWriter) Changed() <-chan struct{} {
	changed, _ := m.Called().Get(0).(chan struct{})

	return changed
}

func (m *MockLogWriter) Close() error {
	return m.Called().Error(0)
}

// MockLogReader stands in for what reads one VM's output.
type MockLogReader struct {
	mock.Mock
}

var _ vm.LogReader = &MockLogReader{}

func (m *MockLogReader) Next(emit func(vm.LogLine) error) error {
	return m.Called(emit).Error(0)
}

// MockUsageReader stands in for what counts a machine's use on the host.
type MockUsageReader struct {
	mock.Mock
}

var _ vm.UsageReader = &MockUsageReader{}

func (m *MockUsageReader) Usage(ctx context.Context, machine vm.Machine, devices []string) (vm.Usage, error) {
	args := m.Called(ctx, machine, devices)

	return args.Get(0).(vm.Usage), args.Error(1)
}
