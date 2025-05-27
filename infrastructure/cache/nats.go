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

func NewNatsCache(nc *nats.Conn, bucket string) (*natsCache, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:         bucket,
		Description:    "This bucket is used for caching",
		Storage:        jetstream.FileStorage,
		History:        1,
		Replicas:       1,
		LimitMarkerTTL: 1 * time.Second, // If we omit LimitMarkerTTL, purge markers are retained forever.
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
