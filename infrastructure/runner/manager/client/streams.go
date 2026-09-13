package client

import (
	"context"
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

// websocketURL is the same endpoint spelled as a websocket one.
func websocketURL(endpoint string) string {
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		return "wss://" + strings.TrimPrefix(endpoint, "https://")
	case strings.HasPrefix(endpoint, "http://"):
		return "ws://" + strings.TrimPrefix(endpoint, "http://")
	}

	return endpoint
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
	endpoint := websocketURL(c.path(path, query))

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
