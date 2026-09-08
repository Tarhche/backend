package getEndpoint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
)

// running builds a container this node is holding, with the ports docker
// published it on.
func running(published map[port.Port]port.Port) container.Container {
	bindings := make(port.PortMap, len(published))
	for containerPort, hostPort := range published {
		bindings[containerPort] = []port.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	return container.Container{
		ID:           "container-1",
		Status:       container.StatusRunning,
		PortBindings: bindings,
		Labels:       map[string]string{container.TaskKindLabelKey: "service"},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a bare request reaches the lowest published port", func(t *testing.T) {
		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{running(map[port.Port]port.Port{8080: 32769, 80: 32768})}, nil)
		defer containerManager.AssertExpectations(t)

		response, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{Host: "docker", Port: 32768}, response)
		assert.Equal(t, "docker:32768", response.Address())
	})

	t.Run("a named port reaches that port", func(t *testing.T) {
		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{running(map[port.Port]port.Port{80: 32768, 8080: 32769})}, nil)
		defer containerManager.AssertExpectations(t)

		response, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 8080})

		assert.NoError(t, err)
		assert.Equal(t, &Response{Host: "docker", Port: 32769}, response)
	})

	t.Run("a port the container does not expose is not there", func(t *testing.T) {
		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{running(map[port.Port]port.Port{80: 32768})}, nil)
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 9999})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a container that publishes nothing cannot be reached", func(t *testing.T) {
		c := running(nil)
		c.PortBindings = port.PortMap{80: []port.PortBinding{{HostPort: 0}}}

		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{c}, nil)
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a container this node is not holding says so", func(t *testing.T) {
		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{}, nil)
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotHeld)
	})

	t.Run("a container that has stopped says so", func(t *testing.T) {
		c := running(map[port.Port]port.Port{80: 32768})
		c.Status = container.StatusExited

		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container{c}, nil)
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotRunning)
	})

	t.Run("docker failing fails the use case", func(t *testing.T) {
		expected := errors.New("docker is not answering")

		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, container.TaskSlugLabelKey, "nginx-xkfqz").Once().
			Return([]container.Container(nil), expected)
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, expected)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Run("a request naming a container is valid", func(t *testing.T) {
		assert.Empty(t, (&Request{Slug: "nginx-xkfqz"}).Validate())
	})

	t.Run("a request naming none is not", func(t *testing.T) {
		assert.Equal(t, "required_field", (&Request{}).Validate()["slug"])
	})
}
