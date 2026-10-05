package getEndpoint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// instance is something this node holds, under a slug, on ports.
func instance(id string, purpose string, slug string, ingress vm.Access, ports ...port.Port) vm.Spec {
	return vm.Spec{
		ID:      id,
		Kind:    vm.KindMachine,
		Image:   "ubuntu:24.04",
		Ports:   ports,
		Network: vm.Network{Ingress: ingress, Egress: vm.AccessDeny},
		Labels:  map[string]string{vm.LabelPurpose: purpose, vm.LabelSlug: slug},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		held    []vm.Spec
		stopped string
		request Request

		want    *Response
		wantErr error
	}{
		{
			name:    "a bare request reaches a VM's lowest port",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessAllow, 8080, 80)},
			request: Request{Slug: "box-abcde"},
			want:    &Response{Port: 80, Address: "vmhost-01:20001"},
		},
		{
			name:    "a named port reaches that port",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessAllow, 8080, 80)},
			request: Request{Slug: "box-abcde", Port: 8080},
			want:    &Response{Port: 8080, Address: "vmhost-01:20000"},
		},
		{
			name:    "a port the VM was not given is not exposed",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessAllow, 80)},
			request: Request{Slug: "box-abcde", Port: 9999},
			wantErr: ErrNotExposed,
		},
		{
			name:    "nothing of a VM whose ingress is denied is exposed",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessDeny, 80)},
			request: Request{Slug: "box-abcde", Port: 80},
			wantErr: ErrNotExposed,
		},
		{
			name:    "a stopped VM says so",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessAllow, 80)},
			stopped: "vm-1",
			request: Request{Slug: "box-abcde"},
			wantErr: ErrNotRunning,
		},
		{
			name:    "a live snippet is reached the same way, through the instance it runs in",
			held:    []vm.Spec{instance("execution-1", vm.PurposeTask, "snippet-abcde", vm.AccessAllow, 3000)},
			request: Request{Slug: "snippet-abcde"},
			want:    &Response{Port: 3000, Address: "vmhost-01:20000"},
		},
		{
			name: "of a task's instances the running one is reached",
			held: []vm.Spec{
				instance("execution-1", vm.PurposeTask, "snippet-abcde", vm.AccessAllow, 3000),
				instance("execution-2", vm.PurposeTask, "snippet-abcde", vm.AccessAllow, 3000),
			},
			stopped: "execution-1",
			request: Request{Slug: "snippet-abcde"},
			want:    &Response{Port: 3000, Address: "vmhost-01:20001"},
		},
		{
			name:    "a slug this node does not hold says so",
			held:    []vm.Spec{instance("vm-1", vm.PurposeVM, "box-abcde", vm.AccessAllow, 80)},
			request: Request{Slug: "other-fghij"},
			wantErr: ErrNotHeld,
		},
		{
			name:    "an instance that is neither a VM nor a task is nothing to reach",
			held:    []vm.Spec{instance("vm-1", "", "box-abcde", vm.AccessAllow, 80)},
			request: Request{Slug: "box-abcde"},
			wantErr: ErrNotHeld,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := memory.New(memory.WithHost("vmhost-01"))
			for _, spec := range tt.held {
				_, err := e.Create(t.Context(), spec)
				require.NoError(t, err)
			}

			if len(tt.stopped) > 0 {
				require.NoError(t, e.Stop(t.Context(), tt.stopped))
			}

			response, err := NewUseCase(e).Execute(t.Context(), &tt.request)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, response)
		})
	}

	t.Run("an engine that does not answer fails the lookup", func(t *testing.T) {
		t.Parallel()

		expected := errors.New("the vmhost is away")

		var e engine.MockEngine
		e.On("List", mock.Anything).Return(nil, expected)
		defer e.AssertExpectations(t)

		_, err := NewUseCase(&e).Execute(t.Context(), &Request{Slug: "box-abcde"})
		assert.ErrorIs(t, err, expected)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Empty(t, (&Request{Slug: "box-abcde"}).Validate())
	assert.Equal(t, "required_field", (&Request{}).Validate()["slug"])
}
