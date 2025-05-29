package cache

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type natsCache struct {
	kv jetstream.KeyValue
}

var _ domain.Cache = (*natsCache)(nil)

type params struct {
	ttl            time.Duration
	limitMarkerTTL time.Duration
	replicas       int
	compression    bool
}

type Option func(*params)

func WithTTL(ttl time.Duration) Option {
	return func(o *params) {
		o.ttl = ttl
	}
}

func WithLimitMarkerTTL(limitMarkerTTL time.Duration) Option {
	return func(o *params) {
		o.limitMarkerTTL = limitMarkerTTL
	}
}

func WithReplicas(replicas int) Option {
	return func(o *params) {
		o.replicas = replicas
	}
}

func WithCompression(compression bool) Option {
	return func(o *params) {
		o.compression = compression
	}
}

var defaultParams = params{
	ttl:            0,               // 0 means no TTL. If we omit TTL, keys do not expire.
	limitMarkerTTL: 1 * time.Second, // 0 means no limit marker TTL. If we omit LimitMarkerTTL, purge markers are retained forever.
	replicas:       1,
	compression:    false, // false means no compression
}

func NewNatsCache(nc *nats.Conn, bucket string, options ...Option) (*natsCache, error) {
	params := defaultParams
	for i := range options {
		options[i](&params)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:         bucket,
		Description:    "This bucket is used for caching",
		Storage:        jetstream.FileStorage,
		History:        1,
		MaxValueSize:   5 << 20, // 5MB
		Replicas:       params.replicas,
		TTL:            params.ttl,
		LimitMarkerTTL: params.limitMarkerTTL,
		Compression:    params.compression,
	})
	if err != nil {
		return nil, err
	}

	return &natsCache{kv: kv}, nil
}

func (c *natsCache) Set(key string, value []byte) error {
	_, err := c.kv.Put(context.Background(), key, value)

	return err
}

func (c *natsCache) Get(key string) ([]byte, error) {
	kv, err := c.kv.Get(context.Background(), key)
	if err == jetstream.ErrKeyNotFound {
		return nil, domain.ErrNotExists
	} else if err != nil {
		return nil, err
	}

	return kv.Value(), nil
}

func (c *natsCache) Purge(key string) error {
	return c.kv.Purge(context.Background(), key)
}
