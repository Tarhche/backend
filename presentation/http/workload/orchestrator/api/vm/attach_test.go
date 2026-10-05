package vm

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attachvm "github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/attachVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// asSubject is the token middleware having verified a token for subject, or
// none at all.
func asSubject(subject string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if len(subject) > 0 {
			r = r.WithContext(middleware.WithSubject(r.Context(), subject))
		}

		next.ServeHTTP(rw, r)
	})
}

// echoShell is a shell that says back what it is typed, upper-cased.
func echoShell(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, stdout io.Writer, _ io.Writer) int {
	buffer := make([]byte, 64)

	for {
		n, err := stdin.Read(buffer)
		if n > 0 {
			if _, err := stdout.Write([]byte(strings.ToUpper(string(buffer[:n])))); err != nil {
				return 1
			}
		}

		if err != nil {
			return 0
		}
	}
}

func serving(t *testing.T, subject string, stopped bool) (*httptest.Server, *memory.Engine) {
	t.Helper()

	e := memory.New(memory.WithExec(echoShell))

	_, err := e.Create(t.Context(), vm.Spec{
		ID:     "vm-1",
		Kind:   vm.KindMachine,
		Image:  "ubuntu:24.04",
		Labels: map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelOwner: "owner-uuid"},
	})
	require.NoError(t, err)

	if stopped {
		require.NoError(t, e.Stop(t.Context(), "vm-1"))
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/vms/{uuid}/attach", asSubject(subject, NewAttachHandler(attachvm.NewUseCase(e, validates{}), slog.New(slog.DiscardHandler))))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, e
}

func TestAttachHandler(t *testing.T) {
	t.Parallel()

	t.Run("the owner's terminal carries a shell both ways, and resizes it", func(t *testing.T) {
		t.Parallel()

		server, e := serving(t, "owner-uuid", false)

		conn, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/vms/vm-1/attach", nil)
		require.NoError(t, err)
		defer conn.Close()
		defer response.Body.Close()

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","rows":50,"cols":132}`)))
		require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("echo hi\n")))

		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

		messageType, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, websocket.BinaryMessage, messageType)
		assert.Equal(t, "ECHO HI\n", string(payload))

		assert.Equal(t, 1, e.Sessions("vm-1"))

		// the client leaving ends the shell.
		require.NoError(t, conn.Close())
		assert.Eventually(t, func() bool { return e.Sessions("vm-1") == 0 }, 5*time.Second, 5*time.Millisecond)
	})

	testcases := []struct {
		name       string
		subject    string
		stopped    bool
		wantStatus int
	}{
		{name: "somebody else's VM is not there for them", subject: "somebody-else", wantStatus: http.StatusNotFound},
		{name: "nobody is not its owner", subject: "", wantStatus: http.StatusNotFound},
		{name: "a stopped VM has no terminal to open", subject: "owner-uuid", stopped: true, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, e := serving(t, tt.subject, tt.stopped)

			_, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/vms/vm-1/attach", nil)
			require.Error(t, err)
			defer response.Body.Close()

			assert.Equal(t, tt.wantStatus, response.StatusCode)
			assert.Zero(t, e.Sessions("vm-1"), "nothing was opened")
		})
	}
}
