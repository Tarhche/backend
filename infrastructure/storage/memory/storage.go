// Package memory is object storage kept in memory, for the tests: what is
// stored can be read back, and what was never stored is not there.
package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
)

// Storage keeps objects in a map. Err, when it is set, is what every call
// answers instead, which is how S3 being away looks.
type Storage struct {
	lock    sync.Mutex
	objects map[string][]byte

	Err error
}

var _ file.Storage = &Storage{}

func New() *Storage {
	return &Storage{objects: make(map[string][]byte)}
}

// Store reads the object to its end, as an upload does, and keeps it only when
// all of it arrived.
func (s *Storage) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	if err := s.failure(); err != nil {
		return err
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	if objectSize >= 0 && int64(len(content)) != objectSize {
		return fmt.Errorf("%d bytes were said to be stored and %d arrived", objectSize, len(content))
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.objects[objectName] = content

	return nil
}

func (s *Storage) Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error) {
	if err := s.failure(); err != nil {
		return nil, err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	content, ok := s.objects[objectName]
	if !ok {
		return nil, fmt.Errorf("%w: no object %q", domain.ErrNotExists, objectName)
	}

	return readSeekCloser{bytes.NewReader(content)}, nil
}

// Delete removes an object. One that is not there is already removed.
func (s *Storage) Delete(ctx context.Context, objectName string) error {
	if err := s.failure(); err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.objects, objectName)

	return nil
}

// Object is what is stored under a name, and whether anything is.
func (s *Storage) Object(objectName string) ([]byte, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	content, ok := s.objects[objectName]

	return slices.Clone(content), ok
}

// Put stores an object as it is, for a test to read.
func (s *Storage) Put(objectName string, content []byte) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.objects[objectName] = slices.Clone(content)
}

// Names is every object stored.
func (s *Storage) Names() []string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Sorted(maps.Keys(s.objects))
}

func (s *Storage) failure() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.Err
}

type readSeekCloser struct {
	*bytes.Reader
}

func (readSeekCloser) Close() error {
	return nil
}
