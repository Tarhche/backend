package volumes

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

	data := docker.Volume{Name: "data", Driver: "local", InUse: true}

	testcases := []struct {
		name    string
		request noderequest.Request
		expect  func(daemon *mocks.MockDaemon)

		want     any
		wantCode noderequest.Code
	}{
		{
			name:    "the volumes in a VM are listed",
			request: noderequest.Request{Op: noderequest.OpVolumesList, VMUUID: "vm-1"},
			expect:  func(d *mocks.MockDaemon) { d.On("Volumes", mock.Anything).Return([]docker.Volume{data}, nil) },
			want:    []noderequest.Volume{noderequest.NewVolume(data)},
		},
		{
			name:    "a volume asked for with no name is named by docker",
			request: noderequest.Request{Op: noderequest.OpVolumesCreate, VMUUID: "vm-1"},
			expect: func(d *mocks.MockDaemon) {
				d.On("CreateVolume", mock.Anything, docker.VolumeSpec{}).Return(docker.Volume{Name: "3f2a"}, nil)
			},
			want: noderequest.NewVolume(docker.Volume{Name: "3f2a"}),
		},
		{
			name:    "a volume is removed by its name",
			request: noderequest.Request{Op: noderequest.OpVolumesRemove, VMUUID: "vm-1", Payload: json.RawMessage(`{"id":"data"}`)},
			expect:  func(d *mocks.MockDaemon) { d.On("RemoveVolume", mock.Anything, "data", false).Return(nil) },
		},
		{
			name:     "removing names what to remove",
			request:  noderequest.Request{Op: noderequest.OpVolumesRemove, VMUUID: "vm-1", Payload: json.RawMessage(`{}`)},
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
