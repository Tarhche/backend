package minio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/minio/minio-go/v7"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
)

// ErrNotConfigured is a bucket that was given no endpoint to be reached at.
var ErrNotConfigured = errors.New("the bucket has no endpoint configured")

// Lazy is a MinIO store that reaches its bucket the first time it is used
// rather than when it is made.
//
// Making a MinIO store checks that its bucket is there, and makes the bucket
// when it is not, which needs S3 to answer. A service for which storing is one
// thing among many would otherwise not start at all while S3 is away or not
// configured yet: an orchestrator, which stores nothing but the snapshots it is
// asked for, and would leave the VMs it holds unattended, or the control plane,
// which deletes a snapshot's archive when the snapshot is deleted. So the store
// is made when it is first needed, made again on the next use when that failed,
// and kept from then on.
//
// An object that is not there is domain.ErrNotExists, whichever way the
// storage says so.
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

// connect is the store, made now when it has not been yet.
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
