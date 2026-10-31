package workload

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestOwners(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(auth.ToContext(request.Context(), &user.User{UUID: "caller-uuid"}))

	assert.Empty(t, Anybody(request), "the workload's routes ask for anybody's")
	assert.Equal(t, "caller-uuid", Caller(request), "the my routes ask for the caller's own")
}

func TestFailed(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{
			name:   "something that is not there is not found",
			err:    domain.ErrNotExists,
			status: http.StatusNotFound,
			body:   `{"code": "not_found"}`,
		},
		{
			name:   "and a node says what was not there",
			err:    &noderequest.Error{Code: noderequest.CodeNotFound, Message: "No such container: c0ffee"},
			status: http.StatusNotFound,
			body:   `{"code": "not_found", "message": "No such container: c0ffee"}`,
		},
		{
			name:   "what took too long may still be under way",
			err:    fmt.Errorf("pulling: %w", context.DeadlineExceeded),
			status: http.StatusGatewayTimeout,
			body:   `{"code": "timeout"}`,
		},
		{
			name:   "a node's timeout says how long it waited",
			err:    &noderequest.Error{Code: noderequest.CodeTimeout, Message: "no answer in 10m0s"},
			status: http.StatusGatewayTimeout,
			body:   `{"code": "timeout", "message": "no answer in 10m0s"}`,
		},
		{
			name:   "anything else failed, and what it failed with is not the caller's to read",
			err:    &noderequest.Error{Code: noderequest.CodeInternal, Message: "open /var/lib/msb/secret: permission denied"},
			status: http.StatusInternalServerError,
			body:   `{"code": "internal"}`,
		},
		{
			name:   "a workload that could not be asked failed too",
			err:    errors.New("dial tcp: connection refused"),
			status: http.StatusInternalServerError,
			body:   `{"code": "internal"}`,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()

			assert.True(t, Failed(response, httptest.NewRequest(http.MethodGet, "/", nil), tt.err))
			assert.Equal(t, tt.status, response.Code)
			assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.body, response.Body.String())
		})
	}

	t.Run("no error is no failure, and nothing is written", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		assert.False(t, Failed(response, httptest.NewRequest(http.MethodGet, "/", nil), nil))
		assert.Empty(t, response.Body.String())
		assert.False(t, response.Flushed)
	})
}

func TestRefused(t *testing.T) {
	t.Parallel()

	t.Run("what was refused, by field", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		assert.True(t, Refused(response, domain.ValidationErrors{"ports.1": "a port is a number from 1 to 65535"}))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, `{"errors": {"ports.1": "a port is a number from 1 to 65535"}}`, response.Body.String())
	})

	t.Run("nothing refused writes nothing", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		assert.False(t, Refused(response, nil))
		assert.False(t, Refused(response, domain.ValidationErrors{}))
		assert.Empty(t, response.Body.String())
	})
}

func TestDecode(t *testing.T) {
	t.Parallel()

	t.Run("json is read into the request", func(t *testing.T) {
		t.Parallel()

		var into struct {
			Name string `json:"name"`
		}

		response := httptest.NewRecorder()

		assert.True(t, Decode(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name": "web"}`)), &into))
		assert.Equal(t, "web", into.Name)
		assert.Empty(t, response.Body.String())
	})

	t.Run("a body that is not json is not a request", func(t *testing.T) {
		t.Parallel()

		var into struct{}

		response := httptest.NewRecorder()

		assert.False(t, Decode(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`)), &into))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.JSONEq(t, `{"errors": {"body": "invalid_value"}}`, response.Body.String())
	})
}

func TestTheQuery(t *testing.T) {
	t.Parallel()

	query := func(raw string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/?"+raw, nil)
	}

	t.Run("a page, the first unless another is asked for", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, uint(3), Page(query("page=3")))
		assert.Equal(t, uint(1), Page(query("")))
		assert.Equal(t, uint(1), Page(query("page=0")))
		assert.Equal(t, uint(1), Page(query("page=two")))
	})

	t.Run("a flag is no unless it says yes", func(t *testing.T) {
		t.Parallel()

		assert.True(t, Flag(query("force=true"), "force"))
		assert.True(t, Flag(query("force=1"), "force"))
		assert.False(t, Flag(query("force=false"), "force"))
		assert.False(t, Flag(query("force=please"), "force"))
		assert.False(t, Flag(query(""), "force"))
	})

	t.Run("since is an RFC 3339 moment, to the nanosecond", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, time.Date(2026, 10, 4, 12, 0, 1, 500, time.UTC), Since(query("since=2026-10-04T12:00:01.0000005Z")))
		assert.Equal(t, time.Date(2026, 10, 4, 12, 0, 1, 0, time.UTC), Since(query("since=2026-10-04T12:00:01Z")))
		assert.True(t, Since(query("since=yesterday")).IsZero(), "from the start")
		assert.True(t, Since(query("")).IsZero())
	})

	t.Run("a tail is how many of the last lines, and all of them when it says none", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, uint(500), Tail(query("tail=500")))
		assert.Zero(t, Tail(query("")))
		assert.Zero(t, Tail(query("tail=-1")))
	})
}
