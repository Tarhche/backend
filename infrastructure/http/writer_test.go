package http

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockResponseWriter struct {
	header      http.Header
	body        *bytes.Buffer
	statusCode  int
	flushed     bool
	hijacked    bool
	hijackError error
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func (m *mockResponseWriter) Flush() {
	m.flushed = true
}

func (m *mockResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if m.hijackError != nil {
		return nil, nil, m.hijackError
	}
	m.hijacked = true
	return nil, nil, nil
}

type mockResponseWriterNoFlusher struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (m *mockResponseWriterNoFlusher) Header() http.Header {
	return m.header
}

func (m *mockResponseWriterNoFlusher) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriterNoFlusher) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

type mockResponseWriterNoHijacker struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (m *mockResponseWriterNoHijacker) Header() http.Header {
	return m.header
}

func (m *mockResponseWriterNoHijacker) Write(b []byte) (int, error) {
	return m.body.Write(b)
}

func (m *mockResponseWriterNoHijacker) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func TestNewResponseWriter(t *testing.T) {
	t.Run("creates writer with recordBody enabled", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		assert.NotNil(t, w)
		assert.Equal(t, http.StatusOK, w.Status())
		assert.Equal(t, 0, w.Len())
		assert.Empty(t, w.Body())
	})

	t.Run("creates writer with recordBody disabled", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, false)

		assert.NotNil(t, w)
		assert.Equal(t, http.StatusOK, w.Status())
		assert.Equal(t, 0, w.Len())
		assert.Nil(t, w.Body())
	})
}

func TestResponseWriterWrite(t *testing.T) {
	t.Run("records bytes written", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		data := []byte("hello world")
		n, err := w.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, len(data), w.Len())
		assert.Equal(t, data, w.Body())
		assert.Equal(t, data, rw.body.Bytes())
	})

	t.Run("records multiple writes", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		w.Write([]byte("hello"))
		w.Write([]byte(" "))
		w.Write([]byte("world"))

		assert.Equal(t, 11, w.Len())
		assert.Equal(t, []byte("hello world"), w.Body())
	})

	t.Run("truncates body when exceeding maxSize", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 5, true)

		w.Write([]byte("hello"))
		w.Write([]byte(" world"))

		assert.Equal(t, 11, w.Len())
		assert.Equal(t, []byte("hello"), w.Body())
	})

	t.Run("does not record body when disabled", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, false)

		w.Write([]byte("hello"))

		assert.Equal(t, 5, w.Len())
		assert.Nil(t, w.Body())
		assert.Equal(t, []byte("hello"), rw.body.Bytes())
	})

	t.Run("handles empty write", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		n, err := w.Write([]byte{})

		assert.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, 0, w.Len())
	})
}

func TestResponseWriterWriteHeader(t *testing.T) {
	t.Run("records status code", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		w.WriteHeader(http.StatusCreated)

		assert.Equal(t, http.StatusCreated, w.Status())
		assert.Equal(t, http.StatusCreated, rw.statusCode)
	})

	t.Run("default status is 200 OK", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		assert.Equal(t, http.StatusOK, w.Status())
	})

	t.Run("can change status before first write", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))

		assert.Equal(t, http.StatusNotFound, w.Status())
	})
}

func TestResponseWriterFlush(t *testing.T) {
	t.Run("flushes underlying writer", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		w.Flush()

		assert.True(t, rw.flushed)
	})

	t.Run("handles writer without flusher", func(t *testing.T) {
		rw := &mockResponseWriterNoFlusher{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		assert.NotPanics(t, func() {
			w.Flush()
		})
	})
}

func TestResponseWriterHijack(t *testing.T) {
	t.Run("hijacks underlying writer", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		conn, readWriter, err := w.Hijack()

		assert.NoError(t, err)
		assert.Nil(t, conn)
		assert.Nil(t, readWriter)
		assert.True(t, rw.hijacked)
	})

	t.Run("returns error when underlying writer does not support hijack", func(t *testing.T) {
		rw := &mockResponseWriterNoHijacker{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		conn, readWriter, err := w.Hijack()

		assert.Error(t, err)
		assert.Equal(t, "Hijack not supported", err.Error())
		assert.Nil(t, conn)
		assert.Nil(t, readWriter)
	})

	t.Run("propagates hijack errors", func(t *testing.T) {
		expectedErr := errors.New("hijack failed")
		rw := &mockResponseWriter{
			header:      make(http.Header),
			body:        &bytes.Buffer{},
			hijackError: expectedErr,
		}
		w := NewResponseWriter(rw, 100, true)

		conn, readWriter, err := w.Hijack()

		assert.Equal(t, expectedErr, err)
		assert.Nil(t, conn)
		assert.Nil(t, readWriter)
	})
}

func TestResponseWriterReadFrom(t *testing.T) {
	t.Run("copies from reader when body recording disabled", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, false)

		src := bytes.NewBufferString("hello world")
		n, err := w.ReadFrom(src)

		assert.NoError(t, err)
		assert.Equal(t, int64(11), n)
		assert.Equal(t, 11, w.Len())
		assert.Nil(t, w.Body())
	})

	t.Run("copies from reader when body recording enabled", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		src := bytes.NewBufferString("hello world")
		n, err := w.ReadFrom(src)

		assert.NoError(t, err)
		assert.Equal(t, int64(11), n)
		assert.Equal(t, 11, w.Len())
		assert.Equal(t, []byte("hello world"), w.Body())
	})

	t.Run("respects maxSize when reading", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 5, true)

		src := bytes.NewBufferString("hello world")
		n, err := w.ReadFrom(src)

		assert.NoError(t, err)
		assert.Equal(t, int64(11), n)
		assert.Equal(t, 11, w.Len())
		assert.Equal(t, []byte("hello"), w.Body())
	})
}

func TestResponseWriterUnwrap(t *testing.T) {
	t.Run("returns underlying writer", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		unwrapped := w.Unwrap()

		assert.Equal(t, rw, unwrapped)
	})
}

func TestResponseWriterWithHTTPTestRecorder(t *testing.T) {
	t.Run("works with httptest.ResponseRecorder", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w := NewResponseWriter(rr, 100, true)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
		w.Flush()

		assert.Equal(t, http.StatusOK, w.Status())
		assert.Equal(t, 5, w.Len())
		assert.Equal(t, []byte("hello"), w.Body())
		assert.Equal(t, "hello", rr.Body.String())
	})
}

func TestResponseWriterHeader(t *testing.T) {
	t.Run("delegates header to underlying writer", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		header := w.Header()

		assert.Equal(t, rw.header, header)
	})
}

func TestWrapResponseWriterInterface(t *testing.T) {
	t.Run("responseWriter implements WrapResponseWriter", func(t *testing.T) {
		rw := &mockResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
		w := NewResponseWriter(rw, 100, true)

		var _ WrapResponseWriter = w
	})
}
