package runTask

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request",
			request: Request{
				Name:          "test-task",
				Image:         "test-image:latest",
				AutoRemove:    true,
				PortBindings:  map[uint][]PortBinding{},
				RestartPolicy: "always",
				RestartCount:  3,
				HealthCheck:   "http://localhost:8080/health",
				AttachStdin:   false,
				AttachStdout:  true,
				AttachStderr:  true,
				Environment:   []string{"ENV=test"},
				Command:       []string{"echo", "hello"},
				Entrypoint:    []string{"/bin/sh"},
				Mounts:        []Mount{},
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 1024,
					Disk:   2048,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				Name:  "",
				Image: "test-image:latest",
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 1024,
					Disk:   2048,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty image",
			request: Request{
				Name:  "test-task",
				Image: "",
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 1024,
					Disk:   2048,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{
				"image": "required_field",
			},
		},
		{
			name: "invalid request with zero cpu",
			request: Request{
				Name:  "test-task",
				Image: "test-image:latest",
				ResourceLimits: ResourceLimits{
					Cpu:    0,
					Memory: 1024,
					Disk:   2048,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{
				"resource_limits.cpu": "required_field",
			},
		},
		{
			name: "invalid request with zero memory",
			request: Request{
				Name:  "test-task",
				Image: "test-image:latest",
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 0,
					Disk:   2048,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{
				"resource_limits.memory": "required_field",
			},
		},
		{
			name: "invalid request with zero disk",
			request: Request{
				Name:  "test-task",
				Image: "test-image:latest",
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 1024,
					Disk:   0,
				},
				OwnerUUID: "owner-uuid-123",
			},
			want: domain.ValidationErrors{
				"resource_limits.disk": "required_field",
			},
		},
		{
			name: "invalid request with empty owner uuid",
			request: Request{
				Name:  "test-task",
				Image: "test-image:latest",
				ResourceLimits: ResourceLimits{
					Cpu:    1.0,
					Memory: 1024,
					Disk:   2048,
				},
				OwnerUUID: "",
			},
			want: domain.ValidationErrors{
				"owner_uuid": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Name:  "",
				Image: "",
				ResourceLimits: ResourceLimits{
					Cpu:    0,
					Memory: 0,
					Disk:   0,
				},
				OwnerUUID: "",
			},
			want: domain.ValidationErrors{
				"name":                   "required_field",
				"image":                  "required_field",
				"resource_limits.cpu":    "required_field",
				"resource_limits.memory": "required_field",
				"resource_limits.disk":   "required_field",
				"owner_uuid":             "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Validate()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRequest_ConvertMounts(t *testing.T) {
	req := Request{
		Mounts: []Mount{
			{
				Source:   "/host/path",
				Target:   "/container/path",
				Type:     "bind",
				ReadOnly: true,
			},
		},
	}

	mounts := req.ConvertMounts()

	assert.Len(t, mounts, 1)

	assert.Equal(t, "/host/path", mounts[0].Source)
}

func TestRequest_ConvertPortBindings(t *testing.T) {
	req := Request{
		PortBindings: map[uint][]PortBinding{
			8080: {
				{
					HostIP:   "0.0.0.0",
					HostPort: 9090,
				},
			},
		},
	}

	portMaps := req.ConvertPortBindings()

	assert.Len(t, portMaps, 1)
}
