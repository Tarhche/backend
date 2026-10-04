package minio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/minio/minio-go/v7"

	"github.com/khanzadimahdi/testproject/domain"
)

// ErrNotConfigured is a bucket that was given no endpoint to be reached at.
var ErrNotConfigured = errors.New("the bucket has no endpoint configured")

// Lazy is a bucket that is reached the first time it is used rather than when
// it is made.
//
// New reaches the bucket at once, to make it when it is not there, so a service
// that holds one cannot start while the storage is away. A service that only
// needs it now and then — the workload's control plane, which deletes a
// snapshot's archive when the snapshot is deleted — would rather start and
// fail the one call: Lazy tries again on every call until the bucket answers,
// and keeps it from then on.
//
// An object that is not there is domain.ErrNotExists, whichever way the
// storage says so.
type Lazy struct {
	options Options

	lock    sync.Mutex
	storage *MinIO
}

func NewLazy(options Options) *Lazy {
	return &Lazy{options: options}
}

func (l *Lazy) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	storage, err := l.connect()
	if err != nil {
		return err
	}

	return storage.Store(ctx, objectName, reader, objectSize)
}

func (l *Lazy) Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error) {
	storage, err := l.connect()
	if err != nil {
		return nil, err
	}

	object, err := storage.Read(ctx, objectName)

	return object, missing(err)
}

func (l *Lazy) Delete(ctx context.Context, objectName string) error {
	storage, err := l.connect()
	if err != nil {
		return err
	}

	return missing(storage.Delete(ctx, objectName))
}

// connect reaches the bucket, once it can.
func (l *Lazy) connect() (*MinIO, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.storage != nil {
		return l.storage, nil
	}

	if len(l.options.Endpoint) == 0 {
		return nil, ErrNotConfigured
	}

	storage, err := New(l.options)
	if err != nil {
		return nil, err
	}

	l.storage = storage

	return storage, nil
}

// missing reads an object that is not there as the domain's own error.
func missing(err error) error {
	if err == nil {
		return nil
	}

	response := minio.ToErrorResponse(err)
	if response.Code == "NoSuchKey" || response.StatusCode == http.StatusNotFound {
		return errors.Join(domain.ErrNotExists, err)
	}

	return err
}
