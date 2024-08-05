package mock

import (
	"context"
	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/stretchr/testify/mock"
	"io"
)

type MockStorage struct {
	mock.Mock
}

var _ file.Storage = &MockStorage{}

func (s *MockStorage) Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error {
	args := s.Called(ctx, objectName, reader, objectSize)

	return args.Error(0)
}

func (s *MockStorage) Delete(ctx context.Context, objectName string) error {
	args := s.Called(ctx, objectName)

	return args.Error(0)
}

func (s *MockStorage) Read(ctx context.Context, objectName string) (io.ReadCloser, error) {
	args := s.Called(ctx, objectName)

	if obj, ok := args.Get(0).(io.ReadCloser); ok {
		return obj, args.Error(1)
	}

	return nil, args.Error(1)
}
