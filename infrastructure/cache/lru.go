package cache

import (
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/khanzadimahdi/testproject/domain"
)

const DefaultLRUSize = 1024

// InMemoryLRU is a fixed-size in-memory LRU cache that implements domain.Cache.
type InMemoryLRU struct {
	cache *lru.Cache[string, []byte]
}

var _ domain.Cache = (*InMemoryLRU)(nil)

func NewInMemoryLRU(size int) (*InMemoryLRU, error) {
	c, err := lru.New[string, []byte](size)
	if err != nil {
		return nil, err
	}

	return &InMemoryLRU{cache: c}, nil
}

func (l *InMemoryLRU) Set(key string, value []byte) error {
	l.cache.Add(key, value)

	return nil
}

func (l *InMemoryLRU) Get(key string) ([]byte, error) {
	value, ok := l.cache.Get(key)
	if !ok {
		return nil, domain.ErrNotExists
	}

	return value, nil
}

func (l *InMemoryLRU) Purge(key string) error {
	l.cache.Remove(key)

	return nil
}
