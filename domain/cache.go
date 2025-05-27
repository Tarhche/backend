package domain

type Cache interface {
	Set(key string, value []byte) error
	Get(key string) ([]byte, error)
	Purge(key string) error
}
