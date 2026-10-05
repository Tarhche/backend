package minio

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3 is as much of S3 as reaching a bucket and storing an object in parts
// takes. It keeps how large each part it was sent was, which is how much of
// the object the client held in memory at a time.
type fakeS3 struct {
	mu        sync.Mutex
	reachable bool
	parts     []int
	objects   map[string][]byte
	uploading map[int][]byte
}

func newFakeS3() *fakeS3 {
	return &fakeS3{reachable: true, objects: make(map[string][]byte), uploading: make(map[int][]byte)}
}

func (s *fakeS3) partSizes() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]int(nil), s.parts...)
}

func (s *fakeS3) setReachable(reachable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reachable = reachable
}

func (s *fakeS3) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.reachable {
		http.Error(rw, "unavailable", http.StatusServiceUnavailable)

		return
	}

	query := r.URL.Query()
	bucket, object, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")

	switch {
	case query.Has("location"):
		rw.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(rw, `<?xml version="1.0" encoding="UTF-8"?><LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)

	case r.Method == http.MethodHead && len(object) == 0:
		rw.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPost && query.Has("uploads"):
		rw.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(rw, `<?xml version="1.0" encoding="UTF-8"?><InitiateMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`, bucket, object)

	case r.Method == http.MethodPut && query.Has("partNumber"):
		body := decoded(r)
		number, _ := strconv.Atoi(query.Get("partNumber"))
		s.parts = append(s.parts, len(body))
		s.uploading[number] = body
		rw.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, number))
		rw.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPost && query.Has("uploadId"):
		var whole []byte
		for number := 1; number <= len(s.uploading); number++ {
			whole = append(whole, s.uploading[number]...)
		}

		s.objects[object] = whole
		rw.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(rw, `<?xml version="1.0" encoding="UTF-8"?><CompleteMultipartUploadResult><Bucket>%s</Bucket><Key>%s</Key><ETag>"whole"</ETag></CompleteMultipartUploadResult>`, bucket, object)

	case r.Method == http.MethodPut:
		body := decoded(r)
		s.parts = append(s.parts, len(body))
		s.objects[object] = body
		rw.Header().Set("ETag", `"whole"`)
		rw.WriteHeader(http.StatusOK)

	default:
		http.Error(rw, "not faked: "+r.Method+" "+r.URL.String(), http.StatusNotImplemented)
	}
}

// decoded is what a request carried, without the chunk signatures a streaming
// upload interleaves with it.
func decoded(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)

	if r.Header.Get("X-Amz-Content-Sha256") != "STREAMING-AWS4-HMAC-SHA256-PAYLOAD" &&
		!strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
		return body
	}

	var payload []byte
	for len(body) > 0 {
		header, rest, found := bytes.Cut(body, []byte("\r\n"))
		if !found {
			break
		}

		sizeHex, _, _ := strings.Cut(string(header), ";")
		size, err := strconv.ParseInt(sizeHex, 16, 64)
		if err != nil || size == 0 {
			break
		}

		payload = append(payload, rest[:size]...)
		body = rest[size+2:]
	}

	return payload
}

func TestLazy(t *testing.T) {
	t.Parallel()

	t.Run("an archive of unknown size is stored in parts of the size asked for", func(t *testing.T) {
		t.Parallel()

		s3 := newFakeS3()
		server := httptest.NewServer(s3)
		defer server.Close()

		storage := NewLazy(Options{
			Endpoint:   strings.TrimPrefix(server.URL, "http://"),
			BucketName: "workload-snapshots",
			PartSize:   5 << 20,
		})

		archive := bytes.Repeat([]byte("snapshot"), (12<<20)/8)
		require.NoError(t, storage.Store(t.Context(), "snapshots/one.msb", bytes.NewReader(archive), -1))

		// a reader of unknown length, read a part at a time: nothing larger
		// than a part was ever held.
		assert.Equal(t, []int{5 << 20, 5 << 20, 2 << 20}, s3.partSizes())
		assert.Equal(t, archive, s3.objects["snapshots/one.msb"])
	})

	t.Run("nothing is reached until the store is used", func(t *testing.T) {
		t.Parallel()

		s3 := newFakeS3()
		s3.setReachable(false)

		server := httptest.NewServer(s3)
		defer server.Close()

		storage := NewLazy(Options{Endpoint: strings.TrimPrefix(server.URL, "http://"), BucketName: "workload-snapshots"})

		err := storage.Store(t.Context(), "snapshots/one.msb", strings.NewReader("archive"), -1)
		assert.Error(t, err, "S3 is away")

		_, err = storage.Read(t.Context(), "snapshots/one.msb")
		assert.Error(t, err)

		// S3 coming back is a store that works, without making it again.
		s3.setReachable(true)

		assert.NoError(t, storage.Store(t.Context(), "snapshots/one.msb", strings.NewReader("archive"), -1))
	})
}
