package noderequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestErrorOf(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		err  error
		want Code
	}{
		"something that is not there":        {err: domain.ErrNotExists, want: CodeNotFound},
		"wrapped, it is still not there":     {err: fmt.Errorf("container web: %w", domain.ErrNotExists), want: CodeNotFound},
		"a vm that is not running":           {err: vm.ErrNotRunning, want: CodeNotRunning},
		"a vm that is not a docker vm":       {err: vm.ErrNotDocker, want: CodeNotDocker},
		"a dockerd that did not come up":     {err: docker.ErrUnavailable, want: CodeDockerUnavailable},
		"a request docker refused":           {err: fmt.Errorf("%w: no such image: nginx:nope", docker.ErrInvalid), want: CodeInvalid},
		"a request that ran out of time":     {err: context.DeadlineExceeded, want: CodeTimeout},
		"anything else":                      {err: errors.New("the engine fell over"), want: CodeInternal},
		"an engine with no room is internal": {err: vm.ErrNoCapacity, want: CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			replied := ErrorOf(tt.err)

			require.NotNil(t, replied)
			assert.Equal(t, tt.want, replied.Code)
			assert.Equal(t, tt.err.Error(), replied.Message, "the message is the error's own words")
		})
	}

	t.Run("no error is no reply error", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, ErrorOf(nil))
	})

	t.Run("an error that already crossed the wire is passed on as it is", func(t *testing.T) {
		t.Parallel()

		arrived := &Error{Code: CodeInvalid, Message: "port is already allocated"}

		assert.Same(t, arrived, ErrorOf(fmt.Errorf("creating web: %w", arrived)))
	})
}

func TestError_Is(t *testing.T) {
	t.Parallel()

	for code, want := range map[Code]error{
		CodeNotFound:          domain.ErrNotExists,
		CodeNotRunning:        vm.ErrNotRunning,
		CodeNotDocker:         vm.ErrNotDocker,
		CodeDockerUnavailable: docker.ErrUnavailable,
		CodeInvalid:           docker.ErrInvalid,
		CodeTimeout:           context.DeadlineExceeded,
	} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()

			// what crossed the wire is the domain's error again on the other
			// side, whoever reads it.
			var arrived Error
			payload, err := json.Marshal(&Error{Code: code, Message: "whatever the node said"})
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(payload, &arrived))

			assert.ErrorIs(t, &arrived, want)
			assert.NotErrorIs(t, &arrived, vm.ErrQuotaExceeded)
		})
	}

	t.Run("an internal error stands for none of them", func(t *testing.T) {
		t.Parallel()

		internal := &Error{Code: CodeInternal, Message: "the engine fell over"}

		assert.NotErrorIs(t, internal, domain.ErrNotExists)
		assert.NotErrorIs(t, internal, context.DeadlineExceeded)
	})

	t.Run("it reads as its code and its message", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "not_found: no such container: web", (&Error{Code: CodeNotFound, Message: "no such container: web"}).Error())
		assert.Equal(t, "timeout", (&Error{Code: CodeTimeout}).Error())
	})
}

func TestFailed(t *testing.T) {
	t.Parallel()

	reply := Failed(vm.ErrNotDocker)

	assert.False(t, reply.OK)
	require.NotNil(t, reply.Error)
	assert.Equal(t, CodeNotDocker, reply.Error.Code)
	assert.ErrorIs(t, reply.Err(), vm.ErrNotDocker)
}
