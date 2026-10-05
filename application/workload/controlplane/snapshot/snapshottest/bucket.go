// Package snapshottest stands in for the snapshots bucket in the snapshot use
// cases' tests.
package snapshottest

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// Bucket keeps archives in memory, under their object names.
type Bucket struct {
	lock    sync.Mutex
	objects map[string][]byte
	deleted []string

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ snapshot.Store = &Bucket{}

func NewBucket(objectNames ...string) *Bucket {
	b := &Bucket{objects: make(map[string][]byte)}
	for _, name := range objectNames {
		b.objects[name] = []byte("archive")
	}

	return b
}

func (b *Bucket) Store(_ context.Context, objectName string, reader io.Reader, _ int64) error {
	if b.Fail != nil {
		return b.Fail
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.objects[objectName] = content

	return nil
}

func (b *Bucket) Read(_ context.Context, objectName string) (io.ReadSeekCloser, error) {
	if b.Fail != nil {
		return nil, b.Fail
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	content, ok := b.objects[objectName]
	if !ok {
		return nil, domain.ErrNotExists
	}

	return nopCloser{bytes.NewReader(content)}, nil
}

// Delete takes an object away; one that is not there is domain.ErrNotExists, as
// the MinIO adapter reports it.
func (b *Bucket) Delete(_ context.Context, objectName string) error {
	if b.Fail != nil {
		return b.Fail
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.deleted = append(b.deleted, objectName)

	if _, ok := b.objects[objectName]; !ok {
		return domain.ErrNotExists
	}

	delete(b.objects, objectName)

	return nil
}

// Has reports whether an object is kept.
func (b *Bucket) Has(objectName string) bool {
	b.lock.Lock()
	defer b.lock.Unlock()

	_, ok := b.objects[objectName]

	return ok
}

// Deleted is every object name Delete was asked for.
func (b *Bucket) Deleted() []string {
	b.lock.Lock()
	defer b.lock.Unlock()

	return append([]string(nil), b.deleted...)
}

type nopCloser struct {
	*bytes.Reader
}

func (nopCloser) Close() error { return nil }
