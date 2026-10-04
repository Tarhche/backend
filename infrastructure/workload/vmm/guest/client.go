// Package guest is vmhost's side of the guest protocol (domain/workload/guest):
// a client for the agent inside one machine, which it reaches through the unix
// socket the machine's firecracker exposes for its vsock, with the
// "CONNECT 1024" handshake firecracker-go-sdk's vsock package speaks.
//
// It trusts nothing that comes back. A guest runs whatever the task put in it,
// so every answer is bounded in size and time, and an agent whose protocol
// version it does not know is refused rather than misread: a machine can
// outlive the vmhost that booted it, and be adopted by a newer one.
//
// What it asks is written once, in the protocol's own route constants, and
// only filled in here: the agent serves the same constants, so the two cannot
// drift apart without the protocol changing.
//
// The agent itself, which runs inside the machine, is in agent/. This client
// is PR #101's, which spoke to the same agent end to end, renamed for the
// workload.
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
	"strings"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/vsock"

	guestProtocol "github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
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

	// readyAttempt is how long one asking of an agent that may not be up yet
	// may take, and readyInterval how long is waited before asking again
	// while a machine boots.
	readyAttempt  = time.Second
	readyInterval = 50 * time.Millisecond

	// callTimeout bounds what the agent answers at once: being told what its
	// machine is, starting the task, signalling it, its stats, turning it
	// off, and agreeing to carry a command or a connection. Waiting for the
	// task to end and following its logs take as long as they take, and are
	// bounded by whoever asks.
	callTimeout = 30 * time.Second

	// the address requests are made to. It names nothing: every connection
	// goes to the one agent this client was made for.
	agentAddress = "http://agent"
)

// Connector makes clients for machines' agents.
type Connector struct{}

var _ vm.GuestConnector = Connector{}

// NewConnector makes clients for machines' agents.
func NewConnector() Connector {
	return Connector{}
}

// Connect is the client for the agent of the machine whose vsock is exposed at
// vsockPath. It connects to nothing until it is asked something.
func (Connector) Connect(vsockPath string) vm.GuestClient {
	return NewClient(vsockPath)
}

// Client speaks to the agent inside one machine, through the unix socket the
// machine's firecracker exposes for its vsock.
type Client struct {
	socketPath string
	http       *http.Client
}

var _ vm.GuestClient = (*Client)(nil)

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
	return vsock.DialContext(ctx, c.socketPath, guestProtocol.Port, vsock.WithDialTimeout(dialTimeout))
}

// Close lets go of the connections kept for the next request.
func (c *Client) Close() error {
	c.http.CloseIdleConnections()

	return nil
}

// Ready waits until the agent answers, which is when a machine has booted.
//
// An agent that answers in a version of the protocol this vmhost does not
// speak is refused at once, with guest.ErrUnknownVersion: it is up, and asking
// it again changes nothing.
func (c *Client) Ready(ctx context.Context) error {
	for {
		attempt, cancel := context.WithTimeout(ctx, readyAttempt)
		err := c.health(attempt)
		cancel()

		if err == nil || errors.Is(err, guestProtocol.ErrUnknownVersion) {
			return err
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(readyInterval):
		}
	}
}

// health asks the agent once whether it is up, and checks which version of
// the protocol it answers in before anything else it says: an agent that
// answered at all has said that much, whether or not it is ready.
func (c *Client) health(ctx context.Context) error {
	method, path := route(guestProtocol.RouteHealth)

	response, err := c.send(ctx, method, path, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if version := response.Header.Get(guestProtocol.VersionHeader); version != guestProtocol.ProtocolVersion {
		if len(version) == 0 {
			return fmt.Errorf("%w: it does not say which it speaks, and this vmhost speaks %q", guestProtocol.ErrUnknownVersion, guestProtocol.ProtocolVersion)
		}

		return fmt.Errorf("%w: it speaks %q, and this vmhost speaks %q", guestProtocol.ErrUnknownVersion, version, guestProtocol.ProtocolVersion)
	}

	if err := refusal(response); err != nil {
		return err
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxAnswer))

	return nil
}

// Configure tells a machine what it is, once, before its task runs.
func (c *Client) Configure(ctx context.Context, config guestProtocol.Config) error {
	return c.call(ctx, guestProtocol.RouteConfig, nil, config, nil)
}

// SetHosts replaces the names the machine's neighbours answer to.
func (c *Client) SetHosts(ctx context.Context, hosts []guestProtocol.Host) error {
	return c.call(ctx, guestProtocol.RouteHosts, nil, hosts, nil)
}

// Start runs the machine's task, again if it has run before.
func (c *Client) Start(ctx context.Context, process guestProtocol.Process) (guestProtocol.Status, error) {
	var status guestProtocol.Status

	return status, c.call(ctx, guestProtocol.RouteStart, nil, process, &status)
}

// Status is what has become of the machine's task.
func (c *Client) Status(ctx context.Context) (guestProtocol.Status, error) {
	var status guestProtocol.Status

	return status, c.call(ctx, guestProtocol.RouteStatus, nil, nil, &status)
}

// Wait waits for the given run of the machine's task to end, and says what
// it ended with. A run that has already ended is answered at once. It waits
// for as long as ctx lets it: a task may run for days.
func (c *Client) Wait(ctx context.Context, generation uint64) (guestProtocol.Status, error) {
	var status guestProtocol.Status

	query := url.Values{guestProtocol.QueryGeneration: {strconv.FormatUint(generation, 10)}}

	return status, c.do(ctx, guestProtocol.RouteWait, query, nil, &status)
}

// Signal sends a signal to the machine's task.
func (c *Client) Signal(ctx context.Context, signal int) error {
	return c.call(ctx, guestProtocol.RouteSignal, nil, guestProtocol.Signal{Signal: signal}, nil)
}

// Stop ends the machine's task, giving it timeout to go on its own first,
// and says what it ended with.
func (c *Client) Stop(ctx context.Context, timeout time.Duration) (guestProtocol.Status, error) {
	var status guestProtocol.Status

	// the agent answers once the task is gone, which takes as long as the task
	// is given, and no longer than ending it after that.
	ctx, cancel := context.WithTimeout(ctx, max(timeout, 0)+callTimeout)
	defer cancel()

	return status, c.do(ctx, guestProtocol.RouteStop, nil, guestProtocol.Stop{Timeout: timeout}, &status)
}

// Stats is what the machine's task is using.
func (c *Client) Stats(ctx context.Context) (guestProtocol.Stats, error) {
	var stats guestProtocol.Stats

	return stats, c.call(ctx, guestProtocol.RouteStats, nil, nil, &stats)
}

// PowerOff turns the machine off, which ends its firecracker process. The
// agent answers before it turns the machine off, so the answer says the
// machine is on its way down rather than that it is down.
func (c *Client) PowerOff(ctx context.Context) error {
	return c.call(ctx, guestProtocol.RoutePowerOff, nil, nil, nil)
}

// Logs hands every line the task wrote after the one numbered after to emit,
// in order. With follow it keeps waiting for more until ctx is done, emit
// refuses a line, or the agent goes away.
func (c *Client) Logs(ctx context.Context, after uint64, follow bool, emit func(guestProtocol.LogLine) error) error {
	query := url.Values{guestProtocol.QueryAfter: {strconv.FormatUint(after, 10)}}
	if follow {
		query.Set(guestProtocol.QueryFollow, "1")
	}

	method, path := route(guestProtocol.RouteLogs)

	response, err := c.send(ctx, method, path+"?"+query.Encode(), nil)
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
		var line guestProtocol.LogLine
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

// Exec starts a command inside the machine and hands back what the agent calls
// it, which is what ends it, and the connection its input and output travel on
// as frames (guest.ReadFrame). vmhost passes the connection on as it is, to
// whoever asked for the command.
func (c *Client) Exec(ctx context.Context, exec guestProtocol.Exec) (string, net.Conn, error) {
	conn, header, err := c.upgrade(ctx, guestProtocol.RouteExec, nil, guestProtocol.UpgradeExec, exec)
	if err != nil {
		return "", nil, err
	}

	id := header.Get(guestProtocol.ExecIDHeader)
	if len(id) == 0 {
		conn.Close()

		return "", nil, errors.New("the agent started a command without saying what it calls it, so it could never be ended")
	}

	return id, conn, nil
}

// EndExec ends a command and everything it started, once nobody is attached
// to it any more.
func (c *Client) EndExec(ctx context.Context, id string, end guestProtocol.EndExec) (guestProtocol.Ended, error) {
	var ended guestProtocol.Ended

	// the agent answers once the command is gone, which takes as long as it
	// is given twice over, and no longer than ending it after that.
	ctx, cancel := context.WithTimeout(ctx, max(end.Grace, 0)+max(end.KillGrace, 0)+callTimeout)
	defer cancel()

	method, path := route(guestProtocol.RouteEndExec)
	path = strings.Replace(path, "{id}", url.PathEscape(id), 1)

	return ended, c.request(ctx, method, path, end, &ended)
}

// Dial connects to one of the task's ports, as its neighbours on its own
// network would. The connection carries raw bytes.
func (c *Client) Dial(ctx context.Context, port uint16) (net.Conn, error) {
	query := url.Values{guestProtocol.QueryPort: {strconv.FormatUint(uint64(port), 10)}}

	conn, _, err := c.upgrade(ctx, guestProtocol.RouteDial, query, guestProtocol.UpgradeDial, nil)

	return conn, err
}

// call asks the agent something it answers at once, and bounds how long that
// may take even when ctx does not.
func (c *Client) call(ctx context.Context, r string, query url.Values, body any, answer any) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	return c.do(ctx, r, query, body, answer)
}

// do asks one of the agent's routes, and reads its answer into answer if there
// is one to read.
func (c *Client) do(ctx context.Context, r string, query url.Values, body any, answer any) error {
	method, path := route(r)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	return c.request(ctx, method, path, body, answer)
}

// request makes one request and reads its answer into answer, if there is one
// to read.
func (c *Client) request(ctx context.Context, method string, path string, body any, answer any) error {
	response, err := c.send(ctx, method, path, body)
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

	if err := json.NewDecoder(io.LimitReader(response.Body, maxAnswer)).Decode(answer); err != nil {
		return fmt.Errorf("the agent's answer is not what was asked for: %w", err)
	}

	return nil
}

// send makes one request, its body encoded as JSON, and hands back the answer
// for the caller to read and close.
func (c *Client) send(ctx context.Context, method string, path string, body any) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, agentAddress+path, payload)
	if err != nil {
		return nil, err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	return c.http.Do(request)
}

// upgrade asks for a connection to become something other than HTTP, and
// hands it back once the agent has agreed. Anything the agent sent after its
// agreement is kept, so nothing that follows is lost.
func (c *Client) upgrade(ctx context.Context, r string, query url.Values, protocol string, body any) (net.Conn, http.Header, error) {
	method, path := route(r)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}

		payload = encoded
	}

	// the handshake belongs to ctx, and is bounded besides; what the
	// connection carries afterwards belongs to neither.
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}

	interrupted := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })

	request, err := http.NewRequest(method, agentAddress+path, bytes.NewReader(payload))
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

// route splits one of the agent's routes, written as Go's ServeMux takes
// them, into the method and the path it is asked by.
func route(r string) (string, string) {
	method, path, _ := strings.Cut(r, " ")

	return method, path
}

// refusal turns an answer that is not a success into the error it carries. The
// agent refuses what only a running task can do with 409 Conflict, which reads
// as guest.ErrNotRunning.
func refusal(response *http.Response) error {
	if response.StatusCode < http.StatusBadRequest {
		return nil
	}

	reason, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	reason = bytes.TrimSpace(reason)

	if response.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: the agent refused: %s: %s", guestProtocol.ErrNotRunning, response.Status, reason)
	}

	return fmt.Errorf("the agent refused: %s: %s", response.Status, reason)
}

// bufferedConn is a connection whose first bytes were already read while the
// handshake was.
//
// It offers no CloseWrite, which PR #101's did. Firecracker's vsock carries
// no half-close: a host that stops writing to its end of a connection ends the
// whole connection, and with it whatever the agent was still to send back. So
// nothing that passes the connection on, splicing it to another, finds a way
// to half-close it, and has to close it whole once it is done with it. A
// command's input ends with guest.FrameCloseStdin instead.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
