package images

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	mocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

type one struct {
	daemon docker.Daemon
}

func (o one) Daemon(string) docker.Daemon {
	return o.daemon
}

func TestUseCase_Answer(t *testing.T) {
	t.Parallel()

	nginx := docker.Image{ID: "sha256:nginx", Tags: []string{"nginx:alpine"}, Size: 42 << 20, InUse: true}

	testcases := []struct {
		name    string
		request noderequest.Request
		expect  func(daemon *mocks.MockDaemon)

		want     any
		wantCode noderequest.Code
	}{
		{
			name:    "the images a VM holds are listed",
			request: noderequest.Request{Op: noderequest.OpImagesList, VMUUID: "vm-1"},
			expect:  func(d *mocks.MockDaemon) { d.On("Images", mock.Anything).Return([]docker.Image{nginx}, nil) },
			want:    []noderequest.Image{noderequest.NewImage(nginx)},
		},
		{
			name:    "an image is pulled",
			request: noderequest.Request{Op: noderequest.OpImagesPull, VMUUID: "vm-1", Payload: json.RawMessage(`{"reference":"nginx:alpine"}`)},
			expect:  func(d *mocks.MockDaemon) { d.On("PullImage", mock.Anything, "nginx:alpine").Return(nginx, nil) },
			want:    noderequest.NewImage(nginx),
		},
		{
			name:     "a pull names what to pull",
			request:  noderequest.Request{Op: noderequest.OpImagesPull, VMUUID: "vm-1", Payload: json.RawMessage(`{}`)},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name:    "an image is removed",
			request: noderequest.Request{Op: noderequest.OpImagesRemove, VMUUID: "vm-1", Payload: json.RawMessage(`{"id":"sha256:nginx","force":true}`)},
			expect:  func(d *mocks.MockDaemon) { d.On("RemoveImage", mock.Anything, "sha256:nginx", true).Return(nil) },
		},
		{
			name: "a pull docker refuses is refused in docker's words",
			request: noderequest.Request{
				Op: noderequest.OpImagesPull, VMUUID: "vm-1", Payload: json.RawMessage(`{"reference":"nothing:here"}`),
			},
			expect: func(d *mocks.MockDaemon) {
				d.On("PullImage", mock.Anything, "nothing:here").Return(docker.Image{}, docker.ErrInvalid)
			},
			wantCode: noderequest.CodeInvalid,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var daemon mocks.MockDaemon
			if tt.expect != nil {
				tt.expect(&daemon)
			}
			defer daemon.AssertExpectations(t)

			result, _, err := NewUseCase(one{daemon: &daemon}).Answer(t.Context(), tt.request)
			if len(tt.wantCode) > 0 {
				assert.Equal(t, tt.wantCode, noderequest.ErrorOf(err).Code)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}
