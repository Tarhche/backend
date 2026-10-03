package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/dialVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/endExec"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/execVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getImages"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getInfo"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMLogs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMStats"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/killVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/prepareImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/removeNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/restartVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/startVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/stopVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/resources/translation"
)

// served is vmhost's API served on a real unix socket, in front of an engine
// made of in-memory fakes, and a client that speaks to it the way the
// orchestrator's microvm driver does.
type served struct {
	host   *vmhosttest.Host
	socket string
	client *http.Client
}

func serve(t *testing.T) *served {
	t.Helper()

	host := vmhosttest.New(t, nil)

	engine := host.Engine
	validate := validator.New(translator.New(translation.Translations, translation.EN))

	handler := NewHandler(UseCases{
		GetInfo:       getInfo.NewUseCase(engine),
		PrepareImage:  prepareImage.NewUseCase(engine, validate),
		GetImages:     getImages.NewUseCase(engine),
		DeleteImage:   deleteImage.NewUseCase(engine, validate),
		EnsureNetwork: ensureNetwork.NewUseCase(engine, validate),
		RemoveNetwork: removeNetwork.NewUseCase(engine, validate),
		CreateVM:      createVM.NewUseCase(engine, validate, createVM.Limits{MaxMemory: 2 << 30, MaxCPU: 2}),
		GetVMs:        getVMs.NewUseCase(engine, validate),
		GetVM:         getVM.NewUseCase(engine, validate),
		StartVM:       startVM.NewUseCase(engine, validate),
		StopVM:        stopVM.NewUseCase(engine, validate),
		RestartVM:     restartVM.NewUseCase(engine, validate),
		KillVM:        killVM.NewUseCase(engine, validate),
		DeleteVM:      deleteVM.NewUseCase(engine, validate),
		GetVMLogs:     getVMLogs.NewUseCase(engine, validate),
		GetVMStats:    getVMStats.NewUseCase(engine, validate),
		ExecVM:        execVM.NewUseCase(engine, validate),
		EndExec:       endExec.NewUseCase(engine, validate),
		DialVM:        dialVM.NewUseCase(engine, validate),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// a socket's path is at most 104 bytes on some systems: a test's own
	// temporary directory is too deep for one.
	dir, err := os.MkdirTemp("", "vmh")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	socket := filepath.Join(dir, "vmhost.sock")

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(ctx)
	})

	dial := func(ctx context.Context, _ string, _ string) (net.Conn, error) {
		var dialer net.Dialer

		return dialer.DialContext(ctx, "unix", socket)
	}

	return &served{
		host:   host,
		socket: socket,
		client: &http.Client{Transport: &http.Transport{DialContext: dial}, Timeout: 30 * time.Second},
	}
}

// do asks vmhost for something, and reads its answer into answer when it
// went well, or hands back the error it carried when it did not.
func (s *served) do(t *testing.T, method string, path string, body any, answer any) (int, error) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, "http://vmhost"+path, payload)
	require.NoError(t, err)

	response, err := s.client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	if response.StatusCode >= http.StatusBadRequest {
		var refused vm.ErrorResponse
		require.NoError(t, json.Unmarshal(content, &refused), string(content))

		return response.StatusCode, refused.Err()
	}

	if answer != nil {
		require.NoError(t, json.Unmarshal(content, answer), string(content))
	}

	return response.StatusCode, nil
}

// upgrade asks for a stream, as the microvm driver does: on a connection of
// its own, which it reads the switch off and then carries the stream.
func (s *served) upgrade(t *testing.T, path string, protocol string, body any) (net.Conn, *http.Response) {
	t.Helper()

	conn, err := net.Dial("unix", s.socket)
	require.NoError(t, err)

	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}

	request, err := http.NewRequest(http.MethodPost, "http://vmhost"+path, bytes.NewReader(payload))
	require.NoError(t, err)

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", protocol)
	request.Header.Set("Content-Type", "application/json")

	require.NoError(t, request.Write(conn))

	reader := bufio.NewReader(conn)

	response, err := http.ReadResponse(reader, request)
	require.NoError(t, err)

	return &bufferedConn{Conn: conn, reader: reader}, response
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func TestAPI(t *testing.T) {
	t.Parallel()

	s := serve(t)

	t.Run("vmhost says what it is", func(t *testing.T) {
		var info vm.Info

		status, err := s.do(t, http.MethodGet, vm.PathInfo, nil, &info)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, status)

		assert.True(t, info.Healthy)
		assert.Equal(t, guest.ProtocolVersion, info.GuestVersion)
		assert.True(t, info.Capacity.Reserved)
	})

	t.Run("an image is made ready, and listed as a list", func(t *testing.T) {
		var image vm.Image

		_, err := s.do(t, http.MethodPost, vm.PathPrepareImage, vm.PrepareImage{Image: "nginx:alpine"}, &image)
		require.NoError(t, err)
		assert.Equal(t, "nginx:alpine", image.Reference)

		var images []vm.Image

		_, err = s.do(t, http.MethodGet, vm.PathImages, nil, &images)
		require.NoError(t, err)
		require.Len(t, images, 1)

		_, err = s.do(t, http.MethodDelete, vm.PathImage(image.Digest), nil, nil)
		require.NoError(t, err)

		_, err = s.do(t, http.MethodDelete, vm.PathImage(image.Digest), nil, nil)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("networks are made and taken away, and what clashes says so", func(t *testing.T) {
		var network vm.Network

		_, err := s.do(t, http.MethodPut, vm.PathNetwork("workload-stack-shop"), vm.NetworkSpec{}, &network)
		require.NoError(t, err)
		assert.Equal(t, "workload-stack-shop", network.Name)
		assert.False(t, network.Masquerade)

		_, err = s.do(t, http.MethodPut, vm.PathNetwork("workload-stack-shop"), vm.NetworkSpec{Masquerade: true}, nil)
		assert.ErrorIs(t, err, vm.ErrConflict)

		status, err := s.do(t, http.MethodPut, vm.PathNetwork("Not_A_Network"), vm.NetworkSpec{}, nil)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		_, err = s.do(t, http.MethodDelete, vm.PathNetwork("workload-stack-shop"), nil, nil)
		require.NoError(t, err)
	})

	t.Run("a vm lives its whole life through the api", func(t *testing.T) {
		spec := vmhosttest.Spec("web")

		var created vm.Created

		status, err := s.do(t, http.MethodPost, vm.PathVMs, spec, &created)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, status)
		require.True(t, vm.IsID(created.ID))

		id := created.ID

		_, err = s.do(t, http.MethodPost, vm.PathVMs, spec, nil)
		assert.ErrorIs(t, err, vm.ErrConflict, "a name another vm answers to")

		var listed []vm.VM

		_, err = s.do(t, http.MethodGet, vm.PathVMs+"?"+url.Values{vm.QueryLabel: {"node.name=orchestrator-01", "task.slug=web"}}.Encode(), nil, &listed)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, id, listed[0].ID)

		_, err = s.do(t, http.MethodGet, vm.PathVMs+"?"+url.Values{vm.QueryLabel: {"node.name=orchestrator-02"}}.Encode(), nil, &listed)
		require.NoError(t, err)
		assert.NotNil(t, listed, "none is an empty list")
		assert.Empty(t, listed)

		status, err = s.do(t, http.MethodPost, vm.PathVMAction(id, "start"), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status)

		var running vm.VM

		_, err = s.do(t, http.MethodGet, vm.PathVM(id), nil, &running)
		require.NoError(t, err)
		assert.Equal(t, vm.StateRunning, running.State)
		assert.Equal(t, spec.Labels, running.Spec.Labels, "a vm is answered with its labels")
		assert.Equal(t, []uint16{80}, running.Endpoints())

		s.host.Agent(t, id).SetStats(guest.Stats{PIDs: 4, MemoryUsage: 32 << 20})

		var stats vm.Stats

		_, err = s.do(t, http.MethodGet, vm.PathVMAction(id, "stats"), nil, &stats)
		require.NoError(t, err)
		assert.Equal(t, uint64(4), stats.PIDs)
		assert.Equal(t, uint64(256<<20), stats.MemoryLimit)

		status, err = s.do(t, http.MethodPost, vm.PathVMAction(id, "restart"), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status)

		status, err = s.do(t, http.MethodPost, vm.PathVMAction(id, "stop")+"?"+url.Values{vm.QueryTimeout: {"1s"}}.Encode(), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status)

		var stopped vm.VM

		_, err = s.do(t, http.MethodGet, vm.PathVM(id), nil, &stopped)
		require.NoError(t, err)
		assert.Equal(t, vm.StateExited, stopped.State)
		assert.Equal(t, 143, stopped.ExitCode)

		_, err = s.do(t, http.MethodPost, vm.PathVMAction(id, "kill"), nil, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		_, err = s.do(t, http.MethodPost, vm.PathVMAction(id, "stop")+"?"+url.Values{vm.QueryTimeout: {"soon"}}.Encode(), nil, nil)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		status, err = s.do(t, http.MethodDelete, vm.PathVM(id), nil, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, status)

		status, err = s.do(t, http.MethodGet, vm.PathVM(id), nil, nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("what is not valid is refused, naming what is not", func(t *testing.T) {
		spec := vmhosttest.Spec("../escape")
		spec.Resources.Memory = 8 << 30

		status, err := s.do(t, http.MethodPost, vm.PathVMs, spec, nil)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.ErrorIs(t, err, vm.ErrInvalid)
		assert.Contains(t, err.Error(), "name")

		_, err = s.do(t, http.MethodGet, vm.PathVM("not-an-id"), nil, nil)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		request, err := http.NewRequest(http.MethodPost, "http://vmhost"+vm.PathVMs, strings.NewReader("{"))
		require.NoError(t, err)

		response, err := s.client.Do(request)
		require.NoError(t, err)
		response.Body.Close()
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	})

	t.Run("no room left travels as the error it is", func(t *testing.T) {
		// 2 GiB and its VMM's overhead fit the 4 GiB budget once.
		huge := vmhosttest.Spec("huge")
		huge.Resources.Memory = 2 << 30

		var created vm.Created

		_, err := s.do(t, http.MethodPost, vm.PathVMs, huge, &created)
		require.NoError(t, err)

		huge.Name = "huge-again"

		status, err := s.do(t, http.MethodPost, vm.PathVMs, huge, nil)
		assert.Equal(t, http.StatusConflict, status)
		assert.ErrorIs(t, err, vm.ErrCapacity)

		_, err = s.do(t, http.MethodDelete, vm.PathVM(created.ID), nil, nil)
		require.NoError(t, err)
	})

	t.Run("output is answered a line at a time as it is written, after the last line the reader has", func(t *testing.T) {
		id := s.run(t, "logs")
		agent := s.host.Agent(t, id)

		agent.Write(guest.StreamStdout, "before")

		request, err := http.NewRequest(http.MethodGet, "http://vmhost"+vm.PathVMAction(id, "logs")+"?"+url.Values{vm.QueryFollow: {"1"}}.Encode(), nil)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		response, err := s.client.Do(request.WithContext(ctx))
		require.NoError(t, err, "the answer begins before any line is there to send")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "application/x-ndjson", response.Header.Get("Content-Type"))

		lines := bufio.NewScanner(response.Body)

		next := func() vm.LogLine {
			t.Helper()

			require.True(t, lines.Scan(), "no line came")

			var line vm.LogLine
			require.NoError(t, json.Unmarshal(lines.Bytes(), &line))

			return line
		}

		assert.Equal(t, "before", next().Content)

		agent.Write(guest.StreamStderr, "after")

		line := next()
		assert.Equal(t, "after", line.Content)
		assert.Equal(t, guest.StreamStderr, line.Stream)
		assert.Equal(t, uint64(2), line.Seq)

		var rest []string

		answer, err := s.client.Get("http://vmhost" + vm.PathVMAction(id, "logs") + "?" + url.Values{vm.QueryAfter: {"1"}}.Encode())
		require.NoError(t, err)
		defer answer.Body.Close()

		after := bufio.NewScanner(answer.Body)
		for after.Scan() {
			var kept vm.LogLine
			require.NoError(t, json.Unmarshal(after.Bytes(), &kept))
			rest = append(rest, kept.Content)
		}

		assert.Equal(t, []string{"after"}, rest)

		_, err = s.do(t, http.MethodGet, vm.PathVMAction(id, "logs")+"?"+url.Values{vm.QuerySince: {"yesterday"}}.Encode(), nil, nil)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		_, err = s.do(t, http.MethodGet, vm.PathVMAction("0123456789abcdef", "logs"), nil, nil)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("a command runs beside the task, its frames carried both ways, and is ended", func(t *testing.T) {
		id := s.run(t, "exec")

		conn, response := s.upgrade(t, vm.PathVMAction(id, "exec"), vm.UpgradeExec, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true})
		defer conn.Close()

		require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
		assert.Equal(t, vm.UpgradeExec, response.Header.Get("Upgrade"))

		execID := response.Header.Get(vm.ExecIDHeader)
		require.NotEmpty(t, execID)

		require.NoError(t, guest.WriteFrame(conn, guest.FrameStdin, []byte("echo 42\n")))

		frame, err := guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, guest.FrameStdout, frame.Type)
		assert.Equal(t, "echo 42\n", string(frame.Payload))

		require.NoError(t, guest.WriteFrame(conn, guest.FrameResize, guest.ResizePayload(24, 80)))

		frame, err = guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, "24 80", string(frame.Payload))

		require.NoError(t, guest.WriteFrame(conn, guest.FrameCloseStdin, nil))

		frame, err = guest.ReadFrame(conn)
		require.NoError(t, err)
		assert.Equal(t, guest.FrameExit, frame.Type)

		code, err := guest.ParseExit(frame.Payload)
		require.NoError(t, err)
		assert.Equal(t, 0, code)

		var ended guest.Ended

		_, err = s.do(t, http.MethodPost, vm.PathEndExec(id, execID), guest.EndExec{Grace: time.Second}, &ended)
		require.NoError(t, err)
		assert.True(t, ended.Signalled)
	})

	t.Run("a task's port is reached through a switched connection, byte for byte", func(t *testing.T) {
		id := s.run(t, "dial")

		conn, response := s.upgrade(t, vm.PathVMAction(id, "dial")+"?"+url.Values{vm.QueryPort: {"80"}}.Encode(), vm.UpgradeDial, nil)
		defer conn.Close()

		require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
		assert.Equal(t, vm.UpgradeDial, response.Header.Get("Upgrade"))

		_, err := conn.Write([]byte("GET / HTTP/1.1\r\n"))
		require.NoError(t, err)

		echoed, err := bufio.NewReader(conn).ReadString('\n')
		require.NoError(t, err)
		assert.Equal(t, "GET / HTTP/1.1\r\n", echoed)

		refusedConn, refusedResponse := s.upgrade(t, vm.PathVMAction(id, "dial")+"?"+url.Values{vm.QueryPort: {"22"}}.Encode(), vm.UpgradeDial, nil)
		defer refusedConn.Close()
		defer refusedResponse.Body.Close()

		assert.Equal(t, http.StatusBadRequest, refusedResponse.StatusCode, "a port the task is not reached on")

		wrongConn, wrongResponse := s.upgrade(t, vm.PathVMAction(id, "dial")+"?"+url.Values{vm.QueryPort: {"80"}}.Encode(), "websocket", nil)
		defer wrongConn.Close()
		defer wrongResponse.Body.Close()

		assert.Equal(t, http.StatusBadRequest, wrongResponse.StatusCode, "a protocol the route does not switch to")
	})
}

// run makes a VM and starts it, through the API.
func (s *served) run(t *testing.T, name string) string {
	t.Helper()

	var created vm.Created

	_, err := s.do(t, http.MethodPost, vm.PathVMs, vmhosttest.Spec(name), &created)
	require.NoError(t, err)

	_, err = s.do(t, http.MethodPost, vm.PathVMAction(created.ID, "start"), nil, nil)
	require.NoError(t, err)

	return created.ID
}
