package websocket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// CacheDecorator wraps a Consumer and a Replyer to add transparent
// payload-based caching for a specified set of subjects. On a cache hit the
// reply is returned immediately without dispatching a message to the queue.
// On a cache miss the request is forwarded normally and the reply payload is
// stored in the cache before being sent. Subjects not in the allowlist are
// consumed without caching.
type CacheDecorator struct {
	consumer domain.Consumer
	replyer  domain.Replyer
	cache    domain.Cache
	subjects map[string]struct{}

	mu      sync.Mutex
	pending map[string]string // serverSideRequestID -> checksumKey
}

var _ domain.Consumer = (*CacheDecorator)(nil)
var _ domain.Replyer = (*CacheDecorator)(nil)

func NewCacheDecorator(consumer domain.Consumer, replyer domain.Replyer, cache domain.Cache, subjects ...string) *CacheDecorator {
	s := make(map[string]struct{}, len(subjects))
	for _, subject := range subjects {
		s[subject] = struct{}{}
	}

	return &CacheDecorator{
		consumer: consumer,
		replyer:  replyer,
		cache:    cache,
		subjects: s,
		pending:  make(map[string]string),
	}
}

func (d *CacheDecorator) Consume(ctx context.Context, subject string, handler domain.MessageHandler) error {
	if _, ok := d.subjects[subject]; !ok {
		return d.consumer.Consume(ctx, subject, handler)
	}

	return d.consumer.Consume(ctx, subject, domain.MessageHandlerFunc(func(payload []byte) error {
		checksum, requestID, err := payloadChecksum(payload)
		if err != nil {
			return handler.Handle(payload)
		}

		if cached, err := d.cache.Get(checksum); err == nil {
			return d.replyer.Reply(context.Background(), &domain.Reply{
				RequestID: requestID,
				Payload:   cached,
			})
		}

		d.mu.Lock()
		d.pending[requestID] = checksum
		d.mu.Unlock()

		return handler.Handle(payload)
	}))
}

func (d *CacheDecorator) Reply(ctx context.Context, reply *domain.Reply) error {
	d.mu.Lock()
	checksum, ok := d.pending[reply.RequestID]
	if ok {
		delete(d.pending, reply.RequestID)
	}
	d.mu.Unlock()

	if ok {
		_ = d.cache.Set(checksum, reply.Payload)
	}

	return d.replyer.Reply(ctx, reply)
}

// payloadChecksum strips the injected server-side "id" field from the JSON payload
// and returns a SHA-256 hex digest of the remaining fields as a stable cache key,
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
