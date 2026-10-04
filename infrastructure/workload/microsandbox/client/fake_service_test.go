package client_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/client"
)

// The service the client is tested against is workload-microsandbox as the
// api package describes it, kept in memory and served by httptest over mutual
// TLS under an authority made for the test. It is written from the contract
// alone, as the real service is, so what is tested is the client speaking the
// API rather than the client agreeing with itself.
//
// It runs nothing. A run's main process is a stand-in whose exit codes are the
// ones the contract promises: 143 for a stop, as a process the stop signal ended
// reports, and 137 for a kill. A published port is a real listener on loopback,
// answering with the port it stands in for, so a port can be reached the way
// the orchestrator reaches one.

const (
	// orchestrator is the node the client under test works for, and
	// neighbour another one using the same service.
	orchestrator = "workload-orchestrator-01"
	neighbour    = "workload-orchestrator-02"

	stoppedBySignal = 143
	killed          = 137
)

// authority is the workload's certificate authority, as the tunnel has it.
type authority struct {
	ca  *certificate.Authority
	pem string
}

func newAuthority(t *testing.T) *authority {
	t.Helper()

	ca, err := certificate.GenerateCA("workload test authority", 0)
	require.NoError(t, err)

	return &authority{ca: ca, pem: string(certificate.EncodeCertificate(ca.Certificate))}
}

// orchestratorCertificate is the certificate an orchestrator holds for the
// tunnel, and its key, as PEM.
func (a *authority) orchestratorCertificate(t *testing.T, name string) (string, string) {
	t.Helper()

	issued, key, err := a.ca.GenerateClientCertificate(certificate.Request{Name: name})
	require.NoError(t, err)

	return encoded(t, issued, key)
}

// serviceCertificate is the certificate the service answers with, issued as
// `app certificate ingress generate --name workload-microsandbox` issues it,
// for whatever other names and addresses it is given.
func (a *authority) serviceCertificate(t *testing.T, names ...string) (string, string) {
	t.Helper()

	request := certificate.Request{Name: "workload-microsandbox"}
	for _, name := range names {
		if address := net.ParseIP(name); address != nil {
			request.IPAddresses = append(request.IPAddresses, address)
		} else {
			request.DNSNames = append(request.DNSNames, name)
		}
	}

	issued, key, err := a.ca.GenerateServerCertificate(request)
	require.NoError(t, err)

	return encoded(t, issued, key)
}

// encoded is a certificate and its key as PEM, which is how a deployment
// carries them.
func encoded(t *testing.T, issued *x509.Certificate, key *ecdsa.PrivateKey) (string, string) {
	t.Helper()

	encodedKey, err := certificate.EncodePrivateKey(key)
	require.NoError(t, err)

	return string(certificate.EncodeCertificate(issued)), string(encodedKey)
}

// stored is one run as the service keeps it.
type stored struct {
	api.Run

	lines     []api.LogLine
	stats     api.Stats
	listeners []net.Listener
}

// received is one request as the service got it.
type received struct {
	route string
	query map[string][]string
	body  []byte
}

// failure is an answer the service is made to give on one route, whatever was
// asked.
type failure struct {
	status int
	code   string

	// message is the error's message. A failure with no code is answered
	// as something between the two would answer it, in plain text.
	message string

	// drop hangs up without answering at all, as a service that went away
	// mid-request does.
	drop bool
}

// session is one exec as the service keeps it.
type session struct {
	runID   string
	request api.ExecRequest
	rows    uint16
	cols    uint16
	ended   bool
}

type service struct {
	t         *testing.T
	authority *authority
	server    *httptest.Server

	lock     sync.Mutex
	info     api.Info
	runs     map[string]*stored
	made     int
	pulled   []string
	requests []received
	failures map[string]failure
	sessions map[string]*session

	// changed is closed, and replaced, whenever a run's log or state
	// changes, which is what wakes the logs being followed.
	changed chan struct{}

	// epoch and ticks are the service's clock; see now.
	epoch time.Time
	ticks int
}

// now is the service's clock. It is a second later each time it is read, so
// the order of whatever it stamps is never in doubt. The lock is held.
func (s *service) now() time.Time {
	s.ticks++

	return s.epoch.Add(time.Duration(s.ticks) * time.Second)
}

// newService starts the service, answering to whoever the authority signed for
// clientAuth and to nobody else, with a certificate for the names it is reached
// by in a test: localhost, and the loopback address httptest listens on.
func newService(t *testing.T) *service {
	t.Helper()

	return newServiceAnsweringFor(t, "localhost", "127.0.0.1")
}

// newServiceAnsweringFor starts the service with a certificate for these names
// and addresses besides its own.
func newServiceAnsweringFor(t *testing.T, names ...string) *service {
	t.Helper()

	s := &service{
		t:         t,
		authority: newAuthority(t),
		info: api.Info{
			APIVersion:          api.Version,
			ServiceVersion:      "test",
			MicrosandboxVersion: "0.7.6",
			Ready:               true,
			Architecture:        runtime.GOARCH,
		},
		runs:     make(map[string]*stored),
		failures: make(map[string]failure),
		sessions: make(map[string]*session),
		changed:  make(chan struct{}),
		epoch:    time.Date(2026, time.October, 4, 9, 30, 0, 0, time.UTC),
	}

	certificatePEM, keyPEM := s.authority.serviceCertificate(t, names...)

	tlsConfig, err := certificate.ServerTLSConfig(certificate.Credentials{
		Authority:   s.authority.pem,
		Certificate: certificatePEM,
		PrivateKey:  keyPEM,
	})
	require.NoError(t, err)

	// httptest answers with a certificate of its own when it is given none,
	// and a client that reaches the service by address sends no name for
	// the callback to answer to.
	own, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	tlsConfig.Certificates = []tls.Certificate{*own}

	mux := http.NewServeMux()
	s.handle(mux, api.RouteInfo, s.answerInfo)
	s.handle(mux, api.RouteListRuns, s.list)
	s.handle(mux, api.RouteCreateRun, s.create)
	s.handle(mux, api.RouteGetRun, s.get)
	s.handle(mux, api.RouteDeleteRun, s.delete)
	s.handle(mux, api.RouteStartRun, s.start)
	s.handle(mux, api.RouteStopRun, s.stop)
	s.handle(mux, api.RouteKillRun, s.kill)
	s.handle(mux, api.RouteRestartRun, s.restart)
	s.handle(mux, api.RouteRunLogs, s.logs)
	s.handle(mux, api.RouteRunStats, s.runStats)
	s.handle(mux, api.RouteExec, s.exec)
	s.handle(mux, api.RouteEndExec, s.endExec)
	s.handle(mux, api.RouteNodeStats, s.nodeStats)
	s.handle(mux, api.RoutePullImage, s.pull)

	s.server = httptest.NewUnstartedServer(mux)
	s.server.TLS = tlsConfig

	// handshakes the service refuses are what some tests are about, and
	// nothing to report.
	s.server.Config.ErrorLog = log.New(io.Discard, "", 0)

	s.server.StartTLS()

	t.Cleanup(s.close)

	return s
}

// close stops the service, and with it whatever its runs published.
func (s *service) close() {
	s.server.CloseClientConnections()
	s.server.Close()

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, run := range s.runs {
		s.unpublish(run)
	}
}

// client is a client for the node, built as the orchestrator builds it: with
// its tunnel credentials, and the service's URL.
func (s *service) client(t *testing.T, node string) *client.Client {
	t.Helper()

	certificatePEM, keyPEM := s.authority.orchestratorCertificate(t, node)

	c, err := client.New(client.Config{
		URL:         s.server.URL,
		Node:        node,
		Authority:   s.authority.pem,
		Certificate: certificatePEM,
		PrivateKey:  keyPEM,
	}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	t.Cleanup(c.Close)

	return c
}

// runtime is the task.Runtime of the node.
func (s *service) runtime(t *testing.T, node string) *client.Runtime {
	t.Helper()

	return client.NewRuntime(s.client(t, node))
}

// speak makes the service say it speaks another version of the API.
func (s *service) speak(version string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.info.APIVersion = version
}

// images is every image the service was asked to have.
func (s *service) images() []string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Clone(s.pulled)
}

// fail makes the service answer one route with a failure, until it is told
// otherwise.
func (s *service) fail(route string, f failure) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.failures[route] = f
}

func (s *service) recover(route string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.failures, route)
}

// asked is every request made of one route, in the order they came.
func (s *service) asked(route string) []received {
	s.lock.Lock()
	defer s.lock.Unlock()

	var asked []received
	for _, r := range s.requests {
		if r.route == route {
			asked = append(asked, r)
		}
	}

	return asked
}

// routesAsked is which routes were asked, in order, without info.
func (s *service) routesAsked() []string {
	s.lock.Lock()
	defer s.lock.Unlock()

	var routes []string
	for _, r := range s.requests {
		if r.route != api.RouteInfo {
			routes = append(routes, r.route)
		}
	}

	return routes
}

// run is a copy of one run as the service holds it.
func (s *service) run(id string) (api.Run, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	run, ok := s.runs[id]
	if !ok {
		return api.Run{}, false
	}

	return run.Run, true
}

// write appends lines to a run's log, as its main process writing them.
func (s *service) write(id string, lines ...api.LogLine) {
	s.lock.Lock()
	defer s.lock.Unlock()

	run := s.runs[id]
	run.lines = append(run.lines, lines...)

	s.wake()
}

// measure sets what a run is using.
func (s *service) measure(id string, stats api.Stats) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.runs[id].stats = stats
}

// exit ends a run's main process with a code of its own choosing.
func (s *service) exit(id string, code int) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.end(s.runs[id], code, "")
}

func (s *service) session(id string) (session, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	found, ok := s.sessions[id]
	if !ok {
		return session{}, false
	}

	return *found, true
}

// wake tells everything following a log that something changed. The lock is
// held.
func (s *service) wake() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// handle registers a route as the service registers it, written exactly as
// the contract writes it.
func (s *service) handle(mux *http.ServeMux, route string, handler http.HandlerFunc) {
	mux.HandleFunc(route, func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)

			return
		}

		r.Body = io.NopCloser(bytes.NewReader(body))

		s.lock.Lock()
		s.requests = append(s.requests, received{route: route, query: r.URL.Query(), body: body})
		f, failing := s.failures[route]
		s.lock.Unlock()

		if !failing {
			handler(rw, r)

			return
		}

		if f.drop {
			conn, _, err := http.NewResponseController(rw).Hijack()
			if err == nil {
				_ = conn.Close()
			}

			return
		}

		if len(f.code) == 0 {
			http.Error(rw, f.message, f.status)

			return
		}

		refuse(rw, f.status, f.code, f.message)
	})
}

// refuse answers with an ErrorResponse, as every failure is answered.
func refuse(rw http.ResponseWriter, status int, code string, message string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(api.ErrorResponse{Error: api.Error{Code: code, Message: message}})
}

func answer(rw http.ResponseWriter, status int, value any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(value)
}

func notFound(rw http.ResponseWriter) {
	refuse(rw, http.StatusNotFound, api.CodeNotFound, "no such run")
}

func (s *service) answerInfo(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	info := s.info
	s.lock.Unlock()

	answer(rw, http.StatusOK, info)
}

func (s *service) list(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	node := query.Get(api.QueryNode)
	if len(node) == 0 {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "a listing names its node")

		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// in no particular order, as nothing in the contract promises one.
	listed := api.RunList{Runs: []api.Run{}}
	for _, run := range s.runs {
		if run.Node != node {
			continue
		}

		if uuid := query.Get(api.QueryTask); len(uuid) > 0 && run.Task.UUID != uuid {
			continue
		}

		if slug := query.Get(api.QuerySlug); len(slug) > 0 && run.Task.Slug != slug {
			continue
		}

		listed.Runs = append(listed.Runs, run.Run)
	}

	answer(rw, http.StatusOK, listed)
}

func (s *service) create(rw http.ResponseWriter, r *http.Request) {
	var spec api.RunSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "the spec could not be read")

		return
	}

	if len(spec.Node) == 0 || len(spec.Name) == 0 || len(spec.Image) == 0 || spec.Memory == 0 {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "a run names its node, its name and its image, and has memory")

		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	for _, run := range s.runs {
		if run.Node == spec.Node && run.Name == spec.Name {
			refuse(rw, http.StatusConflict, api.CodeNameInUse, fmt.Sprintf("%s already has a run called %s", spec.Node, spec.Name))

			return
		}
	}

	s.made++

	run := &stored{Run: api.Run{
		ID:        fmt.Sprintf("%032x", s.made),
		RunSpec:   spec,
		State:     api.StateCreated,
		CreatedAt: s.now(),
	}}
	s.runs[run.ID] = run

	answer(rw, http.StatusCreated, run.Run)
}

// found is the run a route names, or answers that there is none.
func (s *service) found(rw http.ResponseWriter, r *http.Request) (*stored, bool) {
	run, ok := s.runs[r.PathValue(api.WildcardRun)]
	if !ok {
		notFound(rw)
	}

	return run, ok
}

func (s *service) get(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if run, ok := s.found(rw, r); ok {
		answer(rw, http.StatusOK, run.Run)
	}
}

func (s *service) delete(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	run, ok := s.found(rw, r)
	if !ok {
		return
	}

	s.end(run, killed, "")
	delete(s.runs, run.ID)

	rw.WriteHeader(http.StatusNoContent)
}

func (s *service) start(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	run, ok := s.found(rw, r)
	if !ok {
		return
	}

	if run.State != api.StateRunning {
		s.boot(run)
	}

	answer(rw, http.StatusOK, run.Run)
}

// boot starts a run, publishing each of its ports on a listener of its own.
// The lock is held.
func (s *service) boot(run *stored) {
	run.State = api.StateRunning
	run.StartedAt = s.now()
	run.FinishedAt = time.Time{}
	run.ExitCode = 0
	run.Error = ""
	run.Endpoints = nil

	for _, guest := range run.Ports {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(s.t, err)

		name := run.Name
		go func() {
			_ = http.Serve(listener, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(rw, "%s answers on its port %d", name, guest)
			}))
		}()

		run.listeners = append(run.listeners, listener)
		run.Endpoints = append(run.Endpoints, api.Endpoint{
			Port:     guest,
			HostPort: uint16(listener.Addr().(*net.TCPAddr).Port),
		})
	}

	s.wake()
}

// end ends a run's main process. The lock is held.
func (s *service) end(run *stored, code int, reason string) {
	if run.State == api.StateExited || run.State == api.StateCreated {
		return
	}

	run.State = api.StateExited
	run.ExitCode = code
	run.Error = reason
	run.FinishedAt = s.now()
	s.unpublish(run)

	s.wake()
}

func (s *service) unpublish(run *stored) {
	for _, listener := range run.listeners {
		_ = listener.Close()
	}

	run.listeners = nil
	run.Endpoints = nil
}

func (s *service) stop(rw http.ResponseWriter, r *http.Request) {
	if !stopRequest(rw, r) {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if run, ok := s.found(rw, r); ok {
		s.end(run, stoppedBySignal, "")
		answer(rw, http.StatusOK, run.Run)
	}
}

// stopRequest reads a stop's optional body, as the contract allows it to be
// missing altogether.
func stopRequest(rw http.ResponseWriter, r *http.Request) bool {
	body, _ := io.ReadAll(r.Body)
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}

	var request api.StopRequest
	if err := json.Unmarshal(body, &request); err != nil {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "the stop request could not be read")

		return false
	}

	return true
}

func (s *service) kill(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if run, ok := s.found(rw, r); ok {
		s.end(run, killed, "")
		answer(rw, http.StatusOK, run.Run)
	}
}

func (s *service) restart(rw http.ResponseWriter, r *http.Request) {
	if !stopRequest(rw, r) {
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	run, ok := s.found(rw, r)
	if !ok {
		return
	}

	// a restart somebody asked for is not one of the policy's.
	s.end(run, stoppedBySignal, "")
	s.boot(run)

	answer(rw, http.StatusOK, run.Run)
}

func (s *service) logs(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	var since time.Time
	if raw := query.Get(api.QuerySince); len(raw) > 0 {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			refuse(rw, http.StatusBadRequest, api.CodeInvalid, "since is not a time")

			return
		}

		since = parsed
	}

	follow := query.Get(api.QueryFollow) == "true"

	s.lock.Lock()
	run, ok := s.found(rw, r)
	s.lock.Unlock()

	if !ok {
		return
	}

	rw.Header().Set("Content-Type", api.ContentTypeNDJSON)
	rw.WriteHeader(http.StatusOK)

	flusher := http.NewResponseController(rw)
	encoder := json.NewEncoder(rw)

	for sent := 0; ; {
		s.lock.Lock()
		lines := slices.Clone(run.lines[sent:])
		state := run.State
		changed := s.changed
		s.lock.Unlock()

		for _, line := range lines {
			if line.At.Before(since) {
				continue
			}

			if encoder.Encode(line) != nil || flusher.Flush() != nil {
				return
			}
		}

		sent += len(lines)

		following := state == api.StateRunning || state == api.StateStopping || state == api.StateRestarting
		if !follow || !following {
			return
		}

		select {
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

func (s *service) runStats(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	run, ok := s.found(rw, r)
	if !ok {
		return
	}

	if run.State != api.StateRunning {
		refuse(rw, http.StatusConflict, api.CodeNotRunning, "the run is not running")

		return
	}

	answer(rw, http.StatusOK, run.stats)
}

func (s *service) nodeStats(rw http.ResponseWriter, r *http.Request) {
	node := r.URL.Query().Get(api.QueryNode)
	if len(node) == 0 {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "stats are summed over a node")

		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	var sum api.Stats
	for _, run := range s.runs {
		if run.Node != node || run.State != api.StateRunning {
			continue
		}

		sum.CPUPercent += run.stats.CPUPercent
		sum.MemoryUsage += run.stats.MemoryUsage
		sum.MemoryLimit += run.stats.MemoryLimit
		sum.NetworkInput += run.stats.NetworkInput
		sum.NetworkOutput += run.stats.NetworkOutput
		sum.BlockInput += run.stats.BlockInput
		sum.BlockOutput += run.stats.BlockOutput
	}

	answer(rw, http.StatusOK, sum)
}

func (s *service) pull(rw http.ResponseWriter, r *http.Request) {
	var request api.PullRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Reference) == 0 {
		refuse(rw, http.StatusBadRequest, api.CodeInvalid, "a pull names its image")

		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if !slices.Contains(s.pulled, request.Reference) {
		s.pulled = append(s.pulled, request.Reference)
	}

	rw.WriteHeader(http.StatusNoContent)
}

// exec runs the stand-in for a command: it says it started, hands back what it
// is given on stdin as its stdout, says its terminal's size on stdout whenever
// it is resized, and exits once its stdin is closed. A command that is exit
// and a number writes a line to stderr and exits with that number at once.
func (s *service) exec(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	run, ok := s.found(rw, r)
	running := ok && run.State == api.StateRunning
	s.lock.Unlock()

	if !ok {
		return
	}

	if !running {
		refuse(rw, http.StatusConflict, api.CodeNotRunning, "a command runs in a run that is running")

		return
	}

	upgrader := websocket.Upgrader{Subprotocols: []string{api.ExecSubprotocol}}

	conn, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var request api.ExecRequest
	if err := conn.ReadJSON(&request); err != nil {
		return
	}

	if len(request.Command) == 0 {
		_ = conn.WriteJSON(api.Control{Type: api.ControlError, Error: &api.Error{Code: api.CodeInvalid, Message: "a command has to be named"}})

		return
	}

	s.lock.Lock()
	execID := fmt.Sprintf("exec-%d", len(s.sessions)+1)
	s.sessions[execID] = &session{runID: run.ID, request: request, rows: request.Rows, cols: request.Cols}
	s.lock.Unlock()

	if err := conn.WriteJSON(api.Control{Type: api.ControlStarted, ExecID: execID}); err != nil {
		return
	}

	output := func(stream byte, data string) error {
		return conn.WriteMessage(websocket.BinaryMessage, append([]byte{stream}, data...))
	}

	exit := func(code int) {
		_ = conn.WriteJSON(api.Control{Type: api.ControlExit, Code: &code})
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}

	if request.Command[0] == "exit" {
		code, _ := strconv.Atoi(request.Command[1])

		_ = output(api.OutputStdout, "leaving\n")
		_ = output(api.OutputStderr, "with "+request.Command[1]+"\n")
		exit(code)

		return
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			// the client let go of the stream; the command carries on.
			return
		}

		if messageType == websocket.BinaryMessage {
			if output(api.OutputStdout, string(payload)) != nil {
				return
			}

			continue
		}

		var control api.Control
		if json.Unmarshal(payload, &control) != nil {
			continue
		}

		switch control.Type {
		case api.ControlResize:
			s.lock.Lock()
			s.sessions[execID].rows, s.sessions[execID].cols = control.Rows, control.Cols
			s.lock.Unlock()

			if output(api.OutputStdout, fmt.Sprintf("%d %d\n", control.Rows, control.Cols)) != nil {
				return
			}
		case api.ControlCloseStdin:
			exit(0)

			return
		}
	}
}

func (s *service) endExec(rw http.ResponseWriter, r *http.Request) {
	s.lock.Lock()
	defer s.lock.Unlock()

	found, ok := s.sessions[r.PathValue(api.WildcardExec)]
	if !ok || found.runID != r.PathValue(api.WildcardRun) || found.ended {
		refuse(rw, http.StatusNotFound, api.CodeNotFound, "no such exec")

		return
	}

	found.ended = true

	rw.WriteHeader(http.StatusNoContent)
}
