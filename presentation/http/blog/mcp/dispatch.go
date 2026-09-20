package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// maxResponseBytes is as much of an answer as a tool hands back. What a
	// route serves is sometimes a file, and a file is not always something an
	// agent should be made to read.
	maxResponseBytes = 1 << 20

	// uploadField is what the file upload route reads its file out of.
	uploadField = "file"
)

// headers a tool call carries from the MCP request into the request it makes:
// who is asking, which language to answer in, and who the request came from
// before it reached us.
var carriedHeaders = []string{"Authorization", "X-Language-Code", "X-Forwarded-For"}

// call makes the tool's request inside this process, through the same router
// an HTTP client reaches, and answers with whatever the route answered.
//
// Nothing here decides whether the caller may: the request goes through the
// route's own middleware, so it is authenticated and authorized exactly as it
// would be over the network, and a tool that names a route nobody may call
// comes back refused.
func (s *server) call(ctx context.Context, t tool, header http.Header, arguments map[string]any) (*mcpsdk.CallToolResult, error) {
	request, err := s.request(ctx, t, header, arguments)
	if err != nil {
		return refused(err.Error()), nil
	}

	recorder := &recorder{header: make(http.Header)}
	s.router.ServeHTTP(recorder, request)

	return answer(t, recorder), nil
}

func (s *server) request(ctx context.Context, t tool, header http.Header, arguments map[string]any) (*http.Request, error) {
	path := t.path()
	taken := make(map[string]struct{}, len(arguments))

	for _, placeholder := range t.placeholders() {
		name := snake(placeholder)

		value, ok := arguments[name]
		if !ok {
			return nil, fmt.Errorf("%q is required", name)
		}

		path = strings.Replace(path, "{"+placeholder+"}", url.PathEscape(literal(value)), 1)
		taken[name] = struct{}{}
	}

	query := make(url.Values, len(t.params))
	for _, parameter := range t.params {
		if _, used := taken[parameter.name]; used {
			continue
		}

		if value, ok := arguments[parameter.name]; ok && value != nil {
			query.Set(parameter.name, literal(value))
			taken[parameter.name] = struct{}{}
		}
	}

	rest := make(map[string]any, len(arguments))
	for name, value := range arguments {
		if _, used := taken[name]; !used {
			rest[name] = value
		}
	}

	body, contentType, err := payload(t, rest)
	if err != nil {
		return nil, err
	}

	target := path
	if encoded := query.Encode(); len(encoded) > 0 {
		target += "?" + encoded
	}

	request, err := http.NewRequestWithContext(ctx, t.method(), "http://"+internalHost+target, body)
	if err != nil {
		return nil, err
	}

	// a request that arrived over the network carries its host beside the url
	// rather than inside it. Making this one look the same is not tidiness: the
	// cache keys a route's answer by the url it was asked for, and a tool
	// asking for the same thing should find the same answer.
	request.URL.Scheme = ""
	request.URL.Host = ""
	request.Host = internalHost
	request.RequestURI = target

	if len(contentType) > 0 {
		request.Header.Set("Content-Type", contentType)
	}

	request.Header.Set("Accept", "application/json")

	for _, name := range carriedHeaders {
		if value := header.Get(name); len(value) > 0 {
			request.Header.Set(name, value)
		}
	}

	return request, nil
}

// payload is what the request carries: the json a route reads, the file an
// upload route reads, or nothing at all.
func payload(t tool, arguments map[string]any) (io.Reader, string, error) {
	if t.upload {
		return file(arguments)
	}

	// a request that carries nothing still carries an empty body: a handler
	// reading one it was not given would be reading nothing at all.
	if t.body == nil || len(arguments) == 0 {
		return http.NoBody, "", nil
	}

	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, "", err
	}

	return bytes.NewReader(encoded), "application/json", nil
}

func file(arguments map[string]any) (io.Reader, string, error) {
	name, _ := arguments["name"].(string)
	encoded, _ := arguments["content"].(string)

	content, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", fmt.Errorf("the content is not base64: %w", err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	part, err := form.CreateFormFile(uploadField, name)
	if err != nil {
		return nil, "", err
	}

	if _, err := part.Write(content); err != nil {
		return nil, "", err
	}

	if err := form.Close(); err != nil {
		return nil, "", err
	}

	return &body, form.FormDataContentType(), nil
}

// answer turns what the route wrote into what the caller is told.
//
// A refusal is handed back as a failed call rather than as a protocol error,
// so the agent reading it can see why and do something else: ask for
// something it may have, or say that it cannot.
func answer(t tool, recorder *recorder) *mcpsdk.CallToolResult {
	body := recorder.body.Bytes()
	if len(body) > maxResponseBytes {
		body = body[:maxResponseBytes]
	}

	switch {
	case recorder.status == http.StatusUnauthorized:
		return refused("this session is not signed in, or its access token has expired")
	case recorder.status == http.StatusForbidden:
		return refused("this session may not do that")
	case recorder.status == http.StatusNotFound:
		return refused("there is no such thing here")
	case recorder.status == http.StatusTooManyRequests:
		return refused("too many requests; wait a moment and try again")
	case recorder.status >= http.StatusBadRequest:
		return refused(fmt.Sprintf("the request was refused (%d) %s", recorder.status, strings.TrimSpace(string(body))))
	}

	if t.download {
		return downloaded(t, recorder, body)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return content(fmt.Sprintf(`{"status":%d}`, recorder.status))
	}

	return content(string(body))
}

// downloaded hands back a file as what it is: readable when it is text, and
// the bytes themselves when it is not.
func downloaded(t tool, recorder *recorder, body []byte) *mcpsdk.CallToolResult {
	mimeType := recorder.header.Get("Content-Type")
	if len(mimeType) == 0 {
		mimeType = "application/octet-stream"
	}

	contents := &mcpsdk.ResourceContents{
		URI:      "file://" + t.name,
		MIMEType: mimeType,
	}

	if strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") {
		contents.Text = string(body)
	} else {
		contents.Blob = body
	}

	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.EmbeddedResource{Resource: contents}},
	}
}

func content(value string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: value}},
	}
}

func refused(reason string) *mcpsdk.CallToolResult {
	result := content(reason)
	result.IsError = true

	return result
}

// literal writes an argument the way a path or a query string carries it.
// What arrives is json, so a number is a float even when it was written as an
// integer, and it goes back out as it was written.
func literal(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

// recorder keeps what a handler wrote, so it can be handed back rather than
// sent anywhere.
type recorder struct {
	status int
	header http.Header
	body   bytes.Buffer
}

var _ http.ResponseWriter = &recorder{}

func (r *recorder) Header() http.Header {
	return r.header
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	// what is written past what is handed back is counted and dropped: a
	// handler serving a large file should not be made to fail, and should not
	// be kept in memory either.
	if r.body.Len() >= maxResponseBytes {
		return len(p), nil
	}

	return r.body.Write(p)
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}
