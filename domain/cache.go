package domain

import "context"

type Cache interface {
	Set(ctx context.Context, key string, value []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Purge(ctx context.Context, key string) error
}
