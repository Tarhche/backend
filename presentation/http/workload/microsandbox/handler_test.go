package microsandbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	serverName = "workload-microsandbox"
	node       = "orchestrator-1"
	image      = "nginx:alpine"
	eventually = 3 * time.Second
	tick       = 2 * time.Millisecond
)

// service is the API over a supervisor over a fake microsandbox, behind mutual
// TLS as the service serves it.
type service struct {
	t *testing.T

	fake       *fakes.FakeSandboxes
	supervisor *runs.Supervisor
	server     *httptest.Server
	client     *http.Client
	clientTLS  *tls.Config
}

func newService(t *testing.T, change func(*runs.Config)) *service {
	t.Helper()

	return startService(t, change, true)
}

func startService(t *testing.T, change func(*runs.Config), open bool) *service {
	t.Helper()

	authority, err := certificate.GenerateCA("test authority", 0)
	require.NoError(t, err)

	authorityPEM := string(certificate.EncodeCertificate(authority.Certificate))

	serverCertificate, serverKey, err := authority.GenerateServerCertificate(certificate.Request{
		Name:        serverName,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	require.NoError(t, err)

	clientCertificate, clientKey, err := authority.GenerateClientCertificate(certificate.Request{Name: node})
	require.NoError(t, err)

	serverTLS, err := certificate.ServerTLSConfig(certificate.Credentials{
		Authority:   authorityPEM,
		Certificate: string(certificate.EncodeCertificate(serverCertificate)),
		PrivateKey:  pemOf(t, serverKey),
	})
	require.NoError(t, err)

	clientTLS, err := certificate.ClientTLSConfig(certificate.Credentials{
		Authority:   authorityPEM,
		Certificate: string(certificate.EncodeCertificate(clientCertificate)),
		PrivateKey:  pemOf(t, clientKey),
		ServerName:  serverName,
	})
	require.NoError(t, err)

	fake := fakes.NewFakeSandboxes()
	fake.CacheImage(image, runs.ImageConfig{Entrypoint: []string{"/docker-entrypoint.sh"}, Cmd: []string{"nginx"}})

	config := runs.DefaultConfig()
	config.Budget = 4 << 30
	config.StopGrace = 200 * time.Millisecond
	config.KillGrace = 100 * time.Millisecond
	config.VMStopTimeout = 50 * time.Millisecond
	config.Backoff = runs.Backoff{Initial: time.Hour, Max: time.Hour, ResetAfter: time.Hour}
	config.ExecEndGrace = 20 * time.Millisecond
	config.ExecKillGrace = 20 * time.Millisecond
	config.CallTimeout = 2 * time.Second
	config.BootTimeout = 2 * time.Second
	config.QueueTimeout = 2 * time.Second
	config.PullTimeout = 2 * time.Second
	config.RetryInterval = 5 * time.Millisecond
	config.RetryMaxInterval = 5 * time.Millisecond
	config.ServiceVersion = "test"

	if change != nil {
		change(&config)
	}

	logger := slog.New(slog.DiscardHandler)

	supervisor := runs.New(fake, fakes.NewMemoryRecords(), fakes.NewMemoryJournal(), fakes.NewMemoryHostPorts(20000, 20999), config, logger)

	if open {
		require.NoError(t, supervisor.Open(context.Background()))
	}

	server := httptest.NewUnstartedServer(NewHandler(supervisor, logger))
	server.TLS = serverTLS
	server.StartTLS()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = supervisor.Shutdown(ctx)
		server.Close()
	})

	return &service{
		t:          t,
		fake:       fake,
		supervisor: supervisor,
		server:     server,
		client:     &http.Client{Transport: &http.Transport{TLSClientConfig: clientTLS}, Timeout: 10 * time.Second},
		clientTLS:  clientTLS,
	}
}

func pemOf(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()

	encoded, err := certificate.EncodePrivateKey(key)
	require.NoError(t, err)

	return string(encoded)
}

// call makes a request, and decodes a JSON answer into into, if there is one
// to decode into.
func (s *service) call(method, path string, body any, into any) *http.Response {
	s.t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(s.t, err)

		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, s.server.URL+path, reader)
	require.NoError(s.t, err)

	response, err := s.client.Do(request)
	require.NoError(s.t, err)

	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	require.NoError(s.t, err)

	if into != nil && len(data) > 0 {
		require.NoError(s.t, json.Unmarshal(data, into), string(data))
	}

	return response
}

// failure makes a request that has to fail, and is the failure.
func (s *service) failure(method, path string, body any, status int) api.Error {
	s.t.Helper()

	var answer api.ErrorResponse

	response := s.call(method, path, body, &answer)

	require.Equal(s.t, status, response.StatusCode)
	assert.Equal(s.t, "application/json", response.Header.Get("Content-Type"))

	return answer.Error
}

func (s *service) created(name string) api.Run {
	s.t.Helper()

	var run api.Run

	response := s.call(http.MethodPost, "/v1/runs", runSpec(name), &run)
	require.Equal(s.t, http.StatusCreated, response.StatusCode)

	return run
}

func (s *service) started(name string) api.Run {
	s.t.Helper()

	run := s.created(name)

	var started api.Run

	response := s.call(http.MethodPost, "/v1/runs/"+run.ID+"/start", nil, &started)
	require.Equal(s.t, http.StatusOK, response.StatusCode)
	require.Equal(s.t, api.StateRunning, started.State)

	return started
}

func (s *service) main(id string) *fakes.FakeProcess {
	s.t.Helper()

	process, found := s.fake.Main(runs.SandboxName(id), 1)
	require.True(s.t, found)

	return process
}

func runSpec(name string) api.RunSpec {
	return api.RunSpec{
		Node:    node,
		Name:    name,
		Image:   image,
		CPU:     0.5,
		Memory:  128 << 20,
		Network: api.NetworkIsolated,
		Task:    api.Task{UUID: "task-" + name, Slug: name, Kind: "service"},
	}
}

func TestTransport(t *testing.T) {
	t.Parallel()

	s := newService(t, nil)

	t.Run("an orchestrator's certificate is let in", func(t *testing.T) {
		t.Parallel()

		var info api.Info

		response := s.call(http.MethodGet, "/v1/info", nil, &info)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, uint16(tls.VersionTLS13), response.TLS.Version)
		assert.Equal(t, api.Version, info.APIVersion)
		assert.True(t, info.Ready)
		assert.Equal(t, "0.7.6", info.MicrosandboxVersion)
		assert.Equal(t, "test", info.ServiceVersion)
	})

	t.Run("a client with no certificate is not", func(t *testing.T) {
		t.Parallel()

		anonymous := s.clientTLS.Clone()
		anonymous.GetClientCertificate = nil

		client := &http.Client{Transport: &http.Transport{TLSClientConfig: anonymous}}

		_, err := client.Get(s.server.URL + "/v1/info")

		assert.Error(t, err)
	})

	t.Run("a client whose certificate another authority signed is not", func(t *testing.T) {
		t.Parallel()

		stranger := newService(t, nil)

		client := &http.Client{Transport: &http.Transport{TLSClientConfig: stranger.clientTLS}}

		_, err := client.Get(s.server.URL + "/v1/info")

		assert.Error(t, err)
	})
}

func TestRuns(t *testing.T) {
	t.Parallel()

	t.Run("a run's whole life, as the contract answers it", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.created("web")
		assert.Equal(t, api.StateCreated, run.State)
		assert.Equal(t, runSpec("web"), run.RunSpec)

		var got api.Run
		require.Equal(t, http.StatusOK, s.call(http.MethodGet, "/v1/runs/"+run.ID, nil, &got).StatusCode)
		assert.Equal(t, run.ID, got.ID)

		var list api.RunList
		require.Equal(t, http.StatusOK, s.call(http.MethodGet, "/v1/runs?node="+node+"&task=task-web", nil, &list).StatusCode)
		require.Len(t, list.Runs, 1)
		assert.Equal(t, run.ID, list.Runs[0].ID)

		var started api.Run
		require.Equal(t, http.StatusOK, s.call(http.MethodPost, "/v1/runs/"+run.ID+"/start", nil, &started).StatusCode)
		assert.Equal(t, api.StateRunning, started.State)

		var stopped api.Run
		require.Equal(t, http.StatusOK, s.call(http.MethodPost, "/v1/runs/"+run.ID+"/stop", api.StopRequest{TimeoutSeconds: 1}, &stopped).StatusCode)
		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, 143, stopped.ExitCode)

		var restarted api.Run
		require.Equal(t, http.StatusOK, s.call(http.MethodPost, "/v1/runs/"+run.ID+"/restart", nil, &restarted).StatusCode)
		assert.Equal(t, api.StateRunning, restarted.State)

		var killed api.Run
		require.Equal(t, http.StatusOK, s.call(http.MethodPost, "/v1/runs/"+run.ID+"/kill", nil, &killed).StatusCode)
		assert.Equal(t, 137, killed.ExitCode)

		response := s.call(http.MethodDelete, "/v1/runs/"+run.ID, nil, nil)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)

		failure := s.failure(http.MethodDelete, "/v1/runs/"+run.ID, nil, http.StatusNotFound)
		assert.Equal(t, api.CodeNotFound, failure.Code)
	})

	t.Run("an empty listing is a list", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		request, err := http.NewRequest(http.MethodGet, s.server.URL+"/v1/runs?node=nobody", nil)
		require.NoError(t, err)

		response, err := s.client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)

		assert.JSONEq(t, `{"runs":[]}`, string(body))
	})

	t.Run("failures are the contract's, with the status for their code", func(t *testing.T) {
		t.Parallel()

		s := newService(t, func(c *runs.Config) { c.Budget = 128<<20 + runs.DefaultOverhead })
		s.fake.AddImage("missing", runs.ImageConfig{})
		s.fake.FailPull("missing", fmt.Errorf("registry unreachable"))

		running := s.started("web")
		created := s.created("other")

		tests := []struct {
			name   string
			method string
			path   string
			body   any
			status int
			code   string
		}{
			{"a listing without a node", http.MethodGet, "/v1/runs", nil, http.StatusBadRequest, api.CodeInvalid},
			{"a spec that does not hold", http.MethodPost, "/v1/runs", api.RunSpec{Node: node}, http.StatusBadRequest, api.CodeInvalid},
			{"a body that is no spec", http.MethodPost, "/v1/runs", "not a spec", http.StatusBadRequest, api.CodeInvalid},
			{"no body at all", http.MethodPost, "/v1/runs", nil, http.StatusBadRequest, api.CodeInvalid},
			{"a spec microsandbox cannot run", http.MethodPost, "/v1/runs", func() api.RunSpec { s := runSpec("none"); s.Network = "none"; return s }(), http.StatusBadRequest, api.CodeNotSupported},
			{"a name in use", http.MethodPost, "/v1/runs", runSpec("web"), http.StatusConflict, api.CodeNameInUse},
			{"a run that is not there", http.MethodGet, "/v1/runs/nope", nil, http.StatusNotFound, api.CodeNotFound},
			{"starting a run that is not there", http.MethodPost, "/v1/runs/nope/start", nil, http.StatusNotFound, api.CodeNotFound},
			{"a node whose budget cannot take a run", http.MethodPost, "/v1/runs/" + created.ID + "/start", nil, http.StatusConflict, api.CodeCapacity},
			{"a negative stop timeout", http.MethodPost, "/v1/runs/" + running.ID + "/stop", api.StopRequest{TimeoutSeconds: -1}, http.StatusBadRequest, api.CodeInvalid},
			{"stats of a run that is not running", http.MethodGet, "/v1/runs/" + created.ID + "/stats", nil, http.StatusConflict, api.CodeNotRunning},
			{"node stats without a node", http.MethodGet, "/v1/stats", nil, http.StatusBadRequest, api.CodeInvalid},
			{"an image that cannot be pulled", http.MethodPost, "/v1/images/pull", api.PullRequest{Reference: "missing"}, http.StatusBadGateway, api.CodePullFailed},
			{"a pull of nothing", http.MethodPost, "/v1/images/pull", api.PullRequest{}, http.StatusBadRequest, api.CodeInvalid},
			{"ending an exec that is not there", http.MethodPost, "/v1/runs/" + running.ID + "/execs/nope/end", nil, http.StatusNotFound, api.CodeNotFound},
			{"logs of a run that is not there", http.MethodGet, "/v1/runs/nope/logs", nil, http.StatusNotFound, api.CodeNotFound},
			{"logs since what is no time", http.MethodGet, "/v1/runs/" + running.ID + "/logs?since=yesterday", nil, http.StatusBadRequest, api.CodeInvalid},
			{"a route that is not there", http.MethodGet, "/v2/info", nil, http.StatusNotFound, api.CodeNotFound},
		}

		for _, test := range tests {
			failure := s.failure(test.method, test.path, test.body, test.status)

			assert.Equal(t, test.code, failure.Code, test.name)
			assert.NotEmpty(t, failure.Message, test.name)
		}
	})

	t.Run("before it is ready the service says so", func(t *testing.T) {
		t.Parallel()

		s := startService(t, nil, false)

		var info api.Info
		require.Equal(t, http.StatusOK, s.call(http.MethodGet, "/v1/info", nil, &info).StatusCode)
		assert.False(t, info.Ready)
		assert.NotEmpty(t, info.Reason)

		failure := s.failure(http.MethodPost, "/v1/runs", runSpec("web"), http.StatusServiceUnavailable)
		assert.Equal(t, api.CodeUnavailable, failure.Code)

		failure = s.failure(http.MethodGet, "/v1/runs?node="+node, nil, http.StatusServiceUnavailable)
		assert.Equal(t, api.CodeUnavailable, failure.Code)
	})

	t.Run("stats of a run, and of its node", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.started("web")
		s.fake.SetMetrics(runs.SandboxName(run.ID), runs.Metrics{CPUPercent: 7, MemoryUsage: 9, MemoryLimit: 128 << 20})

		var stats api.Stats
		require.Equal(t, http.StatusOK, s.call(http.MethodGet, "/v1/runs/"+run.ID+"/stats", nil, &stats).StatusCode)
		assert.Equal(t, api.Stats{CPUPercent: 7, MemoryUsage: 9, MemoryLimit: 128 << 20}, stats)

		var total api.Stats
		require.Equal(t, http.StatusOK, s.call(http.MethodGet, "/v1/stats?node="+node, nil, &total).StatusCode)
		assert.Equal(t, stats, total)
	})

	t.Run("a pull answers nothing", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)
		s.fake.AddImage("busybox", runs.ImageConfig{Cmd: []string{"sh"}})

		response := s.call(http.MethodPost, "/v1/images/pull", api.PullRequest{Reference: "busybox"}, nil)

		assert.Equal(t, http.StatusNoContent, response.StatusCode)
		assert.Equal(t, 1, s.fake.Pulls("busybox"))
	})
}

func TestLogs(t *testing.T) {
	t.Parallel()

	t.Run("a log is newline-delimited JSON, from since on", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.started("job")
		s.main(run.ID).Write("one\ntwo\n")
		s.main(run.ID).WriteErr("three\n")
		s.main(run.ID).Exit(0)

		require.Eventually(t, func() bool {
			got, err := s.supervisor.Get(run.ID)

			return err == nil && got.State == api.StateExited
		}, eventually, tick)

		response, err := s.client.Get(s.server.URL + "/v1/runs/" + run.ID + "/logs")
		require.NoError(t, err)
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, api.ContentTypeNDJSON, response.Header.Get("Content-Type"))

		lines := readLines(t, response.Body)
		require.Len(t, lines, 3)

		assert.Equal(t, api.LogLine{Stream: api.StreamStdout, At: lines[0].At, Content: "one"}, lines[0])
		assert.Equal(t, api.StreamStderr, lines[2].Stream)

		since := url.QueryEscape(lines[1].At.Format(time.RFC3339Nano))

		resumed, err := s.client.Get(s.server.URL + "/v1/runs/" + run.ID + "/logs?since=" + since)
		require.NoError(t, err)
		defer resumed.Body.Close()

		assert.Equal(t, lines[1:], readLines(t, resumed.Body), "since is inclusive, as docker's is")
	})

	t.Run("a followed log streams each line as it is written, and ends with the run", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.started("web")
		main := s.main(run.ID)

		response, err := s.client.Get(s.server.URL + "/v1/runs/" + run.ID + "/logs?follow=true")
		require.NoError(t, err)
		defer response.Body.Close()

		reader := bufio.NewReader(response.Body)

		main.Write("first\n")

		line, err := reader.ReadBytes('\n')
		require.NoError(t, err)
		assert.Contains(t, string(line), `"content":"first"`)

		main.Write("second\n")

		line, err = reader.ReadBytes('\n')
		require.NoError(t, err)
		assert.Contains(t, string(line), `"content":"second"`)

		main.Exit(0)

		rest, err := io.ReadAll(reader)
		require.NoError(t, err, "the stream ends cleanly once the run has ended")
		assert.Empty(t, rest)
	})
}

func readLines(t *testing.T, body io.Reader) []api.LogLine {
	t.Helper()

	var lines []api.LogLine

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		var line api.LogLine
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &line))

		lines = append(lines, line)
	}

	require.NoError(t, scanner.Err())

	return lines
}

// dial opens a run's exec websocket, offering the given subprotocols.
func (s *service) dial(id string, subprotocols ...string) (*websocket.Conn, *http.Response, error) {
	dialer := websocket.Dialer{
		TLSClientConfig:  s.clientTLS,
		Subprotocols:     subprotocols,
		HandshakeTimeout: 5 * time.Second,
	}

	return dialer.Dial("wss"+strings.TrimPrefix(s.server.URL, "https")+"/v1/runs/"+id+"/exec", nil)
}

func readControl(t *testing.T, conn *websocket.Conn) api.Control {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(eventually))

	messageType, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType, string(payload))

	var control api.Control
	require.NoError(t, json.Unmarshal(payload, &control))

	return control
}

func sendJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
}

func TestExec(t *testing.T) {
	t.Parallel()

	t.Run("a command's frames, from its request to its exit", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.started("web")

		conn, response, err := s.dial(run.ID, api.ExecSubprotocol)
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, api.ExecSubprotocol, response.Header.Get("Sec-WebSocket-Protocol"))

		sendJSON(t, conn, api.ExecRequest{Command: []string{"/bin/sh"}, TTY: true, Rows: 24, Cols: 80})

		started := readControl(t, conn)
		require.Equal(t, api.ControlStarted, started.Type)
		require.NotEmpty(t, started.ExecID)

		var process *fakes.FakeProcess

		require.Eventually(t, func() bool {
			for _, candidate := range s.fake.Processes(runs.SandboxName(run.ID)) {
				if slices.Contains(candidate.Command().Env, "WORKLOAD_TERMINAL_SESSION="+started.ExecID) {
					process = candidate

					return true
				}
			}

			return false
		}, eventually, tick)

		require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, []byte("echo hi\n")))
		sendJSON(t, conn, api.Control{Type: api.ControlResize, Rows: 50, Cols: 132})
		sendJSON(t, conn, api.Control{Type: api.ControlCloseStdin})

		require.Eventually(t, func() bool {
			stdin, closed := process.StdinData()

			return stdin == "echo hi\n" && closed && len(process.Resizes()) == 2
		}, eventually, tick)

		assert.Equal(t, [][2]uint16{{24, 80}, {50, 132}}, process.Resizes())

		process.Write("hi\r\n")
		process.WriteErr("oops")

		_ = conn.SetReadDeadline(time.Now().Add(eventually))

		messageType, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, websocket.BinaryMessage, messageType)
		assert.Equal(t, append([]byte{api.OutputStdout}, "hi\r\n"...), payload)

		messageType, payload, err = conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, websocket.BinaryMessage, messageType)
		assert.Equal(t, append([]byte{api.OutputStderr}, "oops"...), payload)

		sendJSON(t, conn, api.Control{Type: api.ControlSignal, Signal: 2})

		exit := readControl(t, conn)
		require.Equal(t, api.ControlExit, exit.Type)
		require.NotNil(t, exit.Code)
		assert.Equal(t, 130, *exit.Code)

		_, _, err = conn.ReadMessage()
		assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "the service closes once it has said how the command ended: %v", err)
	})

	t.Run("closing the websocket leaves the command running, and ending it is a call of its own", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		run := s.started("web")

		conn, _, err := s.dial(run.ID, api.ExecSubprotocol)
		require.NoError(t, err)

		sendJSON(t, conn, api.ExecRequest{Command: []string{"sleep", "100"}})

		started := readControl(t, conn)
		require.Equal(t, api.ControlStarted, started.Type)

		require.NoError(t, conn.Close())

		time.Sleep(20 * time.Millisecond)

		var process *fakes.FakeProcess
		for _, candidate := range s.fake.Processes(runs.SandboxName(run.ID)) {
			if slices.Contains(candidate.Command().Env, "WORKLOAD_TERMINAL_SESSION="+started.ExecID) {
				process = candidate
			}
		}

		require.NotNil(t, process)
		assert.False(t, process.Ended(), "a terminal whose connection dropped is not killed with the connection")

		response := s.call(http.MethodPost, "/v1/runs/"+run.ID+"/execs/"+started.ExecID+"/end", nil, nil)

		assert.Equal(t, http.StatusNoContent, response.StatusCode)
		assert.True(t, process.Ended())
	})

	t.Run("what cannot run is answered with an error frame", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)
		s.fake.SpawnFails("bash", "ENOENT")

		run := s.started("web")

		for _, test := range []struct {
			name    string
			request any
			message string
		}{
			{"a program that is not there", api.ExecRequest{Command: []string{"bash"}}, "ENOENT"},
			{"no command", api.ExecRequest{}, "command is required"},
			{"a request that is not one", "nonsense", "not an ExecRequest"},
		} {
			conn, _, err := s.dial(run.ID, api.ExecSubprotocol)
			require.NoError(t, err)

			sendJSON(t, conn, test.request)

			failure := readControl(t, conn)

			assert.Equal(t, api.ControlError, failure.Type, test.name)
			require.NotNil(t, failure.Error, test.name)
			assert.Equal(t, api.CodeInvalid, failure.Error.Code, test.name)
			assert.Contains(t, failure.Error.Message, test.message, test.name)

			require.NoError(t, conn.Close())
		}
	})

	t.Run("what can be refused before the upgrade is refused with a status", func(t *testing.T) {
		t.Parallel()

		s := newService(t, nil)

		running := s.started("web")
		created := s.created("other")

		_, response, err := s.dial(running.ID)
		require.Error(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "a client that does not speak the frames is not let in")

		_, response, err = s.dial(created.ID, api.ExecSubprotocol)
		require.Error(t, err)
		assert.Equal(t, http.StatusConflict, response.StatusCode)

		_, response, err = s.dial("nope", api.ExecSubprotocol)
		require.Error(t, err)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}
