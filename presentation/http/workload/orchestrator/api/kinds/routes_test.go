package kinds

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	attachresource "github.com/khanzadimahdi/testproject/application/workload/orchestrator/attachResource"
	getresourceendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kind/kindtest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	infraJWT "github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// keys are what the blog signs tokens with, and what a node verifies them
// with: the public half alone.
func keys(t *testing.T) (*infraJWT.JWT, *infraJWT.JWT) {
	t.Helper()

	private, err := ecdsa.Generate()
	require.NoError(t, err)

	return infraJWT.NewJWT(private, private.Public()), infraJWT.NewJWT(nil, private.Public())
}

// token is an access token for subject, as the blog signs one.
func token(t *testing.T, signer *infraJWT.JWT, subject string) string {
	t.Helper()

	signed, err := signer.Generate(t.Context(), jwt.RegisteredClaims{
		Subject:   subject,
		Audience:  jwt.ClaimStrings{auth.AccessToken},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	require.NoError(t, err)

	return signed
}

// aNode is a node's own http server with the kinds it runs routed as its
// serve command routes them: the api behind "/", and the ports beside it.
func aNode(t *testing.T, kinds *kind.Registry[kind.NodeBinding], verifier *infraJWT.JWT) *httptest.Server {
	t.Helper()

	api := http.NewServeMux()
	mux := http.NewServeMux()
	mux.Handle("/", api)

	routes := NewRoutes(kinds, attachresource.NewUseCase(kinds, validates{}), getresourceendpoint.NewUseCase(kinds), verifier, slog.New(slog.DiscardHandler))
	require.NoError(t, routes.Register(api, mux))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// lighting is what a node holds: lamp-1 lit, lamp-2 not, both the owner's.
func lighting(ports map[port.Port]string) *lampNode {
	return &lampNode{held: []held{
		{uuid: "lamp-1", slug: "desk-abcde", owner: "owner-uuid", lit: true, ports: ports},
		{uuid: "lamp-2", slug: "porch-fghij", owner: "owner-uuid", lit: false, ports: ports},
	}}
}

func websocketURL(server *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + path
}

func TestRoutes_Attach(t *testing.T) {
	t.Parallel()

	signer, verifier := keys(t)

	t.Run("the owner's terminal carries a stream both ways, and resizes it", func(t *testing.T) {
		t.Parallel()

		node := lighting(nil)
		server := aNode(t, running(t, lamps{node}), verifier)

		conn, response, err := websocket.DefaultDialer.Dial(websocketURL(server, "/api/lamps/lamp-1/attach"), http.Header{
			"Authorization": []string{"Bearer " + token(t, signer, "owner-uuid")},
		})
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

		opened := node.opened()
		require.Len(t, opened, 1)
		assert.Eventually(t, func() bool { return len(opened[0].sizes()) == 1 }, 5*time.Second, 5*time.Millisecond)
		assert.Equal(t, []string{"50x132"}, opened[0].sizes())

		// the client leaving ends the stream.
		require.NoError(t, conn.Close())
		assert.Eventually(t, opened[0].isClosed, 5*time.Second, 5*time.Millisecond)
	})

	t.Run("a browser's token, offered as a subprotocol, opens it as well", func(t *testing.T) {
		t.Parallel()

		server := aNode(t, running(t, lamps{lighting(nil)}), verifier)

		dialer := websocket.Dialer{Subprotocols: []string{"bearer", token(t, signer, "owner-uuid")}}

		conn, response, err := dialer.Dial(websocketURL(server, "/api/lamps/lamp-1/attach"), nil)
		require.NoError(t, err)
		defer conn.Close()
		defer response.Body.Close()

		assert.Equal(t, "bearer", conn.Subprotocol(), "the marker is echoed back, never the token")
	})

	for name, tt := range map[string]struct {
		path          string
		authorization func() string
		status        int
	}{
		"somebody else's lamp is not there for them": {
			path:          "/api/lamps/lamp-1/attach",
			authorization: func() string { return "Bearer " + token(t, signer, "somebody-else") },
			status:        http.StatusNotFound,
		},
		"nobody is refused before anything is looked for": {
			path:          "/api/lamps/lamp-1/attach",
			authorization: func() string { return "" },
			status:        http.StatusUnauthorized,
		},
		"and so is a token the estate did not sign": {
			path: "/api/lamps/lamp-1/attach",
			authorization: func() string {
				other, _ := keys(t)

				return "Bearer " + token(t, other, "owner-uuid")
			},
			status: http.StatusUnauthorized,
		},
		"a lamp that is not lit has no terminal to open now": {
			path:          "/api/lamps/lamp-2/attach",
			authorization: func() string { return "Bearer " + token(t, signer, "owner-uuid") },
			status:        http.StatusServiceUnavailable,
		},
		"a lamp this node does not hold is not there": {
			path:          "/api/lamps/lamp-9/attach",
			authorization: func() string { return "Bearer " + token(t, signer, "owner-uuid") },
			status:        http.StatusNotFound,
		},
		"an action that is not a stream is not routed": {
			path:          "/api/lamps/lamp-1/state",
			authorization: func() string { return "Bearer " + token(t, signer, "owner-uuid") },
			status:        http.StatusNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			node := lighting(nil)
			server := aNode(t, running(t, lamps{node}), verifier)

			header := http.Header{}
			if authorization := tt.authorization(); len(authorization) > 0 {
				header.Set("Authorization", authorization)
			}

			_, response, err := websocket.DefaultDialer.Dial(websocketURL(server, tt.path), header)
			require.Error(t, err)
			require.NotNil(t, response)
			defer response.Body.Close()

			assert.Equal(t, tt.status, response.StatusCode)
			assert.Empty(t, node.opened(), "nothing was opened")
		})
	}
}

func TestRoutes_PublicAttach(t *testing.T) {
	t.Parallel()

	signer, verifier := keys(t)

	// a lamp whose terminal anybody may ask for, holding one lamp of
	// everybody's and one of somebody's: which of them is opened for nobody
	// is the lamp's to say.
	public := func(t *testing.T) (*lampNode, *httptest.Server) {
		t.Helper()

		d := lamp()
		for i := range d.Actions {
			if d.Actions[i].Name == "attach" {
				d.Actions[i].Public = true
			}
		}

		node := &lampNode{held: []held{
			{uuid: "lamp-1", slug: "desk-abcde", owner: "owner-uuid", lit: true},
			{uuid: "lamp-3", slug: "street-klmno", owner: "", lit: true},
		}}

		kinds := kind.NewRegistry[kind.NodeBinding]()
		require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](d, lamps{node})))

		return node, aNode(t, kinds, verifier)
	}

	t.Run("one of everybody's is opened for nobody", func(t *testing.T) {
		t.Parallel()

		node, server := public(t)

		conn, response, err := websocket.DefaultDialer.Dial(websocketURL(server, "/api/lamps/lamp-3/attach"), nil)
		require.NoError(t, err)
		defer conn.Close()
		defer response.Body.Close()

		require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("hi\n")))
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

		_, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, "HI\n", string(payload))
		assert.Len(t, node.opened(), 1)
	})

	for name, tt := range map[string]struct {
		path          string
		authorization func() string
		status        int
	}{
		"one of somebody's is not there for nobody": {
			path:          "/api/lamps/lamp-1/attach",
			authorization: func() string { return "" },
			status:        http.StatusNotFound,
		},
		"and a token that is there is still verified": {
			path: "/api/lamps/lamp-3/attach",
			authorization: func() string {
				other, _ := keys(t)

				return "Bearer " + token(t, other, "owner-uuid")
			},
			status: http.StatusUnauthorized,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			node, server := public(t)

			header := http.Header{}
			if authorization := tt.authorization(); len(authorization) > 0 {
				header.Set("Authorization", authorization)
			}

			_, response, err := websocket.DefaultDialer.Dial(websocketURL(server, tt.path), header)
			require.Error(t, err)
			require.NotNil(t, response)
			defer response.Body.Close()

			assert.Equal(t, tt.status, response.StatusCode)
			assert.Empty(t, node.opened(), "nothing was opened")
		})
	}

	t.Run("and its owner's is opened for its owner, as any terminal is", func(t *testing.T) {
		t.Parallel()

		_, server := public(t)

		conn, response, err := websocket.DefaultDialer.Dial(websocketURL(server, "/api/lamps/lamp-1/attach"), http.Header{
			"Authorization": []string{"Bearer " + token(t, signer, "owner-uuid")},
		})
		require.NoError(t, err)
		defer conn.Close()
		defer response.Body.Close()
	})
}

func TestRoutes_Ports(t *testing.T) {
	t.Parallel()

	// what a lamp serves, where its port is published.
	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(rw, r.Host+" "+r.URL.Path+"?"+r.URL.RawQuery)
	}))
	t.Cleanup(upstream.Close)

	published := upstream.Listener.Addr().String()

	_, verifier := keys(t)
	server := aNode(t, running(t, lamps{lighting(map[port.Port]string{80: published, 8080: "127.0.0.1:1"})}), verifier)

	for name, tt := range map[string]struct {
		path   string
		status int
		body   string
	}{
		"a lamp's lowest port is reached by its slug": {
			path:   "/lamps/desk-abcde/0/index.html?a=1",
			status: http.StatusOK,
			body:   "desk-abcde.workload.localhost /index.html?a=1",
		},
		"and a named one by its number": {
			path:   "/lamps/desk-abcde/80/",
			status: http.StatusOK,
			body:   "desk-abcde.workload.localhost /?",
		},
		"a port it does not expose is not there": {
			path:   "/lamps/desk-abcde/9999/",
			status: http.StatusNotFound,
		},
		"a slug this node does not hold is not there": {
			path:   "/lamps/other-pqrst/0/",
			status: http.StatusNotFound,
		},
		"a lamp that is not lit cannot be reached now": {
			path:   "/lamps/porch-fghij/0/",
			status: http.StatusServiceUnavailable,
		},
		"a port that is no port is refused": {
			path:   "/lamps/desk-abcde/http/",
			status: http.StatusBadRequest,
		},
		"one that is there and does not answer says so": {
			path:   "/lamps/desk-abcde/8080/",
			status: http.StatusBadGateway,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequest(http.MethodGet, server.URL+tt.path, nil)
			require.NoError(t, err)
			request.Host = "desk-abcde.workload.localhost"

			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()

			assert.Equal(t, tt.status, response.StatusCode)

			if len(tt.body) > 0 {
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.Equal(t, tt.body, string(body))
			}
		})
	}
}

func TestRoutes_Register(t *testing.T) {
	t.Parallel()

	signer, verifier := keys(t)

	// asks is what a node answers a request with, by status.
	asks := func(t *testing.T, server *httptest.Server, path string) int {
		t.Helper()

		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+token(t, signer, "owner-uuid"))

		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		return response.StatusCode
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(rw, "lit")
	}))
	t.Cleanup(upstream.Close)

	ports := map[port.Port]string{80: upstream.Listener.Addr().String()}

	for name, tt := range map[string]struct {
		kinds  func(t *testing.T) *kind.Registry[kind.NodeBinding]
		attach int
		ports  int
	}{
		"a node that runs no kinds routes nothing of them": {
			kinds:  func(*testing.T) *kind.Registry[kind.NodeBinding] { return kind.NewRegistry[kind.NodeBinding]() },
			attach: http.StatusNotFound,
			ports:  http.StatusNotFound,
		},
		"a kind's streams and ports are routed when its strategy serves them": {
			kinds: func(t *testing.T) *kind.Registry[kind.NodeBinding] { return running(t, lamps{lighting(ports)}) },
			// a plain request is no websocket, which is all that is refused.
			attach: http.StatusBadRequest,
			ports:  http.StatusOK,
		},
		"and only its streams when it serves no ports": {
			kinds: func(t *testing.T) *kind.Registry[kind.NodeBinding] {
				return running(t, attachingLamps{lighting(ports)})
			},
			attach: http.StatusBadRequest,
			ports:  http.StatusNotFound,
		},
		"and only its ports when it serves no streams": {
			kinds:  func(t *testing.T) *kind.Registry[kind.NodeBinding] { return running(t, exposingLamps{lighting(ports)}) },
			attach: http.StatusNotFound,
			ports:  http.StatusOK,
		},
		"and no ports of a kind that declares no endpoints, whatever its strategy serves": {
			kinds: func(t *testing.T) *kind.Registry[kind.NodeBinding] {
				d := lamp()
				d.Endpoints = false

				kinds := kind.NewRegistry[kind.NodeBinding]()
				require.NoError(t, kinds.Register(kind.BindNode[lampSpec, lampStatus](d, lamps{lighting(ports)})))

				return kinds
			},
			attach: http.StatusBadRequest,
			ports:  http.StatusNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			server := aNode(t, tt.kinds(t), verifier)

			assert.Equal(t, tt.attach, asks(t, server, "/api/lamps/lamp-1/attach"), "attach")
			assert.Equal(t, tt.ports, asks(t, server, "/lamps/desk-abcde/0/"), "ports")
		})
	}

	t.Run("the VMs' and the tasks' own routes are left as they are", func(t *testing.T) {
		t.Parallel()

		api := http.NewServeMux()
		mux := http.NewServeMux()
		mux.Handle("/", api)

		own := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusTeapot) })

		api.Handle("GET /api/vms/{uuid}/attach", own)
		api.Handle("GET /api/tasks/{uuid}/attach", own)
		mux.Handle("/vms/{slug}/{port}/{path...}", own)
		mux.Handle("/tasks/{slug}/{port}/{path...}", own)

		kinds := running(t, lamps{lighting(ports)})
		routes := NewRoutes(kinds, attachresource.NewUseCase(kinds, validates{}), getresourceendpoint.NewUseCase(kinds), verifier, slog.New(slog.DiscardHandler))
		require.NoError(t, routes.Register(api, mux))

		server := httptest.NewServer(mux)
		defer server.Close()

		for _, path := range []string{"/api/vms/vm-1/attach", "/api/tasks/task-1/attach", "/vms/box-abcde/0/", "/tasks/box-abcde/0/"} {
			assert.Equal(t, http.StatusTeapot, asks(t, server, path), path)
		}

		assert.Equal(t, http.StatusOK, asks(t, server, "/lamps/desk-abcde/0/"))
	})

	t.Run("a kind whose route is taken already is an error, not a panic", func(t *testing.T) {
		t.Parallel()

		for name, taken := range map[string]func(api *http.ServeMux, mux *http.ServeMux){
			"its stream": func(api *http.ServeMux, _ *http.ServeMux) {
				api.Handle("GET /api/lamps/{uuid}/attach", http.NotFoundHandler())
			},
			"its ports": func(_ *http.ServeMux, mux *http.ServeMux) {
				mux.Handle("/lamps/{slug}/{port}/{path...}", http.NotFoundHandler())
			},
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				api := http.NewServeMux()
				mux := http.NewServeMux()
				taken(api, mux)

				kinds := running(t, lamps{lighting(ports)})
				routes := NewRoutes(kinds, attachresource.NewUseCase(kinds, validates{}), getresourceendpoint.NewUseCase(kinds), verifier, slog.New(slog.DiscardHandler))

				assert.ErrorContains(t, routes.Register(api, mux), "cannot be routed")
			})
		}
	})
}

// TestConformance holds the lamp these tests route, registered in every
// service through the bindings every kind is, to the rules every kind keeps.
func TestConformance(t *testing.T) {
	t.Parallel()

	controlPlane := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, controlPlane.Register(kind.BindControlPlane[lampSpec, lampStatus](lamp(), lampControlPlane{})))

	ingress := kind.NewRegistry[kind.IngressBinding]()
	require.NoError(t, ingress.Register(kind.BindIngress(lamp(), lampIngress{})))

	kindtest.Conformance(t, kind.Services{
		ControlPlane: controlPlane,
		Node:         running(t, lamps{lighting(nil)}),
		Ingress:      ingress,
	}, lampPermissions())
}
