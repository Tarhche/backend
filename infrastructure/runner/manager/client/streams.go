package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// writeWait bounds one write to the manager, so a stalled peer cannot hold a
// stream's writer forever.
const writeWait = 10 * time.Second

// AttachContainer opens a command inside a container and hands back the stream
// it runs on.
func (c *Client) AttachContainer(ctx context.Context, uuid string, command []string) (runnerManager.Attachment, error) {
	query := url.Values{}
	for _, argument := range command {
		query.Add("command", argument)
	}

	conn, err := c.dial(ctx, "/api/tasks/"+url.PathEscape(uuid)+"/attach", query)
	if err != nil {
		return nil, err
	}

	return &attachment{conn: conn}, nil
}

// FollowContainerLogs follows a container's output from a moment onward.
func (c *Client) FollowContainerLogs(ctx context.Context, uuid string, after time.Time) (runnerManager.LogStream, error) {
	query := url.Values{}
	if !after.IsZero() {
		query.Set("after", after.UTC().Format(time.RFC3339Nano))
	}

	conn, err := c.dial(ctx, "/api/tasks/"+url.PathEscape(uuid)+"/logs/stream", query)
	if err != nil {
		return nil, err
	}

	return &logStream{conn: conn, taskUUID: uuid}, nil
}

// WatchContainers follows what happens to the containers the runner holds.
func (c *Client) WatchContainers(ctx context.Context) (runnerManager.ContainerStream, error) {
	conn, err := c.dial(ctx, "/api/tasks/watch", nil)
	if err != nil {
		return nil, err
	}

	return &containerStream{conn: conn}, nil
}

// WatchStacks follows what happens to the stacks the runner holds.
func (c *Client) WatchStacks(ctx context.Context) (runnerManager.StackStream, error) {
	conn, err := c.dial(ctx, "/api/stacks/watch", nil)
	if err != nil {
		return nil, err
	}

	return &stackStream{conn: conn}, nil
}

// dial opens a websocket to the manager. A handshake refused with 404 is a
// container that is not there, which the layers above already know how to
// report.
func (c *Client) dial(ctx context.Context, path string, query url.Values) (*websocket.Conn, error) {
	endpoint := c.path(path, query)

	switch {
	case strings.HasPrefix(endpoint, "https://"):
		endpoint = "wss://" + strings.TrimPrefix(endpoint, "https://")
	case strings.HasPrefix(endpoint, "http://"):
		endpoint = "ws://" + strings.TrimPrefix(endpoint, "http://")
	}

	conn, response, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		if response != nil {
			defer response.Body.Close()

			if response.StatusCode == http.StatusNotFound {
				return nil, domain.ErrNotExists
			}
		}

		return nil, err
	}

	return conn, nil
}

// attachment carries a command's bytes over a websocket: binary frames are the
// command's own input and output, and a text frame is a control message, which
// today means a terminal that has been resized.
type attachment struct {
	conn *websocket.Conn

	// reader is whatever is left of the frame currently being read.
	reader io.Reader

	writeLock sync.Mutex
	shut      sync.Once
	shutErr   error
}

var _ runnerManager.Attachment = &attachment{}

func (a *attachment) Read(p []byte) (int, error) {
	for {
		if a.reader != nil {
			n, err := a.reader.Read(p)
			if err == io.EOF {
				a.reader = nil

				// an empty frame says nothing; wait for one that does rather
				// than reporting the end of the stream.
				if n == 0 {
					continue
				}

				return n, nil
			}

			return n, err
		}

		messageType, reader, err := a.conn.NextReader()
		if err != nil {
			return 0, io.EOF
		}

		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}

		a.reader = reader
	}
}

func (a *attachment) Write(p []byte) (int, error) {
	a.writeLock.Lock()
	defer a.writeLock.Unlock()

	_ = a.conn.SetWriteDeadline(time.Now().Add(writeWait))

	if err := a.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}

	return len(p), nil
}

// resize is the control message a terminal sends when its window changes.
type resize struct {
	Type string `json:"type"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

func (a *attachment) Resize(_ context.Context, rows uint, cols uint) error {
	payload, err := json.Marshal(resize{Type: "resize", Rows: rows, Cols: cols})
	if err != nil {
		return err
	}

	a.writeLock.Lock()
	defer a.writeLock.Unlock()

	_ = a.conn.SetWriteDeadline(time.Now().Add(writeWait))

	return a.conn.WriteMessage(websocket.TextMessage, payload)
}

func (a *attachment) Close() error {
	a.shut.Do(func() {
		a.shutErr = a.conn.Close()
	})

	return a.shutErr
}

// containerStream reads the containers' changes as the manager sends them.
type containerStream struct {
	conn *websocket.Conn

	shut    sync.Once
	shutErr error
}

var _ runnerManager.ContainerStream = &containerStream{}

func (s *containerStream) Next(ctx context.Context) (runnerManager.ContainerChange, error) {
	// closing the connection is what releases a reader parked here, so a
	// cancelled context has to reach it that way.
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()

	var payload changePayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return runnerManager.ContainerChange{}, io.EOF
	}

	return payload.toChange(), nil
}

func (s *containerStream) Close() error {
	s.shut.Do(func() {
		s.shutErr = s.conn.Close()
	})

	return s.shutErr
}

// stackStream reads the stacks' changes as the manager sends them.
type stackStream struct {
	conn *websocket.Conn

	shut    sync.Once
	shutErr error
}

var _ runnerManager.StackStream = &stackStream{}

func (s *stackStream) Next(ctx context.Context) (runnerManager.StackChange, error) {
	// closing the connection is what releases a reader parked here, so a
	// cancelled context has to reach it that way.
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()

	var payload stackChangePayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return runnerManager.StackChange{}, io.EOF
	}

	return payload.toChange(), nil
}

func (s *stackStream) Close() error {
	s.shut.Do(func() {
		s.shutErr = s.conn.Close()
	})

	return s.shutErr
}

// logStream reads a container's lines as the manager sends them.
type logStream struct {
	conn     *websocket.Conn
	taskUUID string

	shut    sync.Once
	shutErr error
}

var _ runnerManager.LogStream = &logStream{}

func (s *logStream) Next(ctx context.Context) (container.Log, error) {
	// closing the connection is what releases a reader parked here, so a
	// cancelled context has to reach it that way.
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()

	var payload logPayload
	if err := s.conn.ReadJSON(&payload); err != nil {
		return container.Log{}, io.EOF
	}

	return payload.toLog(s.taskUUID), nil
}

func (s *logStream) Close() error {
	s.shut.Do(func() {
		s.shutErr = s.conn.Close()
	})

	return s.shutErr
}
