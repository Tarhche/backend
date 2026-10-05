package attachVM

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// opened keeps what was exec'd into a VM.
type opened struct {
	command []string
	tty     bool
}

func holding(t *testing.T, labels map[string]string, stopped bool, ran chan<- opened) *memory.Engine {
	t.Helper()

	e := memory.New(memory.WithExec(func(_ context.Context, _ string, options vm.ExecOptions, stdin io.Reader, _ io.Writer, _ io.Writer) int {
		ran <- opened{command: options.Command, tty: options.TTY}
		_, _ = io.Copy(io.Discard, stdin)

		return 0
	}))

	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04", Labels: labels})
	require.NoError(t, err)

	if stopped {
		require.NoError(t, e.Stop(t.Context(), "vm-1"))
	}

	return e
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	owned := map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelOwner: "owner-uuid"}

	testcases := []struct {
		name    string
		labels  map[string]string
		stopped bool
		request Request

		wantErr        error
		wantValidation domain.ValidationErrors
	}{
		{
			name:    "a terminal is opened in the owner's own VM",
			labels:  owned,
			request: Request{UUID: "vm-1", OwnerUUID: "owner-uuid"},
		},
		{
			name:    "somebody else's VM is not there for them",
			labels:  owned,
			request: Request{UUID: "vm-1", OwnerUUID: "somebody-else"},
			wantErr: domain.ErrNotExists,
		},
		{
			name:    "nobody is not the owner",
			labels:  owned,
			request: Request{UUID: "vm-1"},
			wantErr: domain.ErrNotExists,
		},
		{
			name:    "a VM that says no owner is opened for nobody",
			labels:  map[string]string{vm.LabelPurpose: vm.PurposeVM},
			request: Request{UUID: "vm-1"},
			wantErr: domain.ErrNotExists,
		},
		{
			name:    "a task's instance is not a VM to open here",
			labels:  map[string]string{vm.LabelPurpose: vm.PurposeTask, vm.LabelOwner: "owner-uuid"},
			request: Request{UUID: "vm-1", OwnerUUID: "owner-uuid"},
			wantErr: domain.ErrNotExists,
		},
		{
			name:    "a VM that is not here is not there",
			labels:  owned,
			request: Request{UUID: "vm-2", OwnerUUID: "owner-uuid"},
			wantErr: domain.ErrNotExists,
		},
		{
			name:           "the owner's VM has to be running",
			labels:         owned,
			stopped:        true,
			request:        Request{UUID: "vm-1", OwnerUUID: "owner-uuid"},
			wantValidation: domain.ValidationErrors{"uuid": "vm_is_not_running"},
		},
		{
			name:           "a request naming no VM is refused",
			labels:         owned,
			request:        Request{OwnerUUID: "owner-uuid"},
			wantValidation: domain.ValidationErrors{"uuid": "required_field"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ran := make(chan opened, 1)
			e := holding(t, tt.labels, tt.stopped, ran)

			session, validationErrors, err := NewUseCase(e, validates{}).Execute(t.Context(), &tt.request)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, session)
				assert.Empty(t, ran, "nothing is opened for somebody who may not")

				return
			}

			require.NoError(t, err)

			if tt.wantValidation != nil {
				assert.Equal(t, tt.wantValidation, validationErrors)
				assert.Nil(t, session)

				return
			}

			require.NotNil(t, session)
			defer session.Close()

			shell := <-ran
			assert.True(t, shell.tty, "an interactive shell needs a terminal")
			assert.Equal(t, []string{"/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash; fi; exec /bin/sh"}, shell.command)
		})
	}
}
