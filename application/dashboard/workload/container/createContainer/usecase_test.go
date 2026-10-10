package createContainer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator())
}

// read is a request the way the dashboard's form sends one, from whoever is
// signed in.
func read(t *testing.T, body string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))

	request.OwnerUUID = workloadtest.OwnerUUID

	return &request
}

func created(made bool) workloadControlPlane.CreatedContainer {
	return workloadControlPlane.CreatedContainer{
		VM:        workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1", Created: made},
		Container: docker.Container{ID: "c0ffee", Name: "web", Image: "nginx:1.27", State: "running", Status: "Up 1 second", CreatedAt: workloadtest.At},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("the container as it was asked for, in the Docker VM it named", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.ContainerRequest{
			VM: workloadControlPlane.DockerVMChoice{UUID: "vm-uuid"},
			Container: docker.ContainerSpec{
				Name:          "web",
				Image:         "nginx:1.27",
				Command:       []string{"nginx", "-g", "daemon off;"},
				Env:           []string{"TZ=UTC"},
				WorkingDir:    "/srv",
				Ports:         []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}, {ContainerPort: 53, Protocol: "udp"}},
				Mounts:        []docker.Mount{{Type: "volume", Source: "data", Target: "/data"}, {Type: "tmpfs", Target: "/tmp", ReadOnly: false}},
				Networks:      []string{"backend"},
				RestartPolicy: "unless-stopped",
				CPUs:          0.5,
				Memory:        256 << 20,
			},
		}).Once().Return(created(false), nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), read(t, `{
			"vm_uuid": "vm-uuid",
			"name": "web",
			"image": "nginx:1.27",
			"command": ["nginx", "-g", "daemon off;"],
			"env": ["TZ=UTC"],
			"working_dir": "/srv",
			"ports": [{"container_port": 80, "host_port": 8080, "protocol": "tcp"}, {"container_port": 53, "protocol": "udp"}],
			"mounts": [{"type": "volume", "source": "data", "target": "/data"}, {"type": "tmpfs", "target": "/tmp"}],
			"networks": ["backend"],
			"restart_policy": "unless-stopped",
			"cpus": 0.5,
			"memory": 268435456
		}`))
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		// which VM it went into, and whether it was made for it, beside the
		// container itself.
		assert.JSONEq(t, `{
			"vm": {"uuid": "vm-uuid", "name": "docker-1", "created": false},
			"container": {
				"id": "c0ffee",
				"name": "web",
				"image": "nginx:1.27",
				"state": "running",
				"status": "Up 1 second",
				"command": "",
				"ports": [],
				"networks": [],
				"mounts": [],
				"created_at": "2026-10-04T12:00:00Z"
			}
		}`, string(presented))
	})

	t.Run("a Docker VM described is made for it, with the rest left to the defaults", func(t *testing.T) {
		t.Parallel()

		var asked workloadControlPlane.ContainerRequest

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().
			Run(func(args mock.Arguments) { asked = args.Get(2).(workloadControlPlane.ContainerRequest) }).
			Return(created(true), nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), read(t, `{
			"image": "nginx",
			"vm": {"name": "docker-2", "resources": {"cpus": 4}, "ports": [443, 80], "network": {"egress": "deny"}}
		}`))
		require.NoError(t, err)

		assert.Equal(t, workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{
			Name:      "docker-2",
			Resources: &vm.Resources{CPUs: 4},
			Ports:     []port.Port{80, 443},
			Network:   &vm.Network{Egress: vm.AccessDeny},
		}}, asked.VM)

		require.NotNil(t, response.VM)
		assert.True(t, response.VM.Created)
	})

	t.Run("neither asks the workload for a new one with the defaults", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.ContainerRequest{
			Container: docker.ContainerSpec{Image: "nginx"},
		}).Once().Return(created(true), nil)
		defer workload.AssertExpectations(t)

		_, err := useCase(&workload).Execute(context.Background(), read(t, `{"image": "nginx"}`))
		require.NoError(t, err)
	})

	t.Run("a request the rules refuse never reaches the workload", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), read(t, `{
			"vm_uuid": "vm-uuid",
			"vm": {},
			"name": "-web",
			"image": "",
			"ports": [{"container_port": 0}, {"container_port": 80, "host_port": 8080}, {"container_port": 81, "host_port": 8080, "protocol": "tcp"}, {"container_port": 82, "protocol": "sctp"}],
			"mounts": [{"type": "nfs", "target": "/data"}, {"type": "bind", "target": "/data"}, {"type": "volume", "source": "data", "target": "data"}],
			"restart_policy": "sometimes",
			"cpus": -1
		}`))
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"vm":                     "name a Docker VM or describe a new one, not both",
			"name":                   "may hold only letters, digits, dots, dashes and underscores, and starts with a letter or a digit",
			"image":                  "this field is required",
			"ports.0.container_port": "a port is a number from 1 to 65535",
			"ports.2.host_port":      "the same port is listed more than once",
			"ports.3.protocol":       "the protocol must be either tcp or udp",
			"mounts.0.type":          "a mount's type must be volume, bind or tmpfs",
			"mounts.1":               "a mount needs the path it is mounted at and, unless it is a tmpfs, what is mounted there",
			"mounts.2":               "a mount needs the path it is mounted at and, unless it is a tmpfs, what is mounted there",
			"restart_policy":         "the restart policy must be one of: no, always, unless-stopped, on-failure",
			"cpus":                   "the provided value is invalid",
		}, response.ValidationErrors)

		workload.AssertNotCalled(t, "CreateContainer", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("the same host port for another protocol is another port", func(t *testing.T) {
		t.Parallel()

		request := read(t, `{"image": "dns", "ports": [{"container_port": 53, "host_port": 53}, {"container_port": 53, "host_port": 53, "protocol": "udp"}, {"container_port": 54}, {"container_port": 55}]}`)

		assert.Empty(t, request.Validate(), "and docker picks a free host port for each that names none")
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(workloadControlPlane.CreatedContainer{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), read(t, `{"image": "nginx"}`))

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.VM)
			assert.Nil(t, response.Container)
		})
	}

	t.Run("a Docker VM no node has room for is the control plane's to say", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(
			workloadControlPlane.CreatedContainer{},
			&client.ValidationError{ValidationErrors: domain.ValidationErrors{"vm": "no_capacity"}},
		)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), read(t, `{"image": "nginx"}`))
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"vm": "there is no room for it on any node right now, try again later"}, response.ValidationErrors)
	})

	// the control plane names the VM to use vm.uuid, which this request
	// names vm_uuid; both name the VM to make vm.
	for name, tt := range map[string]struct {
		body    string
		refused domain.ValidationErrors
		want    domain.ValidationErrors
	}{
		"the vm it names is refused where it named it": {
			body:    `{"image": "nginx", "vm_uuid": "machine-uuid"}`,
			refused: domain.ValidationErrors{"vm.uuid": "not_docker"},
			want:    domain.ValidationErrors{"vm_uuid": "this VM is not a Docker VM"},
		},
		"the vm it describes is refused where it described it": {
			body:    `{"image": "nginx", "vm": {"resources": {"memory": 1048576}}}`,
			refused: domain.ValidationErrors{"vm.resources.memory": "too_small"},
			want:    domain.ValidationErrors{"vm.resources.memory": "this is smaller than allowed"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("CreateContainer", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(
				workloadControlPlane.CreatedContainer{},
				&client.ValidationError{ValidationErrors: tt.refused},
			)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), read(t, tt.body))
			require.NoError(t, err)

			assert.Equal(t, tt.want, response.ValidationErrors)
		})
	}
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValidationErrors{"image": "required_field"}, (&Request{}).Validate(), "an image is all a container needs")
}
