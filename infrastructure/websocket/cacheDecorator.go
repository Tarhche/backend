package websocket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
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

	mu sync.RWMutex

	// We need this to associate the reply with the original request's checksum key when it replies back.
	pending map[string]string // serverSideRequestID -> checksumKey
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

func NewCacheDecorator(ws ws, cache domain.Cache, subjects ...string) *CacheDecorator {
	s := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		s[subject] = struct{}{}
	}

	return &CacheDecorator{
		parent:   ws,
		cache:    cache,
		subjects: s,
		pending:  make(map[string]string),
	}
}

func (d *CacheDecorator) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if _, ok := d.subjects[subject]; !ok {
		return d.parent.Consume(ctx, subject, handler)
	}

	return d.parent.Consume(
		ctx,
		subject,
		domain.MessageHandlerFunc(func(payload []byte) error {
			checksum, requestID, err := payloadChecksum(payload)
			if err != nil {
				return handler.Handle(payload)
			}

			// if we have cached reply, return it immediately.
			log.Println("checking cache for checksum key:", checksum, "for request ID:", requestID)
			if cached, err := d.cache.Get(checksum); err == nil {
				log.Println("cache hit for checksum key:", checksum, "for request ID:", requestID)
				return d.parent.Reply(context.Background(), &domain.Reply{
					RequestID: requestID,
					Payload:   cached,
				})
			}

			d.mu.Lock()
			d.pending[requestID] = checksum
			d.mu.Unlock()

			return handler.Handle(payload)
		}),
	)
}

func (d *CacheDecorator) Reply(ctx context.Context, reply *domain.Reply) error {
	d.mu.RLock()
	checksum, ok := d.pending[reply.RequestID]
	d.mu.RUnlock()

	log.Println("pending checksum key:", checksum, "for request ID:", reply.RequestID, "exists:", ok)
	if ok {
		log.Println("caching reply with checksum key:", checksum)
		if err := d.cache.Set(checksum, reply.Payload); err != nil {
			log.Println("WS cache set error:", err)
		}

		d.mu.Lock()
		delete(d.pending, reply.RequestID)
		d.mu.Unlock()
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
