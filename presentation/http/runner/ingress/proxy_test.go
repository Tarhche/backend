package ingress

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getRunner "github.com/khanzadimahdi/testproject/application/runner/ingress/getRunner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

// serve builds the ingress the way the provider does, so the tests exercise the
// route the proxy is actually reached through.
//
// The transport stands for the tunnel: in production the address it is given
// names a runner and it answers with one of that runner's own connections, so
// here it answers with a connection to the server standing in for one.
func serve(t *testing.T, connected map[string]string) *httptest.Server {
	t.Helper()

	registry := connectedRunners(connected)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			id, _, _ := net.SplitHostPort(address)

			upstream, ok := connected[id]
			if !ok {
				return nil, domain.ErrNotExists
			}

			return (&net.Dialer{}).DialContext(ctx, "tcp", upstream)
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/runners/{id}/{path...}", NewProxyHandler(getRunner.NewUseCase(registry), transport, slog.New(slog.DiscardHandler)))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestProxyHandler(t *testing.T) {
	t.Run("the request reaches the runner the id names", func(t *testing.T) {
		var gotPath, gotQuery, gotHeader string

		runner := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			gotHeader = r.Header.Get("X-Test")

			rw.WriteHeader(http.StatusTeapot)
			io.WriteString(rw, "answered by the runner")
		}))
		defer runner.Close()

		ingressServer := serve(t, map[string]string{"runner-worker-01": strings.TrimPrefix(runner.URL, "http://")})

		request, err := http.NewRequest(http.MethodGet, ingressServer.URL+"/runners/runner-worker-01/api/tasks/abc/logs?after=1", nil)
		assert.NoError(t, err)
		request.Header.Set("X-Test", "carried through")

		response, err := http.DefaultClient.Do(request)
		assert.NoError(t, err)
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		assert.NoError(t, err)

		assert.Equal(t, http.StatusTeapot, response.StatusCode)
		assert.Equal(t, "answered by the runner", string(body))
		assert.Equal(t, "/api/tasks/abc/logs", gotPath)
		assert.Equal(t, "after=1", gotQuery)
		assert.Equal(t, "carried through", gotHeader)
	})

	t.Run("a runner that is not connected is not found", func(t *testing.T) {
		ingressServer := serve(t, nil)

		response, err := http.Get(ingressServer.URL + "/runners/runner-worker-01/api/tasks")
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("a runner that is connected but not answering is a bad gateway", func(t *testing.T) {
		unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		address := strings.TrimPrefix(unreachable.URL, "http://")
		unreachable.Close()

		ingressServer := serve(t, map[string]string{"runner-worker-01": address})

		response, err := http.Get(ingressServer.URL + "/runners/runner-worker-01/api/tasks")
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadGateway, response.StatusCode)
	})

	t.Run("what the runner streams arrives as it is written", func(t *testing.T) {
		lines := make(chan string)

		runner := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// the answer starts before there is anything to say, which is what
			// makes the reader below wait on the stream rather than on a
			// response that has not begun.
			rw.WriteHeader(http.StatusOK)
			rw.(http.Flusher).Flush()

			for line := range lines {
				io.WriteString(rw, line)
				rw.(http.Flusher).Flush()
			}
		}))
		defer runner.Close()

		ingressServer := serve(t, map[string]string{"runner-worker-01": strings.TrimPrefix(runner.URL, "http://")})

		response, err := http.Get(ingressServer.URL + "/runners/runner-worker-01/api/tasks/abc/logs")
		assert.NoError(t, err)
		defer response.Body.Close()

		lines <- "first\n"

		read := make(chan string, 1)
		go func() {
			buffer := make([]byte, len("first\n"))
			if _, err := io.ReadFull(response.Body, buffer); err == nil {
				read <- string(buffer)
			}
			close(lines)
		}()

		select {
		case line := <-read:
			assert.Equal(t, "first\n", line)
		case <-time.After(2 * time.Second):
			t.Fatal("the first line did not arrive before the stream ended")
		}
	})

	t.Run("the ingress answers for itself when no runner is named", func(t *testing.T) {
		ingressServer := serve(t, nil)

		response, err := http.Get(ingressServer.URL + "/runners/")
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

func TestProxyHandler_upstream(t *testing.T) {
	t.Run("the runner is addressed by its id rather than by a machine", func(t *testing.T) {
		var gotHost string

		runner := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			gotHost = r.Host
		}))
		defer runner.Close()

		ingressServer := serve(t, map[string]string{"runner-worker-01": strings.TrimPrefix(runner.URL, "http://")})

		target, err := url.Parse(ingressServer.URL + "/runners/runner-worker-01/health")
		assert.NoError(t, err)

		response, err := http.DefaultClient.Do(&http.Request{Method: http.MethodGet, URL: target, Host: "ingress.example"})
		assert.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, "runner-worker-01", gotHost)
	})
}

// connectedRunners is the registry the tunnel is in production: a runner is in
// it for as long as it has a connection open.
type connectedRunners map[string]string

var _ ingress.Registry = connectedRunners{}

func (r connectedRunners) Get(_ context.Context, id string) (ingress.Runner, error) {
	if _, ok := r[id]; !ok {
		return ingress.Runner{}, domain.ErrNotExists
	}

	return ingress.Runner{ID: id, Connections: 1}, nil
}

func (r connectedRunners) All(_ context.Context) ([]ingress.Runner, error) {
	runners := make([]ingress.Runner, 0, len(r))
	for id := range r {
		runners = append(runners, ingress.Runner{ID: id, Connections: 1})
	}

	return runners, nil
}
