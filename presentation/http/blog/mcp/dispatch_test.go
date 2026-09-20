package mcp

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answered reads back what a caller is told: the text of it, and whether the
// call failed.
func answered(t *testing.T, result *mcpsdk.CallToolResult) (string, bool) {
	t.Helper()

	require.NotNil(t, result)

	var text strings.Builder
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcpsdk.TextContent:
			text.WriteString(c.Text)
		case *mcpsdk.EmbeddedResource:
			require.NotNil(t, c.Resource)
			text.WriteString(c.Resource.Text)
			if len(c.Resource.Blob) > 0 {
				text.WriteString("blob:" + base64.StdEncoding.EncodeToString(c.Resource.Blob))
			}
		}
	}

	return text.String(), result.IsError
}

// answeredWith is a route's answer, as the dispatcher recorded it.
func answeredWith(status int, contentType string, body string) *recorder {
	r := &recorder{header: make(http.Header)}

	if len(contentType) > 0 {
		r.header.Set("Content-Type", contentType)
	}

	r.WriteHeader(status)
	r.Write([]byte(body))

	return r
}

func TestLiteral(t *testing.T) {
	t.Parallel()

	// what arrives is json, so a page number is a float even though nobody
	// wrote one
	assert.Equal(t, "2", literal(float64(2)))
	assert.Equal(t, "1.5", literal(1.5))
	assert.Equal(t, "an-uuid", literal("an-uuid"))
	assert.Equal(t, "true", literal(true))
	assert.Equal(t, "", literal(nil))

	// a json number is a float, so an integer past 2^53 is written back as the
	// nearest one that is. Nothing here is named by a number that large.
	assert.Equal(t, "9007199254740992", literal(float64(9007199254740993)))
}

func TestRecorder(t *testing.T) {
	t.Parallel()

	t.Run("a handler that writes without saying so answered 200", func(t *testing.T) {
		r := &recorder{header: make(http.Header)}
		r.Write([]byte("something"))

		assert.Equal(t, http.StatusOK, r.status)
		assert.Equal(t, "something", r.body.String())
	})

	t.Run("the first status is the one that counts", func(t *testing.T) {
		r := &recorder{header: make(http.Header)}
		r.WriteHeader(http.StatusCreated)
		r.WriteHeader(http.StatusTeapot)

		assert.Equal(t, http.StatusCreated, r.status)
	})

	t.Run("what is past the ceiling is counted and dropped", func(t *testing.T) {
		r := &recorder{header: make(http.Header)}

		written, err := r.Write(make([]byte, maxResponseBytes+10))
		require.NoError(t, err)
		assert.Equal(t, maxResponseBytes+10, written)

		// a handler serving a large file is not made to fail, and what it
		// wrote past the ceiling is not kept either
		written, err = r.Write([]byte("more"))
		require.NoError(t, err)
		assert.Equal(t, 4, written)
		assert.Equal(t, maxResponseBytes+10, r.body.Len())
	})
}

func TestAnswer(t *testing.T) {
	t.Parallel()

	listing := tool{name: "things_list", route: "GET /api/things", readOnly: true}

	t.Run("what a route answered is what the caller is told", func(t *testing.T) {
		text, isError := answered(t, answer(listing, answeredWith(http.StatusOK, "application/json", `{"items":[]}`)))

		assert.False(t, isError)
		assert.Equal(t, `{"items":[]}`, text)
	})

	t.Run("an answer with nothing in it still says it worked", func(t *testing.T) {
		text, isError := answered(t, answer(listing, answeredWith(http.StatusNoContent, "", "")))

		assert.False(t, isError)
		assert.Equal(t, `{"status":204}`, text)
	})

	t.Run("a refusal is a failed call with a reason, not a protocol error", func(t *testing.T) {
		for status, expected := range map[int]string{
			http.StatusUnauthorized:    "expired",
			http.StatusForbidden:       "may not",
			http.StatusNotFound:        "no such thing",
			http.StatusTooManyRequests: "too many requests",
		} {
			text, isError := answered(t, answer(listing, answeredWith(status, "", "")))

			assert.True(t, isError, status)
			assert.Contains(t, text, expected)
		}
	})

	t.Run("what was wrong with a request is handed back whole", func(t *testing.T) {
		text, isError := answered(t, answer(listing, answeredWith(http.StatusBadRequest, "application/json", `{"errors":{"title":"required_field"}}`)))

		assert.True(t, isError)
		assert.Contains(t, text, "required_field")
	})

	t.Run("a failure is a failure", func(t *testing.T) {
		text, isError := answered(t, answer(listing, answeredWith(http.StatusInternalServerError, "", "")))

		assert.True(t, isError)
		assert.Contains(t, text, "500")
	})

	t.Run("a file that reads as text is handed over as text", func(t *testing.T) {
		download := tool{name: "file_download", route: "GET /files/{uuid}", download: true}

		text, isError := answered(t, answer(download, answeredWith(http.StatusOK, "text/markdown", "# a note")))

		assert.False(t, isError)
		assert.Equal(t, "# a note", text)
	})

	t.Run("a file that does not is handed over as bytes", func(t *testing.T) {
		download := tool{name: "file_download", route: "GET /files/{uuid}", download: true}

		text, isError := answered(t, answer(download, answeredWith(http.StatusOK, "image/png", "\x89PNG")))

		assert.False(t, isError)
		assert.Equal(t, "blob:"+base64.StdEncoding.EncodeToString([]byte("\x89PNG")), text)
	})
}

func TestPayload(t *testing.T) {
	t.Parallel()

	t.Run("a route that reads nothing is sent an empty body rather than none", func(t *testing.T) {
		body, contentType, err := payload(tool{route: "GET /api/things"}, map[string]any{})

		require.NoError(t, err)
		assert.Empty(t, contentType)

		// a handler reading a body it was not given would be reading nothing
		// at all, which is a panic rather than an empty request
		assert.Equal(t, http.NoBody, body)
	})

	t.Run("what is left over becomes the json a route reads", func(t *testing.T) {
		body, contentType, err := payload(
			tool{route: "POST /api/things", body: object(map[string]*jsonschema.Schema{"title": {Type: "string"}})},
			map[string]any{"title": "a title"},
		)

		require.NoError(t, err)
		assert.Equal(t, "application/json", contentType)

		read, err := io.ReadAll(body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"title":"a title"}`, string(read))
	})

	t.Run("a file is sent as the form a route reads it from", func(t *testing.T) {
		body, contentType, err := payload(
			tool{route: "POST /api/files", upload: true},
			map[string]any{"name": "notes.txt", "content": base64.StdEncoding.EncodeToString([]byte("hello"))},
		)
		require.NoError(t, err)

		mediaType, params, err := mime.ParseMediaType(contentType)
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)

		part, err := multipart.NewReader(body, params["boundary"]).NextPart()
		require.NoError(t, err)
		assert.Equal(t, uploadField, part.FormName())
		assert.Equal(t, "notes.txt", part.FileName())

		content, err := io.ReadAll(part)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("content that is not base64 is said to be", func(t *testing.T) {
		_, _, err := payload(
			tool{route: "POST /api/files", upload: true},
			map[string]any{"name": "notes.txt", "content": "not base64 at all!"},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "base64")
	})
}

func TestRequestBuilding(t *testing.T) {
	t.Parallel()

	s := &server{}

	header := http.Header{}
	header.Set("Authorization", "Bearer a-token")
	header.Set("X-Language-Code", "fa")
	header.Set("Cookie", "session=not-carried")

	t.Run("a path is filled in, a query is added and the rest is the body", func(t *testing.T) {
		request, err := s.request(context.Background(), tool{
			name:   "thing_update",
			route:  "PUT /api/things/{thingUUID}/{language_code}",
			params: []Parameter{text("thing_uuid", "which thing"), languageCode(), page()},
			body:   object(map[string]*jsonschema.Schema{"title": {Type: "string"}}),
		}, header, map[string]any{
			"thing_uuid":    "an uuid",
			"language_code": "fa",
			"page":          float64(3),
			"title":         "a title",
		})
		require.NoError(t, err)

		assert.Equal(t, http.MethodPut, request.Method)
		assert.Equal(t, "/api/things/an uuid/fa", request.URL.Path)
		assert.Equal(t, "/api/things/an%20uuid/fa", request.URL.EscapedPath())
		assert.Equal(t, "page=3", request.URL.RawQuery)

		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"title":"a title"}`, string(body))
	})

	t.Run("it carries who is asking and in which language, and nothing else", func(t *testing.T) {
		request, err := s.request(context.Background(), tool{
			name:  "things_list",
			route: "GET /api/things",
		}, header, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, "Bearer a-token", request.Header.Get("Authorization"))
		assert.Equal(t, "fa", request.Header.Get("X-Language-Code"))
		assert.Equal(t, "application/json", request.Header.Get("Accept"))
		assert.Empty(t, request.Header.Get("Cookie"))
	})

	t.Run("it looks like a request that arrived rather than one going out", func(t *testing.T) {
		request, err := s.request(context.Background(), tool{
			name:  "things_list",
			route: "GET /api/things",
		}, header, map[string]any{})
		require.NoError(t, err)

		// the cache keys a route's answer by the url it was asked for, and a
		// tool asking for the same thing should find the same answer
		assert.Empty(t, request.URL.Scheme)
		assert.Empty(t, request.URL.Host)
		assert.Equal(t, internalHost, request.Host)
		assert.Equal(t, "/api/things", request.RequestURI)
	})

	t.Run("a path with nothing to fill it in with is refused before it is made", func(t *testing.T) {
		_, err := s.request(context.Background(), tool{
			name:   "thing_show",
			route:  "GET /api/things/{uuid}",
			params: []Parameter{uuidOf("thing")},
		}, header, map[string]any{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "uuid")
	})

	t.Run("a parameter that was not asked for is left out of the query", func(t *testing.T) {
		request, err := s.request(context.Background(), tool{
			name:   "things_list",
			route:  "GET /api/things",
			params: []Parameter{page(), text("after", "since when")},
		}, header, map[string]any{"page": float64(2)})
		require.NoError(t, err)

		assert.Equal(t, "page=2", request.URL.RawQuery)
	})

	t.Run("the context it is given is the one it carries", func(t *testing.T) {
		type key struct{}

		ctx := context.WithValue(context.Background(), key{}, "carried")

		request, err := s.request(ctx, tool{name: "things_list", route: "GET /api/things"}, header, map[string]any{})
		require.NoError(t, err)

		assert.Equal(t, "carried", request.Context().Value(key{}))
	})
}
