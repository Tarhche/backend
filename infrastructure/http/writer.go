package http

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
)

// WrapResponseWriter is an interface that extends http.ResponseWriter
// with additional methods to access the response status code, length, and body.
type WrapResponseWriter interface {
	http.ResponseWriter
	http.Flusher
	http.Hijacker
	io.ReaderFrom

	Status() int
	Len() int
	Body() []byte
}

// responseWriter wraps an http.ResponseWriter to record the status code, the number
// of bytes written and, optionally, a bounded copy of the response body.
type responseWriter struct {
	http.ResponseWriter
	body    *bytes.Buffer
	maxSize int
	length  int
	status  int
}

// Ensure responseWriter implements the WrapResponseWriter interface.
var _ WrapResponseWriter = &responseWriter{}

func NewResponseWriter(rw http.ResponseWriter, maxSize int, recordBody bool) *responseWriter {
	var body *bytes.Buffer
	if recordBody {
		body = bytes.NewBufferString("")
	}

	return &responseWriter{
		ResponseWriter: rw,
		body:           body,
		maxSize:        maxSize,
		length:         0,
		status:         http.StatusOK,
	}
}

// Write writes the data to the connection as part of an HTTP reply.
// It records the number of bytes written and, optionally, a bounded copy of the response body.
func (w *responseWriter) Write(b []byte) (int, error) {
	length := len(b)

	if w.body != nil {
		if w.body.Len()+length > w.maxSize {
			w.body.Truncate(min(w.maxSize, length, w.body.Len()))
			w.body.Write(b[:min(w.maxSize-w.body.Len(), length)])
		} else {
			w.body.Write(b)
		}
	}
	w.length += length

	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hi, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hi.Hijack()
	}

	return nil, nil, errors.New("Hijack not supported")
}

func (w *responseWriter) ReadFrom(r io.Reader) (int64, error) {
	if w.body == nil {
		if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
			n, err := rf.ReadFrom(r)
			w.length += int(n)
			return n, err
		}
	}

	// hide ReaderFrom interface to avoid infinite recursion
	return io.Copy(struct{ io.Writer }{w}, r)
}

// Unwrap exposes the wrapped writer for http.ResponseController.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Status returns the HTTP status code of the response.
func (w *responseWriter) Status() int {
	return w.status
}

// Len returns the number of bytes written to the response.
func (w *responseWriter) Len() int {
	return w.length
}

// Body returns a bounded copy of the response body, or nil if recording is disabled.
func (w *responseWriter) Body() []byte {
	if w.body == nil {
		return nil
	}

	return w.body.Bytes()
}
