package minio

import (
	"context"
	"io"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/file"
)

// Lazy is a MinIO store that reaches its bucket the first time it is used
// rather than when it is made.
//
// Making a MinIO store checks that its bucket is there, and makes the bucket
// when it is not, which needs S3 to answer. A service for which storing is one
// thing among many — an orchestrator, which stores nothing but the snapshots
// it is asked for — would otherwise not start at all while S3 is away or not
// configured yet, and the VMs it holds would go unattended with it. So the
// store is made when it is first needed, and made again on the next use when
// that failed.
type Lazy struct {
	options Options

	lock    sync.Mutex
	storage *MinIO
}

var _ file.Storage = &Lazy{}

// NewLazy is a store for the bucket options describe, which nothing has
// reached yet.
func NewLazy(options Options) *Lazy {
	return &Lazy{options: options}
}

func (l *Lazy) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	storage, err := l.get()
	if err != nil {
		return err
	}

	return storage.Store(ctx, objectName, reader, objectSize)
}

func (l *Lazy) Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error) {
	storage, err := l.get()
	if err != nil {
		return nil, err
	}

	return storage.Read(ctx, objectName)
}

func (l *Lazy) Delete(ctx context.Context, objectName string) error {
	storage, err := l.get()
	if err != nil {
		return err
	}

	return storage.Delete(ctx, objectName)
}

// get is the store, made now when it has not been yet.
func (l *Lazy) get() (*MinIO, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.storage != nil {
		return l.storage, nil
	}

	storage, err := New(l.options)
	if err != nil {
		return nil, err
	}

	l.storage = storage

	return storage, nil
}
