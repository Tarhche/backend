package microvm

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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// vmhostAddress is the address every request is made to. It names nothing:
	// every connection goes to the one socket the client was made for.
	vmhostAddress = "http://vmhost"

	// maxAnswer bounds what is read of an answer that is not a stream. A
	// node's VMs listed whole are the largest of them, at a few kilobytes a
	// VM, so this is room for far more VMs than one host can hold.
	maxAnswer = 4 << 20

	// maxRefusal is how much of a refusal is read to say why.
	maxRefusal = 64 << 10

	// maxLogLine bounds one line of a VM's output as it travels. The agent cuts
	// a task's line at 64 KiB, and written as JSON a line can grow to several
	// times that.
	maxLogLine = 1 << 20

	// requestTimeout bounds a request that is answered rather than streamed.
	// Booting a VM is the slowest of them, and stopping one gives its task
	// vm.DefaultStopTimeout before anything else happens.
	requestTimeout = 2 * time.Minute

	// prepareTimeout bounds making an image into a disk. The first time an
	// image is asked for it is pulled from its registry, which takes as long
	// as the image is large; this only keeps a vmhost that stopped answering
	// from holding a task's start forever.
	prepareTimeout = 30 * time.Minute

	// handshakeTimeout bounds how long vmhost may take to start answering a
	// stream: a VM's output, a command's terminal, a connection to a task's
	// port. What a stream carries once it has started is not bounded.
	handshakeTimeout = 30 * time.Second

	// idleTimeout is how long a connection is kept for the next request.
	idleTimeout = 90 * time.Second

	// maxIdleConnections is how many of those are kept. The node's heartbeats
	// ask several times a second, and each log that is followed holds one of
	// its own for as long as it is followed.
	maxIdleConnections = 8
)

// Client is vmhost's API (domain/workload/vm), reached over its unix socket.
// What vmhost refuses comes back as the error vmhost had, so
// errors.Is(err, vm.ErrCapacity) holds here as it did there, and a vmhost that
// cannot be reached at all is vm.ErrUnavailable.
//
// Every method asks exactly one route, named by its constant in the vm
// package, so that what is asked can be read off the contract and nowhere
// else.
type Client struct {
	socketPath string
	http       *http.Client
	dialer     net.Dialer
}

// NewClient builds a client for the vmhost at endpoint, a unix socket written
// unix:///path.
func NewClient(endpoint string) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "unix" || len(parsed.Host) > 0 || len(parsed.Path) == 0 {
		return nil, fmt.Errorf("%q is not a vmhost: it is reached at unix:///path", endpoint)
	}

	c := &Client{socketPath: parsed.Path}

	c.http = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _ string, _ string) (net.Conn, error) {
				return c.dial(ctx)
			},
			IdleConnTimeout:     idleTimeout,
			MaxIdleConnsPerHost: maxIdleConnections,

			// a socket on the same host gains nothing from compression, and an
			// answer decompressed on the way in is no longer the size vmhost
			// said it was.
			DisableCompression: true,
		},

		// vmhost has nowhere to send anybody: a redirect would be an answer
		// it never meant, so it is taken as the answer it is.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return c, nil
}

// dial opens one connection to vmhost.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	return c.dialer.DialContext(ctx, "unix", c.socketPath)
}

// Close lets go of the connections kept for the next request.
func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

// Info is what vmhost says about itself: whether it can run VMs, what they can
// do, and how much room it has left for them.
func (c *Client) Info(ctx context.Context) (vm.Info, error) {
	var info vm.Info
	if err := c.call(ctx, vm.RouteInfo, vm.PathInfo, nil, &info); err != nil {
		return vm.Info{}, err
	}

	return info, nil
}

// PrepareImage makes an image ready to boot, pulling and converting it if
// vmhost does not hold it yet.
func (c *Client) PrepareImage(ctx context.Context, image string) (vm.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, prepareTimeout)
	defer cancel()

	var prepared vm.Image
	if err := c.send(ctx, vm.RoutePrepareImage, vm.PathPrepareImage, vm.PrepareImage{Image: image}, &prepared); err != nil {
		return vm.Image{}, err
	}

	return prepared, nil
}

// Images is every image vmhost holds.
func (c *Client) Images(ctx context.Context) ([]vm.Image, error) {
	var images list[vm.Image]
	if err := c.call(ctx, vm.RouteImages, vm.PathImages, nil, &images); err != nil {
		return nil, err
	}

	return images, nil
}

// DeleteImage lets go of one image's disk.
func (c *Client) DeleteImage(ctx context.Context, digest string) error {
	if len(digest) == 0 {
		return fmt.Errorf("%w: an image is deleted by its digest", vm.ErrInvalid)
	}

	return c.call(ctx, vm.RouteDeleteImage, vm.PathImage(digest), nil, nil)
}

// EnsureNetwork makes a network, if it is not there already, and says what it
// is. Whether a network routes out is settled when it is made.
func (c *Client) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	if err := checkNetwork(name); err != nil {
		return vm.Network{}, err
	}

	var made vm.Network
	if err := c.call(ctx, vm.RouteEnsureNetwork, vm.PathNetwork(name), vm.NetworkSpec{Masquerade: masquerade}, &made); err != nil {
		return vm.Network{}, err
	}

	return made, nil
}

// RemoveNetwork takes a network away. vmhost refuses it with
// vm.ErrNetworkInUse while anything is plugged into it.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	if err := checkNetwork(name); err != nil {
		return err
	}

	return c.call(ctx, vm.RouteRemoveNetwork, vm.PathNetwork(name), nil, nil)
}

// Create makes a VM, booting nothing, and says what vmhost calls it.
func (c *Client) Create(ctx context.Context, spec vm.Spec) (string, error) {
	var created vm.Created
	if err := c.call(ctx, vm.RouteCreateVM, vm.PathVMs, spec, &created); err != nil {
		return "", err
	}

	// the ID is what every later request names the VM by, and what it is
	// written into a path with, so one that cannot be a VM's is not taken.
	if !vm.IsID(created.ID) {
		return "", fmt.Errorf("vmhost made a vm and called it %q, which cannot name one", created.ID)
	}

	return created.ID, nil
}

// VMs is every VM carrying every label filter given, each key=value.
func (c *Client) VMs(ctx context.Context, labels ...string) ([]vm.VM, error) {
	path := vm.PathVMs
	if len(labels) > 0 {
		path += "?" + url.Values{vm.QueryLabel: labels}.Encode()
	}

	var vms list[vm.VM]
	if err := c.call(ctx, vm.RouteVMs, path, nil, &vms); err != nil {
		return nil, err
	}

	return vms, nil
}

// VM is one VM, or vm.ErrNotFound.
func (c *Client) VM(ctx context.Context, id string) (vm.VM, error) {
	if err := checkID(id); err != nil {
		return vm.VM{}, err
	}

	var found vm.VM
	if err := c.call(ctx, vm.RouteVM, vm.PathVM(id), nil, &found); err != nil {
		return vm.VM{}, err
	}

	return found, nil
}

// Start boots a VM and starts its task.
func (c *Client) Start(ctx context.Context, id string) error {
	return c.act(ctx, vm.RouteStartVM, id, "start", nil)
}

// Stop asks a VM's task to end, gives it timeout to do so, and then ends it.
func (c *Client) Stop(ctx context.Context, id string, timeout time.Duration) error {
	return c.act(ctx, vm.RouteStopVM, id, "stop", url.Values{vm.QueryTimeout: {timeout.String()}})
}

// Restart stops a VM's task and starts it again, on the disks it had.
func (c *Client) Restart(ctx context.Context, id string) error {
	return c.act(ctx, vm.RouteRestartVM, id, "restart", nil)
}

// Kill ends a VM's task at once.
func (c *Client) Kill(ctx context.Context, id string) error {
	return c.act(ctx, vm.RouteKillVM, id, "kill", nil)
}

// Delete takes a VM away, with everything kept for it.
func (c *Client) Delete(ctx context.Context, id string) error {
	if err := checkID(id); err != nil {
		return err
	}

	return c.call(ctx, vm.RouteDeleteVM, vm.PathVM(id), nil, nil)
}

// act asks for one thing to be done to a VM, which vmhost answers with nothing
// but whether it was done.
func (c *Client) act(ctx context.Context, route string, id string, action string, query url.Values) error {
	if err := checkID(id); err != nil {
		return err
	}

	path := vm.PathVMAction(id, action)
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	return c.call(ctx, route, path, nil, nil)
}

// Logs hands a VM's output to emit, after the line numbered after, or from
// since; with follow it keeps waiting for more until ctx is done. It returns
// what emit returned when emit refuses a line.
func (c *Client) Logs(ctx context.Context, id string, after uint64, since time.Time, follow bool, emit func(vm.LogLine) error) error {
	if err := checkID(id); err != nil {
		return err
	}

	query := url.Values{}
	if after > 0 {
		query.Set(vm.QueryAfter, strconv.FormatUint(after, 10))
	}

	if !since.IsZero() {
		query.Set(vm.QuerySince, since.UTC().Format(time.RFC3339Nano))
	}

	if follow {
		query.Set(vm.QueryFollow, "1")
	}

	path := vm.PathVMAction(id, "logs")
	if len(query) > 0 {
		path += "?" + query.Encode()
	}

	streaming, stop := context.WithCancel(ctx)
	defer stop()

	request, err := http.NewRequestWithContext(streaming, methodOf(vm.RouteVMLogs), vmhostAddress+path, nil)
	if err != nil {
		return err
	}

	carry(ctx, request.Header)

	// how long vmhost takes to start answering is bounded; how long the answer
	// then goes on for is not, since following a VM's output is meant to go on
	// for as long as the VM writes any.
	late := time.AfterFunc(handshakeTimeout, stop)

	response, err := c.http.Do(request)
	if !late.Stop() {
		if err == nil {
			response.Body.Close()
		}

		return fmt.Errorf("%w: it did not start answering for the output of vm %s within %s", vm.ErrUnavailable, id, handshakeTimeout)
	}

	if err != nil {
		return c.unreachable(err)
	}
	defer response.Body.Close()

	if err := refusal(response); err != nil {
		return err
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLogLine)

	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}

		var line vm.LogLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return fmt.Errorf("a line of output vmhost sent is not one: %w", err)
		}

		if err := emit(line); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return scanner.Err()
}

// Stats is what a VM uses right now.
func (c *Client) Stats(ctx context.Context, id string) (vm.Stats, error) {
	if err := checkID(id); err != nil {
		return vm.Stats{}, err
	}

	var stats vm.Stats
	if err := c.call(ctx, vm.RouteVMStats, vm.PathVMAction(id, "stats"), nil, &stats); err != nil {
		return vm.Stats{}, err
	}

	return stats, nil
}

// Exec starts a command inside a VM and hands back its ID and the connection
// its input and output travel on, as guest frames.
func (c *Client) Exec(ctx context.Context, id string, exec guest.Exec) (string, net.Conn, error) {
	if err := checkID(id); err != nil {
		return "", nil, err
	}

	conn, header, err := c.upgrade(ctx, vm.RouteExec, vm.PathVMAction(id, "exec"), vm.UpgradeExec, exec)
	if err != nil {
		return "", nil, err
	}

	// a command is ended by what vmhost calls it, once nobody is attached to
	// it any more. One that was started without a name could never be ended,
	// so it is not taken.
	execID := header.Get(vm.ExecIDHeader)
	if len(execID) == 0 {
		conn.Close()

		return "", nil, fmt.Errorf("vmhost started a command in vm %s without saying what it calls it", id)
	}

	return execID, conn, nil
}

// EndExec ends a command and everything it started, once nobody is attached
// to it any more.
func (c *Client) EndExec(ctx context.Context, id string, exec string, end guest.EndExec) (guest.Ended, error) {
	if err := checkID(id); err != nil {
		return guest.Ended{}, err
	}

	if len(exec) == 0 {
		return guest.Ended{}, fmt.Errorf("%w: a command is ended by what vmhost calls it", vm.ErrInvalid)
	}

	var ended guest.Ended
	if err := c.call(ctx, vm.RouteEndExec, vm.PathEndExec(id, exec), end, &ended); err != nil {
		return guest.Ended{}, err
	}

	return ended, nil
}

// Dial connects to one of a VM's task's ports, through vmhost and its agent.
// The connection carries raw bytes, as a connection to the port itself would.
func (c *Client) Dial(ctx context.Context, id string, port uint16) (net.Conn, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}

	if port == 0 {
		return nil, fmt.Errorf("%w: 0 is not a port", vm.ErrInvalid)
	}

	path := vm.PathVMAction(id, "dial") + "?" + url.Values{vm.QueryPort: {strconv.FormatUint(uint64(port), 10)}}.Encode()

	conn, _, err := c.upgrade(ctx, vm.RouteDial, path, vm.UpgradeDial, nil)

	return conn, err
}

// call makes one request that is answered rather than streamed, bounded by
// requestTimeout.
func (c *Client) call(ctx context.Context, route string, path string, body any, answer any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	return c.send(ctx, route, path, body, answer)
}

// send makes one request of a route and reads its answer into answer, when
// one is expected. A refusal is turned back into the error vmhost had.
func (c *Client) send(ctx context.Context, route string, path string, body any, answer any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, methodOf(route), vmhostAddress+path, payload)
	if err != nil {
		return err
	}

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	carry(ctx, request.Header)

	response, err := c.http.Do(request)
	if err != nil {
		return c.unreachable(err)
	}
	defer response.Body.Close()

	if err := refusal(response); err != nil {
		return err
	}

	// read even when nothing is expected, so the connection can be used again.
	content, err := io.ReadAll(io.LimitReader(response.Body, maxAnswer+1))
	if err != nil {
		return err
	}

	if len(content) > maxAnswer {
		return fmt.Errorf("vmhost answered %s with more than %d bytes", route, maxAnswer)
	}

	if answer == nil {
		return nil
	}

	if len(bytes.TrimSpace(content)) == 0 {
		return fmt.Errorf("vmhost answered %s with nothing", route)
	}

	return json.Unmarshal(content, answer)
}

// upgrade asks for a connection to become a stream, and hands it back once
// vmhost has agreed. Whatever vmhost sent after agreeing is kept, so nothing
// that follows the handshake is lost.
func (c *Client) upgrade(ctx context.Context, route string, path string, protocol string, body any) (net.Conn, http.Header, error) {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}

		payload = encoded
	}

	handshaking, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(handshaking, methodOf(route), vmhostAddress+path, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", protocol)

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	carry(ctx, request.Header)

	conn, err := c.dial(handshaking)
	if err != nil {
		return nil, nil, c.unreachable(err)
	}

	// the handshake belongs to the context; what the connection carries
	// afterwards does not, so the context only ever cuts the handshake short.
	interrupted := context.AfterFunc(handshaking, func() { _ = conn.SetDeadline(time.Now()) })

	reader := bufio.NewReader(conn)

	response, err := handshake(conn, reader, request, protocol)
	if !interrupted() {
		// the context ended during the handshake, and took the deadline with
		// it: the connection is of no use any more, whatever was read.
		err = errors.Join(handshaking.Err(), err)
	}

	if err != nil {
		conn.Close()

		return nil, nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	return &bufferedConn{Conn: conn, reader: reader}, response.Header, nil
}

// handshake writes an upgrade request and reads vmhost's answer to it.
func handshake(conn net.Conn, reader *bufio.Reader, request *http.Request, protocol string) (*http.Response, error) {
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

		return nil, fmt.Errorf("vmhost answered %s rather than switching to %s", response.Status, protocol)
	}

	if switched := response.Header.Get("Upgrade"); !strings.EqualFold(switched, protocol) {
		return nil, fmt.Errorf("vmhost switched to %q rather than to %s", switched, protocol)
	}

	return response, nil
}

// unreachable is a request that never got an answer. Unless whoever asked gave
// up on it, vmhost is not there to run anything, which is what
// vm.ErrUnavailable says.
func (c *Client) unreachable(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	return fmt.Errorf("%w: it cannot be reached at %s: %w", vm.ErrUnavailable, c.socketPath, err)
}

// refusal turns an answer that is not a success into the error vmhost had.
// An answer that is not one of vmhost's errors at all — the router's own
// "404 page not found", from a vmhost too old to know a route — says so
// rather than being taken for one of them: a VM that is not there and a route
// that is not there are different things to be told.
func refusal(response *http.Response) error {
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	content, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusal))

	var refused vm.ErrorResponse
	if err := json.Unmarshal(content, &refused); err == nil && len(refused.Code) > 0 {
		return refused.Err()
	}

	return fmt.Errorf("vmhost answered %s: %s", response.Status, bytes.TrimSpace(content))
}

// carry puts the trace a request belongs to into its headers, so a VM made
// for a task shows as one trace across the orchestrator and vmhost.
func carry(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

// methodOf is the method a route is asked with: vm writes its routes as Go's
// ServeMux takes them, "METHOD /path".
func methodOf(route string) string {
	method, _, _ := strings.Cut(route, " ")

	return method
}

// checkID refuses what cannot name a VM before it is written into a path. It
// cannot name anything vmhost holds either, which is what it is said as.
func checkID(id string) error {
	if !vm.IsID(id) {
		return fmt.Errorf("%w: %q is not what vmhost calls a vm", vm.ErrNotFound, id)
	}

	return nil
}

// checkNetwork refuses what cannot name a network before it is written into a
// path.
func checkNetwork(name string) error {
	if !vm.IsNetworkName(name) {
		return fmt.Errorf("%w: %q cannot name a network", vm.ErrInvalid, name)
	}

	return nil
}

// list is a listing vmhost answers. The contract names the shape of every
// answer but the two listings, so one is read whether it comes as the list
// itself, as the docker engine answers its listings, or as an object holding
// it under one name, as the rest of the workload answers them
// ({"items": […]}).
type list[T any] []T

func (l *list[T]) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	if len(trimmed) > 0 && trimmed[0] == '{' {
		var wrapped map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &wrapped); err != nil {
			return err
		}

		if len(wrapped) != 1 {
			return fmt.Errorf("a listing is a list, or an object holding one, not an object of %d fields", len(wrapped))
		}

		for _, inner := range wrapped {
			trimmed = inner
		}
	}

	var items []T
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return err
	}

	*l = items

	return nil
}

// bufferedConn is a connection whose first bytes may have been read along
// with the handshake.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

// CloseWrite passes a half-close on, so a peer that has finished sending can
// say so while it still reads what comes back.
func (c *bufferedConn) CloseWrite() error {
	if closer, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return closer.CloseWrite()
	}

	return c.Conn.Close()
}
