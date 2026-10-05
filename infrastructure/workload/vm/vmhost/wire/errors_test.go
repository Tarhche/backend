package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestErrorOf(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		err  error

		wantCode   Code
		wantStatus int
	}{
		{name: "a vm that is not there", err: fmt.Errorf("%w: no instance %q", domain.ErrNotExists, "vm-1"), wantCode: CodeNotFound, wantStatus: http.StatusNotFound},
		{name: "a vm that is there already", err: domain.ErrAlreadyExists, wantCode: CodeAlreadyExists, wantStatus: http.StatusConflict},
		{name: "a vm that is not running", err: vm.ErrNotRunning, wantCode: CodeNotRunning, wantStatus: http.StatusConflict},
		{name: "a vm that is not a docker vm", err: vm.ErrNotDocker, wantCode: CodeNotDocker, wantStatus: http.StatusConflict},
		{name: "a node with no room", err: fmt.Errorf("%w: 8 GiB asked", vm.ErrNoCapacity), wantCode: CodeNoCapacity, wantStatus: http.StatusInsufficientStorage},
		{name: "an archive from another engine", err: vm.ErrEngineMismatch, wantCode: CodeEngineMismatch, wantStatus: http.StatusUnprocessableEntity},
		{name: "more than a quota", err: vm.ErrQuotaExceeded, wantCode: CodeQuotaExceeded, wantStatus: http.StatusForbidden},
		{name: "a request that cannot be read", err: fmt.Errorf("%w: not JSON", ErrInvalid), wantCode: CodeInvalid, wantStatus: http.StatusBadRequest},
		{name: "out of time", err: context.DeadlineExceeded, wantCode: CodeTimeout, wantStatus: http.StatusGatewayTimeout},
		{name: "given up on", err: context.Canceled, wantCode: CodeCanceled, wantStatus: 499},
		{name: "shutting down", err: ErrUnavailable, wantCode: CodeUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "anything else", err: errors.New("the engine fell over"), wantCode: CodeInternal, wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			answered := ErrorOf(tc.err)

			require.NotNil(t, answered)
			assert.Equal(t, tc.wantCode, answered.Code)
			assert.Equal(t, tc.wantStatus, answered.Status())
			assert.Equal(t, tc.err.Error(), answered.Message, "the message is the error's own words")
		})
	}

	t.Run("no error is no answer", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, ErrorOf(nil))
	})

	t.Run("an error that crossed the socket already is passed on as it is", func(t *testing.T) {
		t.Parallel()

		arrived := &Error{Code: CodeNoCapacity, Message: "no room"}

		assert.Same(t, arrived, ErrorOf(fmt.Errorf("creating vm-1: %w", arrived)))
	})
}

// TestError_Is holds every code to standing for its error again once it has
// crossed the socket, so the client's callers match it as they would the
// engine's own.
func TestError_Is(t *testing.T) {
	t.Parallel()

	for _, meaning := range meanings {
		t.Run(string(meaning.code), func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(ErrorOf(fmt.Errorf("vm-1: %w", meaning.err)))
			require.NoError(t, err)

			var arrived Error
			require.NoError(t, json.Unmarshal(encoded, &arrived))

			assert.ErrorIs(t, &arrived, meaning.err)
			assert.Contains(t, arrived.Error(), "vm-1", "what the error said survives")

			for _, other := range meanings {
				if other.code != meaning.code {
					assert.NotErrorIs(t, &arrived, other.err)
				}
			}
		})
	}

	t.Run("an internal error is none of them", func(t *testing.T) {
		t.Parallel()

		internal := &Error{Code: CodeInternal, Message: "boom"}

		for _, meaning := range meanings {
			assert.NotErrorIs(t, internal, meaning.err)
		}

		assert.Equal(t, http.StatusInternalServerError, internal.Status())
		assert.Equal(t, "internal: boom", internal.Error())
	})
}
