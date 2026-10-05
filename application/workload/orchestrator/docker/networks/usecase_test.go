package networks

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

	backend := docker.Network{ID: "n1", Name: "backend", Driver: "bridge", Containers: []string{"web"}}

	testcases := []struct {
		name    string
		request noderequest.Request
		expect  func(daemon *mocks.MockDaemon)

		want     any
		wantCode noderequest.Code
	}{
		{
			name:    "the networks in a VM are listed",
			request: noderequest.Request{Op: noderequest.OpNetworksList, VMUUID: "vm-1"},
			expect:  func(d *mocks.MockDaemon) { d.On("Networks", mock.Anything).Return([]docker.Network{backend}, nil) },
			want:    []noderequest.Network{noderequest.NewNetwork(backend)},
		},
		{
			name:    "a network is created",
			request: noderequest.Request{Op: noderequest.OpNetworksCreate, VMUUID: "vm-1", Payload: json.RawMessage(`{"name":"backend","internal":true}`)},
			expect: func(d *mocks.MockDaemon) {
				d.On("CreateNetwork", mock.Anything, docker.NetworkSpec{Name: "backend", Internal: true}).Return(backend, nil)
			},
			want: noderequest.NewNetwork(backend),
		},
		{
			name:     "a network has a name",
			request:  noderequest.Request{Op: noderequest.OpNetworksCreate, VMUUID: "vm-1", Payload: json.RawMessage(`{}`)},
			wantCode: noderequest.CodeInvalid,
		},
		{
			name:    "a network is removed",
			request: noderequest.Request{Op: noderequest.OpNetworksRemove, VMUUID: "vm-1", Payload: json.RawMessage(`{"id":"n1"}`)},
			expect:  func(d *mocks.MockDaemon) { d.On("RemoveNetwork", mock.Anything, "n1").Return(nil) },
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
