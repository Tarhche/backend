// Package watch follows a set of things the runner holds and tells a client
// what changes about them, so that whoever is showing them does not have to
// ask over and over.
//
// Like the log stream, it reads them from where they are kept rather than from
// the nodes holding them, so a watch survives a container moving, a node going
// away, and the manager being asked by a replica that never held it.
package watch

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// interval is how often the store is asked what things look like now.
	interval = time.Second

	// writeWait bounds one write to the client.
	writeWait = 10 * time.Second
)

// the kinds of change a client is told about.
const (
	changed = "changed"
	deleted = "deleted"
)

// Watch is one set of things to follow: how to read them, what tells one from
// another, and the name they travel under.
type Watch[T any] struct {
	// Field is what one of these is called on the wire, so a client reads a
	// change in the terms it already has.
	Field string

	// Identify is the identity of one of them, which is what a change names.
	Identify func(*T) string

	// Poll reads them as they are now.
	Poll func(context.Context) ([]T, error)

	Logger *slog.Logger
}

// Handler serves a watch over a websocket.
func Handler[T any](watch Watch[T]) http.Handler {
	// the peer is another service rather than a browser, so there is no origin
	// to check.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(rw, r, nil)
		if err != nil {
			watch.Logger.ErrorContext(r.Context(), "failed to upgrade a watch", "error", err)

			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// the client says nothing on this stream, so a read is only ever how it
		// tells us it has gone.
		go func() {
			defer cancel()

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		watch.serve(ctx, conn)
	})
}

// serve tells the client what changed since the last look, over and over, until
// it goes away.
//
// The first look only takes note of what is there: a watch is opened alongside
// a listing the client already has, so telling it about things that have not
// changed would be repeating that listing back to it.
func (w Watch[T]) serve(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var seen map[string][sha256.Size]byte

	for {
		items, err := w.Poll(ctx)
		if err != nil {
			if ctx.Err() == nil {
				w.Logger.ErrorContext(ctx, "error on reading what a watch follows", "error", err, "watching", w.Field)
			}

			return
		}

		current := make(map[string][sha256.Size]byte, len(items))

		for i := range items {
			item := &items[i]
			uuid := w.Identify(item)

			digest, err := fingerprint(item)
			if err != nil {
				w.Logger.ErrorContext(ctx, "error on reading what a watch follows", "error", err, "uuid", uuid)

				return
			}

			current[uuid] = digest

			if seen == nil || seen[uuid] == digest {
				continue
			}

			if !w.send(ctx, conn, map[string]any{"kind": changed, "uuid": uuid, w.Field: item}) {
				return
			}
		}

		for uuid := range seen {
			if _, kept := current[uuid]; kept {
				continue
			}

			if !w.send(ctx, conn, map[string]any{"kind": deleted, "uuid": uuid}) {
				return
			}
		}

		seen = current

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// send reports whether the client took the change; a client that did not is one
// that is gone, and the watch ends with it.
func (w Watch[T]) send(ctx context.Context, conn *websocket.Conn, change map[string]any) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))

	if err := conn.WriteJSON(change); err != nil {
		if ctx.Err() == nil {
			w.Logger.WarnContext(ctx, "a watch ended", "error", err, "watching", w.Field)
		}

		return false
	}

	return true
}

// fingerprint is what one thing looks like now, so that a look which found
// nothing new sends nothing.
func fingerprint(item any) ([sha256.Size]byte, error) {
	payload, err := json.Marshal(item)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	return sha256.Sum256(payload), nil
}
