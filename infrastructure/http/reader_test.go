package http

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockReadCloser struct {
	reader io.Reader
	closed bool
}

func (m *mockReadCloser) Read(b []byte) (int, error) {
	return m.reader.Read(b)
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

type mockReadCloserWithError struct {
	closed bool
	err    error
}

func (m *mockReadCloserWithError) Read(b []byte) (int, error) {
	return 0, m.err
}

func (m *mockReadCloserWithError) Close() error {
	m.closed = true
	return nil
}

type errorReadCloser struct {
	closed bool
}

func (e *errorReadCloser) Read(b []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (e *errorReadCloser) Close() error {
	e.closed = true
	return nil
}

func TestNewRequestReader(t *testing.T) {
	t.Run("creates reader with recordBody enabled", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("test")}
		r := NewRequestReader(underlying, 100, true)

		assert.NotNil(t, r)
		assert.Equal(t, 0, r.Len())
		assert.Empty(t, r.Body())
	})

	t.Run("creates reader with recordBody disabled", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("test")}
		r := NewRequestReader(underlying, 100, false)

		assert.NotNil(t, r)
		assert.Equal(t, 0, r.Len())
		assert.Nil(t, r.Body())
	})
}

func TestRequestReaderRead(t *testing.T) {
	t.Run("records bytes read", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello world")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 5)
		n, err := r.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, 5, r.Len())
		assert.Equal(t, []byte("hello"), buf)
		assert.Equal(t, []byte("hello"), r.Body())
	})

	t.Run("records multiple reads", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello world")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 5)
		n1, _ := r.Read(buf)
		n2, _ := r.Read(buf)
		n3, _ := r.Read(buf)

		assert.Equal(t, 5, n1)
		assert.Equal(t, 5, n2)
		assert.Equal(t, 1, n3)
		assert.Equal(t, 11, r.Len())
		assert.Equal(t, []byte("hello world"), r.Body())
	})

	t.Run("respects maxSize for body recording", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello world")}
		r := NewRequestReader(underlying, 5, true)

		buf := make([]byte, 5)
		r.Read(buf)
		r.Read(buf)

		assert.Equal(t, 10, r.Len())
		assert.Equal(t, []byte("hello"), r.Body())
	})

	t.Run("does not record body when disabled", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello world")}
		r := NewRequestReader(underlying, 100, false)

		buf := make([]byte, 5)
		r.Read(buf)

		assert.Equal(t, 5, r.Len())
		assert.Nil(t, r.Body())
	})

	t.Run("handles EOF", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 10)
		n, err := r.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.Equal(t, 5, r.Len())
		assert.Equal(t, []byte("hello"), r.Body())

		// Second read should return EOF
		n, err = r.Read(buf)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 0, n)
	})

	t.Run("propagates read errors", func(t *testing.T) {
		expectedErr := errors.New("read error")
		underlying := &mockReadCloserWithError{err: expectedErr}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 5)
		n, err := r.Read(buf)

		assert.Equal(t, expectedErr, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, 0, r.Len())
	})

	t.Run("partial read after error", func(t *testing.T) {
		underlying := &mockReadCloser{reader: io.MultiReader(
			bytes.NewBufferString("hello"),
			&errorReadCloser{},
		)}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 5)
		n1, err1 := r.Read(buf)
		assert.NoError(t, err1)
		assert.Equal(t, 5, n1)

		n2, err2 := r.Read(buf)
		assert.Error(t, err2)
		assert.Equal(t, 0, n2)
		assert.Equal(t, 5, r.Len())
	})

	t.Run("empty read", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 5)
		n, err := r.Read(buf)

		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, 0, r.Len())
	})
}

func TestRequestReaderBodyRecording(t *testing.T) {
	t.Run("truncates when adding to existing data", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello")}
		r := NewRequestReader(underlying, 3, true)

		buf := make([]byte, 5)
		n, _ := r.Read(buf)

		assert.Equal(t, 5, n)
		assert.Equal(t, []byte("hel"), r.Body())
	})

	t.Run("stops recording after maxSize", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("abcdefghij")}
		r := NewRequestReader(underlying, 5, true)

		buf := make([]byte, 20)
		n, _ := r.Read(buf)

		assert.Equal(t, 10, n)
		assert.Equal(t, 10, r.Len())
		assert.Equal(t, []byte("abcde"), r.Body())
	})

	t.Run("incremental recording up to maxSize", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("abcdefghij")}
		r := NewRequestReader(underlying, 5, true)

		buf := make([]byte, 2)
		r.Read(buf) // reads "ab"
		r.Read(buf) // reads "cd"
		r.Read(buf) // reads "ef"

		assert.Equal(t, 6, r.Len())
		assert.Equal(t, []byte("abcde"), r.Body())
	})

	t.Run("body is nil when recording disabled", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello")}
		r := NewRequestReader(underlying, 100, false)

		buf := make([]byte, 5)
		n, _ := r.Read(buf)

		assert.Equal(t, 5, n)
		assert.Nil(t, r.Body())
	})
}

func TestRequestReaderLen(t *testing.T) {
	t.Run("tracks cumulative bytes read", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("hello world")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 3)
		r.Read(buf) // 3 bytes
		r.Read(buf) // 3 bytes
		r.Read(buf) // 3 bytes
		r.Read(buf) // 2 bytes + EOF

		assert.Equal(t, 11, r.Len())
	})

	t.Run("continues counting after maxSize reached", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("abcdefghij")}
		r := NewRequestReader(underlying, 3, true)

		buf := make([]byte, 10)
		_, _ = r.Read(buf)

		assert.Equal(t, 10, r.Len())
		assert.Equal(t, []byte("abc"), r.Body())
	})
}

func TestRequestReaderClose(t *testing.T) {
	t.Run("closes underlying reader", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("test")}
		r := NewRequestReader(underlying, 100, true)

		err := r.Close()

		assert.NoError(t, err)
		assert.True(t, underlying.closed)
	})

	t.Run("propagates close errors", func(t *testing.T) {
		expectedErr := errors.New("close failed")
		underlying := &closeErrorReadCloser{err: expectedErr}
		r := NewRequestReader(underlying, 100, true)

		err := r.Close()

		assert.Equal(t, expectedErr, err)
	})
}

type closeErrorReadCloser struct {
	err error
}

func (c *closeErrorReadCloser) Read(b []byte) (int, error) {
	return 0, nil
}

func (c *closeErrorReadCloser) Close() error {
	return c.err
}

func TestRequestReaderIntegration(t *testing.T) {
	t.Run("full request body reading", func(t *testing.T) {
		body := "POST data with special chars: éàü"
		underlying := &mockReadCloser{reader: bytes.NewBufferString(body)}
		r := NewRequestReader(underlying, 1000, true)

		buf := make([]byte, 64)
		n, err := r.Read(buf)

		assert.NoError(t, err)
		assert.Equal(t, len(body), n)
		assert.Equal(t, len(body), r.Len())
		assert.Equal(t, []byte(body), r.Body())

		// Second read should return EOF
		n, err = r.Read(buf)
		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 0, n)
	})

	t.Run("large body with small maxSize", func(t *testing.T) {
		largeBody := bytes.Repeat([]byte("x"), 1000)
		underlying := &mockReadCloser{reader: bytes.NewReader(largeBody)}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 256)
		var totalRead int
		for {
			n, err := r.Read(buf)
			totalRead += n
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
		}

		assert.Equal(t, 1000, totalRead)
		assert.Equal(t, 1000, r.Len())
		assert.Equal(t, 100, len(r.Body()))
	})

	t.Run("preserves body ordering", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("abcdefghij")}
		r := NewRequestReader(underlying, 100, true)

		buf := make([]byte, 2)
		r.Read(buf)
		r.Read(buf)
		r.Read(buf)
		r.Read(buf)
		r.Read(buf)

		assert.Equal(t, []byte("abcdefghij"), r.Body())
	})
}

func TestWrapRequestReaderInterface(t *testing.T) {
	t.Run("requestReader implements WrapRequestReader", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("test")}
		r := NewRequestReader(underlying, 100, true)

		var _ WrapRequestReader = r
	})

	t.Run("requestReader implements io.ReadCloser", func(t *testing.T) {
		underlying := &mockReadCloser{reader: bytes.NewBufferString("test")}
		r := NewRequestReader(underlying, 100, true)

		var _ io.ReadCloser = r
	})
}
