package http

import (
	"bytes"
	"io"
)

type WrapRequestReader interface {
	io.ReadCloser

	Len() int
	Body() []byte
}

// requestReader wraps an io.ReadCloser to record the number of bytes read and,
// optionally, a bounded copy of the body.
type requestReader struct {
	io.ReadCloser
	body    *bytes.Buffer
	maxSize int
	length  int
}

// Ensure requestReader implements the WrapRequestReader interface.
var _ WrapRequestReader = &requestReader{}

func NewRequestReader(reader io.ReadCloser, maxSize int, recordBody bool) *requestReader {
	var body *bytes.Buffer
	if recordBody {
		body = bytes.NewBufferString("")
	}

	return &requestReader{
		ReadCloser: reader,
		body:       body,
		maxSize:    maxSize,
		length:     0,
	}
}

func (r *requestReader) Read(b []byte) (int, error) {
	n, err := r.ReadCloser.Read(b)

	if r.body != nil {
		if remain := r.maxSize - r.body.Len(); remain > 0 {
			r.body.Write(b[:min(n, remain)])
		}
	}
	r.length += n

	return n, err
}

func (r *requestReader) Len() int {
	return r.length
}

func (r *requestReader) Body() []byte {
	if r.body == nil {
		return nil
	}

	return r.body.Bytes()
}
