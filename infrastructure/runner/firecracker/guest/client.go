package guest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/vsock"
)

const (
	// maxAnswer bounds what is read back of any answer that is not a stream.
	maxAnswer = 1 << 20

	// maxLogLine bounds one line of a log stream as it travels: a line is
	// capped at 64 KiB inside the guest, and escaping it can grow it.
	maxLogLine = 512 << 10

	// maxErrorBody is how much of a refusal is kept to say why.
	maxErrorBody = 4 << 10

	// dialTimeout is how long connecting to the socket firecracker exposes
	// for a machine may take, which is local and quick or not there at all.
	dialTimeout = time.Second

	// readyInterval is how often an agent that is not answering yet is asked
	// again while a machine boots.
	readyInterval = 50 * time.Millisecond

	// the address requests are made to. It names nothing: every connection
	// goes to the one agent this client was made for.
	agentAddress = "http://agent"
)

// ErrNotRunning is an agent refusing what only a running task can do.
var ErrNotRunning = errors.New("the task is not running")

// Client speaks to the agent inside one machine, through the unix socket the
// machine's firecracker process exposes for its vsock.
type Client struct {
	socketPath string
	http       *http.Client
}

// NewClient builds a client for the machine whose vsock is exposed at
// socketPath.
func NewClient(socketPath string) *Client {
	c := &Client{socketPath: socketPath}

	c.http = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
				return c.dial(ctx)
			},
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	return c
}

// dial opens one connection to the agent. Firecracker takes it on the unix
// socket and passes it to whatever listens on the port inside the guest; an
// agent that is not listening yet reads as a connection that is closed as soon
// as it is made, which is tried again until ctx says otherwise.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	return vsock.DialContext(ctx, c.socketPath, Port, vsock.WithDialTimeout(dialTimeout))
}

// Close lets go of the connections kept for the next request.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// Ready waits until the agent answers, which is when a machine has booted.
func (c *Client) Ready(ctx context.Context) error {
	for {
		attempt, cancel := context.WithTimeout(ctx, time.Second)
		err := c.do(attempt, http.MethodGet, "/health", nil, nil)
		cancel()

		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(readyInterval):
		}
	}
}

// Configure tells a machine what it is, once, before its task runs.
func (c *Client) Configure(ctx context.Context, config Config) error {
	return c.do(ctx, http.MethodPut, "/config", config, nil)
}

// SetHosts replaces the names the machine's neighbours answer to.
func (c *Client) SetHosts(ctx context.Context, hosts []Host) error {
	return c.do(ctx, http.MethodPut, "/hosts", hosts, nil)
}

// Start runs the machine's task, again if it has run before.
func (c *Client) Start(ctx context.Context, process Process) (Status, error) {
	var status Status

	return status, c.do(ctx, http.MethodPost, "/process", process, &status)
}

// Status is what has become of the machine's task.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var status Status

	return status, c.do(ctx, http.MethodGet, "/process", nil, &status)
}

// Wait waits for the given run of the machine's task to end, and says what
// it ended with. A run that has already ended is answered at once.
func (c *Client) Wait(ctx context.Context, generation uint64) (Status, error) {
	var status Status

	path := "/process/wait?generation=" + strconv.FormatUint(generation, 10)

	return status, c.do(ctx, http.MethodGet, path, nil, &status)
}

// Signal sends a signal to the machine's task.
func (c *Client) Signal(ctx context.Context, signal int) error {
	return c.do(ctx, http.MethodPost, "/process/signal", Signal{Signal: signal}, nil)
}

// Stop ends the machine's task, giving it timeout to go on its own first,
// and says what it ended with.
func (c *Client) Stop(ctx context.Context, timeout time.Duration) (Status, error) {
	var status Status

	return status, c.do(ctx, http.MethodPost, "/process/stop", Stop{Timeout: timeout}, &status)
}

// Stats is what the machine's task is using.
func (c *Client) Stats(ctx context.Context) (Stats, error) {
	var stats Stats

	return stats, c.do(ctx, http.MethodGet, "/stats", nil, &stats)
}

// PowerOff turns the machine off, which ends its firecracker process.
func (c *Client) PowerOff(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/poweroff", nil, nil)
}

// Logs hands every line the task wrote after the one numbered after to emit,
// in order. With follow it keeps waiting for more until ctx is done, emit
// refuses a line, or the agent goes away.
func (c *Client) Logs(ctx context.Context, after uint64, follow bool, emit func(LogLine) error) error {
	query := url.Values{"after": {strconv.FormatUint(after, 10)}}
	if follow {
		query.Set("follow", "1")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, agentAddress+"/logs?"+query.Encode(), nil)
	if err != nil {
		return err
	}

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err := refusal(response); err != nil {
		return err
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), maxLogLine)

	for scanner.Scan() {
		var line LogLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return fmt.Errorf("a log line the agent sent is not one: %w", err)
		}

		if err := emit(line); err != nil {
			return err
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return scanner.Err()
}

// Exec starts a command inside the machine and hands back the connection its
// input and output travel on.
func (c *Client) Exec(ctx context.Context, exec Exec) (*ExecStream, error) {
	conn, header, err := c.upgrade(ctx, "/exec", UpgradeExec, exec)
	if err != nil {
		return nil, err
	}

	return &ExecStream{ID: header.Get(ExecIDHeader), conn: conn}, nil
}

// EndExec ends a command and everything it started, once nobody is attached
// to it any more.
func (c *Client) EndExec(ctx context.Context, id string, end EndExec) (Ended, error) {
	var ended Ended

	return ended, c.do(ctx, http.MethodPost, "/exec/"+url.PathEscape(id)+"/end", end, &ended)
}

// Dial connects to one of the task's ports, as its neighbours on its own
// network would. The connection carries raw bytes.
func (c *Client) Dial(ctx context.Context, port uint16) (net.Conn, error) {
	conn, _, err := c.upgrade(ctx, "/dial?port="+strconv.FormatUint(uint64(port), 10), UpgradeDial, nil)

	return conn, err
}

// do makes one request and reads its answer into answer, if there is one to
// read.
func (c *Client) do(ctx context.Context, method string, path string, body any, answer any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, agentAddress+path, payload)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err := refusal(response); err != nil {
		return err
	}

	if answer == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxAnswer))

		return nil
	}

	return json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(answer)
}

// upgrade asks for a connection to become something other than HTTP, and
// hands it back once the agent has agreed. Anything the agent sent after its
// agreement is kept, so nothing that follows is lost.
func (c *Client) upgrade(ctx context.Context, path string, protocol string, body any) (net.Conn, http.Header, error) {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}

		payload = encoded
	}

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}

	// the handshake belongs to ctx; what the connection carries afterwards
	// does not.
	interrupted := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })

	request, err := http.NewRequest(http.MethodPost, agentAddress+path, bytes.NewReader(payload))
	if err != nil {
		interrupted()
		conn.Close()

		return nil, nil, err
	}

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", protocol)
	request.Header.Set("Content-Type", "application/json")

	reader := bufio.NewReader(conn)

	response, err := handshake(conn, reader, request)
	if !interrupted() {
		// ctx ended during the handshake, and took the deadline with it.
		err = errors.Join(ctx.Err(), err)
	}

	if err != nil {
		conn.Close()

		return nil, nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	return &bufferedConn{Conn: conn, reader: reader}, response.Header, nil
}

func handshake(conn net.Conn, reader *bufio.Reader, request *http.Request) (*http.Response, error) {
	if err := request.Write(conn); err != nil {
		return nil, err
	}

	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode != http.StatusSwitchingProtocols {
		defer response.Body.Close()

		if err := refusal(response); err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("the agent answered %s rather than switching protocols", response.Status)
	}

	return response, nil
}

// refusal turns an answer that is not a success into the error it carries.
func refusal(response *http.Response) error {
	if response.StatusCode < http.StatusBadRequest {
		return nil
	}

	reason, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	err := fmt.Errorf("the agent refused: %s: %s", response.Status, bytes.TrimSpace(reason))

	if response.StatusCode == http.StatusConflict {
		return errors.Join(ErrNotRunning, err)
	}

	return err
}

// bufferedConn is a connection whose first bytes were already read while the
// handshake was.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

// CloseWrite passes a half-close on, so a peer that finished sending can say so
// while still reading.
func (c *bufferedConn) CloseWrite() error {
	if closer, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return closer.CloseWrite()
	}

	return c.Conn.Close()
}

// ExecStream is a command running inside a machine: reading takes its output,
// writing feeds its input, and resizing reshapes its terminal.
type ExecStream struct {
	// ID is what the agent calls the command, which is what ends it.
	ID string

	conn net.Conn

	writing sync.Mutex
	pending []byte

	exitCode int
	exited   bool

	closing sync.Once
	closed  error
}

// Read takes the command's output, both streams as one. It ends with io.EOF
// once the command has ended.
func (s *ExecStream) Read(p []byte) (int, error) {
	for len(s.pending) == 0 {
		if s.exited {
			return 0, io.EOF
		}

		frame, err := ReadFrame(s.conn)
		if err != nil {
			return 0, err
		}

		switch frame.Type {
		case FrameStdout, FrameStderr:
			s.pending = frame.Payload
		case FrameExit:
			code, err := ParseExit(frame.Payload)
			if err != nil {
				return 0, err
			}

			s.exitCode, s.exited = code, true
		}
	}

	n := copy(p, s.pending)
	s.pending = s.pending[n:]

	return n, nil
}

// Write feeds the command's input.
func (s *ExecStream) Write(p []byte) (int, error) {
	s.writing.Lock()
	defer s.writing.Unlock()

	if err := WriteFrame(s.conn, FrameStdin, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Resize tells the command's terminal how big it now is.
func (s *ExecStream) Resize(rows uint16, cols uint16) error {
	s.writing.Lock()
	defer s.writing.Unlock()

	return WriteFrame(s.conn, FrameResize, ResizePayload(rows, cols))
}

// ExitCode is what the command returned, once Read has said it ended.
func (s *ExecStream) ExitCode() (int, bool) {
	return s.exitCode, s.exited
}

// Close lets go of the connection. What was running carries on, which is
// what ending it is for.
func (s *ExecStream) Close() error {
	s.closing.Do(func() {
		s.closed = s.conn.Close()
	})

	return s.closed
}
