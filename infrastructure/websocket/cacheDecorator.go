package websocket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/khanzadimahdi/testproject/domain"
)

const (
	// pendingKeyPrefix namespaces requestID -> checksum entries in the shared
	// cache so they don't collide with checksum -> reply entries.
	pendingKeyPrefix = "pending."

	// cachedKeyPrefix namespaces checksum -> reply payload entries.
	cachedKeyPrefix = "cached."
)

// the interface that should be implemented by a decorator
type ws interface {
	domain.Consumer
	domain.Replyer
	http.Handler
	io.Closer
}

// CacheDecorator wraps a Consumer and a Replyer to add transparent
// payload-based caching for a specified set of subjects. On a cache hit the
// reply is returned immediately without dispatching a message to the queue.
// On a cache miss the request is forwarded normally and the reply payload is
// stored in the cache before being sent. Subjects not in the allowlist are
// consumed without caching.
type CacheDecorator struct {
	parent   ws
	cache    domain.Cache
	subjects map[string]struct{}
	logger   *slog.Logger
}

// Ensure CacheDecorator implements the ws interface
var _ ws = &CacheDecorator{}

// Ensure Websocket implements the domain.Consumer interface
var _ domain.Consumer = &Websocket{}

// Ensure Websocket implements the domain.Replyer interface
var _ domain.Replyer = &Websocket{}

// make sure the websocket implements the http.Handler interface
var _ http.Handler = &Websocket{}

// make sure the websocket implements the io.Closer interface
var _ io.Closer = &Websocket{}

func NewCacheDecorator(ws ws, cache domain.Cache, logger *slog.Logger, subjects ...string) *CacheDecorator {
	s := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		s[subject] = struct{}{}
	}

	return &CacheDecorator{
		parent:   ws,
		cache:    cache,
		subjects: s,
		logger:   logger,
	}
}

func (d *CacheDecorator) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if _, ok := d.subjects[subject]; !ok {
		return d.parent.Consume(ctx, subject, handler)
	}

	return d.parent.Consume(
		ctx,
		subject,
		domain.MessageHandlerFunc(func(ctx context.Context, payload []byte) error {
			checksum, requestID, err := payloadChecksum(payload)
			if err != nil {
				return handler.Handle(ctx, payload)
			}

			// if we have cached reply, return it immediately.
			cachedKey := cachedKeyPrefix + checksum
			d.logger.Info("checking cache for checksum key", "cachedKey", cachedKey, "requestID", requestID)
			if cached, err := d.cache.Get(ctx, cachedKey); err == nil {
				d.logger.Info("cache hit for checksum key", "cachedKey", cachedKey, "requestID", requestID)
				return d.parent.Reply(ctx, &domain.Reply{
					RequestID: requestID,
					Payload:   cached,
				})
			}

			if err := d.cache.Set(ctx, pendingKeyPrefix+requestID, []byte(checksum)); err != nil {
				d.logger.Error("WS pending set error", "error", err)
			}

			return handler.Handle(ctx, payload)
		}),
	)
}

func (d *CacheDecorator) Reply(ctx context.Context, reply *domain.Reply) error {
	pendingKey := pendingKeyPrefix + reply.RequestID
	checksum, err := d.cache.Get(ctx, pendingKey)

	d.logger.Info("pending checksum key lookup", "checksum", string(checksum), "requestID", reply.RequestID, "exists", err == nil)
	if err == nil {
		cachedKey := cachedKeyPrefix + string(checksum)
		d.logger.Info("caching reply with checksum key", "cachedKey", cachedKey)
		if err := d.cache.Set(ctx, cachedKey, reply.Payload); err != nil {
			d.logger.Error("WS cache set error", "error", err)
		}

		if err := d.cache.Purge(ctx, pendingKey); err != nil {
			d.logger.Error("WS pending purge error", "error", err)
		}
	}

	return d.parent.Reply(ctx, reply)
}

func (d *CacheDecorator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.parent.ServeHTTP(w, r)
}

func (d *CacheDecorator) Close() error {
	return d.parent.Close()
}

// payloadChecksum strips the injected server-side "id" field from the JSON payload
// and returns a hex digest of the remaining fields as a stable cache key,
// along with the request ID itself.
func payloadChecksum(payload []byte) (checksum, requestID string, err error) {
	var m map[string]any
	if err = json.Unmarshal(payload, &m); err != nil {
		return "", "", err
	}

	id, ok := m["id"].(string)
	if !ok {
		return "", "", errors.New("payload missing string id field")
	}

	delete(m, "id")

	canonical, err := json.Marshal(m)
	if err != nil {
		return "", "", err
	}

	h := sha256.Sum256(canonical)

	return hex.EncodeToString(h[:]), id, nil
}
