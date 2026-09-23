package launcher

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// serve stands up a launcher answering with handler on a unix socket, and
// returns a client for it.
func serve(t *testing.T, handler http.Handler) *Client {
	t.Helper()

	// short on purpose: a socket's path is at most 108 bytes.
	dir, err := os.MkdirTemp("", "launcher")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socket := filepath.Join(dir, "launcher.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	return NewClient(socket)
}

func TestClient(t *testing.T) {
	t.Run("a machine is asked for as it is, and said back as the launcher made it", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("POST /machines", func(rw http.ResponseWriter, r *http.Request) {
			var spec machine.Spec
			require.NoError(t, json.NewDecoder(r.Body).Decode(&spec))
			assert.Equal(t, "0123456789abcdef", spec.ID)

			rw.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(rw).Encode(map[string]any{"machine": machine.Machine{ID: spec.ID, Running: true, Socket: "/s"}})
		})

		launched, err := serve(t, mux).Launch(context.Background(), machine.Spec{ID: "0123456789abcdef"})

		require.NoError(t, err)
		assert.Equal(t, machine.Machine{ID: "0123456789abcdef", Running: true, Socket: "/s"}, launched)
	})

	t.Run("an orchestrator asks for its own machines by name", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /machines", func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "runner-orchestrator-01", r.URL.Query().Get("owner"))

			_ = json.NewEncoder(rw).Encode(map[string]any{"machines": []machine.Machine{{ID: "0000000000000001"}}})
		})

		held, err := serve(t, mux).Machines(context.Background(), "runner-orchestrator-01")

		require.NoError(t, err)
		assert.Equal(t, []machine.Machine{{ID: "0000000000000001"}}, held)
	})

	t.Run("a network is made with whether it routes out, and said back", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("PUT /networks/{owner}/{name}", func(rw http.ResponseWriter, r *http.Request) {
			var body struct {
				Masquerade bool `json:"masquerade"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

			_ = json.NewEncoder(rw).Encode(map[string]any{"network": machine.Network{
				Owner:      r.PathValue("owner"),
				Name:       r.PathValue("name"),
				Subnet:     "10.200.1.0/24",
				Masquerade: body.Masquerade,
			}})
		})

		made, err := serve(t, mux).EnsureNetwork(context.Background(), "runner-orchestrator-01", "runner-public", true)

		require.NoError(t, err)
		assert.Equal(t, machine.Network{Owner: "runner-orchestrator-01", Name: "runner-public", Subnet: "10.200.1.0/24", Masquerade: true}, made)
	})

	t.Run("a refusal is turned into what it means", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /networks/{owner}/{name}", func(rw http.ResponseWriter, r *http.Request) {
			http.Error(rw, "the network still has machines plugged into it", http.StatusConflict)
		})
		mux.HandleFunc("DELETE /machines/{id}", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(rw).Encode(map[string]any{"errors": map[string]string{"id": "invalid_value"}})
		})
		mux.HandleFunc("POST /machines", func(rw http.ResponseWriter, r *http.Request) {
			http.Error(rw, "no /dev/kvm", http.StatusInternalServerError)
		})

		client := serve(t, mux)

		assert.ErrorIs(t, client.RemoveNetwork(context.Background(), "runner-orchestrator-01", "runner-stack-shop"), machine.ErrNetworkInUse)
		assert.ErrorContains(t, client.Terminate(context.Background(), "../etc"), "invalid_value")

		_, err := client.Launch(context.Background(), machine.Spec{})
		assert.ErrorContains(t, err, "no /dev/kvm")
	})

	t.Run("a launcher that is not there says so", func(t *testing.T) {
		_, err := NewClient(filepath.Join(t.TempDir(), "nothing.sock")).Machines(context.Background(), "runner-orchestrator-01")

		assert.ErrorContains(t, err, "the launcher could not be reached")
	})
}
