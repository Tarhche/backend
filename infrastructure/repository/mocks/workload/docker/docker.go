// Package docker stands in for what runs inside a Docker VM: its dockerd, and
// docker compose.
package docker

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// MockDaemon stands in for the dockerd of one Docker VM.
type MockDaemon struct {
	mock.Mock
}

var _ docker.Daemon = &MockDaemon{}

func (m *MockDaemon) Ping(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockDaemon) Containers(ctx context.Context, filter docker.ContainerFilter) ([]docker.Container, error) {
	args := m.Called(ctx, filter)

	containers, _ := args.Get(0).([]docker.Container)

	return containers, args.Error(1)
}

func (m *MockDaemon) Container(ctx context.Context, id string) (docker.Container, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(docker.Container), args.Error(1)
}

func (m *MockDaemon) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (docker.Container, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(docker.Container), args.Error(1)
}

func (m *MockDaemon) StartContainer(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockDaemon) StopContainer(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockDaemon) RestartContainer(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockDaemon) RemoveContainer(ctx context.Context, id string, force bool) error {
	return m.Called(ctx, id, force).Error(0)
}

func (m *MockDaemon) ContainerLogs(ctx context.Context, id string, options docker.LogOptions) ([]docker.LogLine, error) {
	args := m.Called(ctx, id, options)

	lines, _ := args.Get(0).([]docker.LogLine)

	return lines, args.Error(1)
}

func (m *MockDaemon) ContainerStats(ctx context.Context, id string) (docker.Stats, error) {
	args := m.Called(ctx, id)

	return args.Get(0).(docker.Stats), args.Error(1)
}

func (m *MockDaemon) ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error {
	return m.Called(ctx, network, container, aliases).Error(0)
}

func (m *MockDaemon) DisconnectNetwork(ctx context.Context, network string, container string, force bool) error {
	return m.Called(ctx, network, container, force).Error(0)
}

func (m *MockDaemon) Images(ctx context.Context) ([]docker.Image, error) {
	args := m.Called(ctx)

	images, _ := args.Get(0).([]docker.Image)

	return images, args.Error(1)
}

func (m *MockDaemon) PullImage(ctx context.Context, reference string) (docker.Image, error) {
	args := m.Called(ctx, reference)

	return args.Get(0).(docker.Image), args.Error(1)
}

func (m *MockDaemon) RemoveImage(ctx context.Context, id string, force bool) error {
	return m.Called(ctx, id, force).Error(0)
}

func (m *MockDaemon) Networks(ctx context.Context) ([]docker.Network, error) {
	args := m.Called(ctx)

	networks, _ := args.Get(0).([]docker.Network)

	return networks, args.Error(1)
}

func (m *MockDaemon) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (docker.Network, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(docker.Network), args.Error(1)
}

func (m *MockDaemon) RemoveNetwork(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockDaemon) Volumes(ctx context.Context) ([]docker.Volume, error) {
	args := m.Called(ctx)

	volumes, _ := args.Get(0).([]docker.Volume)

	return volumes, args.Error(1)
}

func (m *MockDaemon) CreateVolume(ctx context.Context, spec docker.VolumeSpec) (docker.Volume, error) {
	args := m.Called(ctx, spec)

	return args.Get(0).(docker.Volume), args.Error(1)
}

func (m *MockDaemon) RemoveVolume(ctx context.Context, name string, force bool) error {
	return m.Called(ctx, name, force).Error(0)
}
