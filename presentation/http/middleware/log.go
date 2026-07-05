package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraHttp "github.com/khanzadimahdi/testproject/infrastructure/http"
)

// request attribute keys are used when request attributes are logged.
const (
	requestGroupKey = "request"

	// requestTimeKey is the attribute key used when the request time is logged.
	requestTimeKey = "time"

	// requestMethodKey is the attribute key used when the request method is logged.
	requestMethodKey = "method"

	// requestHostKey is the attribute key used when the request host is logged.
	requestHostKey = "host"

	// requestPathKey is the attribute key used when the request path is logged.
	requestPathKey = "path"

	// requestQueryKey is the attribute key used when the query is logged.
	requestQueryKey = "query"

	// requestRefererKey is the attribute key used when the referer is logged.
	requestRefererKey = "referer"

	// requestLengthKey is the attribute key used when the request length is logged.
	requestLengthKey = "length"

	// requestIDKey is the attribute key used when the request id is logged.
	requestIDKey = "id"

	// requestIPKey is the attribute key used when the client IP is logged.
	requestIPKey = "ip"

	// requestUserAgentKey is the attribute key used when the user agent is logged.
	requestUserAgentKey = "user-agent"

	// requestBodyKey is the attribute key used when the request body is logged.
	requestBodyKey = "body"

	// requestHeaderKey is the attribute key used when the request headers are logged.
	requestHeaderKey = "header"
)

// response attribute keys are used when response attributes are logged.
const (
	responseGroupKey = "response"

	// responseTimeKey is the attribute key used when the response time is logged.
	responseTimeKey = "time"

	// responseLatencyKey is the attribute key used when the response latency is logged.
	responseLatencyKey = "latency"

	// responseStatusKey is the attribute key used when the response status is logged.
	responseStatusKey = "status"

	// responseLengthKey is the attribute key used when the response length is logged.
	responseLengthKey = "length"

	// responseBodyKey is the attribute key used when the response body is logged.
	responseBodyKey = "body"

	// responseHeaderKey is the attribute key used when the response headers are logged.
	responseHeaderKey = "header"
)

var (
	requestBodyMaxSize  = 64 * 1024 // 64KB
	responseBodyMaxSize = 64 * 1024 // 64KB

	// hiddenRequestHeaders are request headers that are never logged.
	hiddenRequestHeaders = map[string]struct{}{
		"authorization": {},
		"cookie":        {},
		"set-cookie":    {},
		"x-auth-token":  {},
		"x-csrf-token":  {},
		"x-xsrf-token":  {},
	}
	// hiddenResponseHeaders are response headers that are never logged.
	hiddenResponseHeaders = map[string]struct{}{
		"set-cookie": {},
	}
)

// LogConfig configures the Log middleware.
type LogConfig struct {
	DefaultLevel     slog.Level
	ClientErrorLevel slog.Level
	ServerErrorLevel slog.Level

	WithUserAgent      bool
	WithRequestBody    bool
	WithRequestHeader  bool
	WithResponseBody   bool
	WithResponseHeader bool
	WithClientIP       bool
}

// DefaultLogConfig returns the default configuration for the Log middleware.
func DefaultLogConfig() LogConfig {
	return LogConfig{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,

		WithUserAgent:      false,
		WithRequestBody:    false,
		WithRequestHeader:  false,
		WithResponseBody:   false,
		WithResponseHeader: false,
		WithClientIP:       true,
	}
}

// Log logs every request and its response using slog. Requests are logged after
// the handler returns, so the response status, latency and size are available.
// The request identifier and trace/span identifiers are included when the
// RequestID and Telemetry middleware run before this one.
type Log struct {
	next   http.Handler
	logger *slog.Logger
	config LogConfig
}

// Ensure Log implements the http.Handler interface.
var _ http.Handler = &Log{}

// NewLogMiddleware creates a Log middleware with the default configuration.
func NewLogMiddleware(next http.Handler, logger *slog.Logger) *Log {
	return NewLogMiddlewareWithConfig(next, logger, DefaultLogConfig())
}

// NewLogMiddlewareWithConfig creates a Log middleware with the given configuration.
func NewLogMiddlewareWithConfig(next http.Handler, logger *slog.Logger, config LogConfig) *Log {
	return &Log{
		next:   next,
		logger: logger,
		config: config,
	}
}

func (m *Log) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	start := time.Now()
	path := r.URL.Path
	query := r.URL.RawQuery

	br := infraHttp.NewRequestReader(r.Body, requestBodyMaxSize, m.config.WithRequestBody)
	r.Body = br

	bw := infraHttp.NewResponseWriter(rw, responseBodyMaxSize, m.config.WithResponseBody)

	defer func() {
		end := time.Now()
		status := bw.Status()
		latency := end.Sub(start)

		requestAttributes := make([]slog.Attr, 0, 10)
		responseAttributes := make([]slog.Attr, 0, 6)
		baseAttributes := make([]slog.Attr, 0, 3)

		requestAttributes = append(requestAttributes,
			slog.Time(requestTimeKey, start.UTC()),
			slog.String(requestMethodKey, r.Method),
			slog.String(requestHostKey, r.Host),
			slog.String(requestPathKey, path),
			slog.String(requestQueryKey, query),
			slog.String(requestRefererKey, r.Referer()),
		)

		if m.config.WithClientIP {
			requestAttributes = append(requestAttributes, slog.String(requestIPKey, infraHttp.ClientIP(r)))
		}

		requestAttributes = append(requestAttributes, slog.Int(requestLengthKey, br.Len()))
		if m.config.WithRequestBody {
			requestAttributes = append(requestAttributes, slog.String(requestBodyKey, string(br.Body())))
		}

		if m.config.WithRequestHeader {
			requestAttributes = append(requestAttributes, slog.Group(requestHeaderKey, headerAttributes(r.Header, hiddenRequestHeaders)...))
		}

		if m.config.WithUserAgent {
			requestAttributes = append(requestAttributes, slog.String(requestUserAgentKey, r.UserAgent()))
		}

		responseAttributes = append(responseAttributes,
			slog.Time(responseTimeKey, end.UTC()),
			slog.Duration(responseLatencyKey, latency),
			slog.Int(responseStatusKey, status),
			slog.Int(responseLengthKey, bw.Len()),
		)

		if m.config.WithResponseBody {
			responseAttributes = append(responseAttributes, slog.String(responseBodyKey, string(bw.Body())))
		}

		if m.config.WithResponseHeader {
			responseAttributes = append(responseAttributes, slog.Group(responseHeaderKey, headerAttributes(rw.Header(), hiddenResponseHeaders)...))
		}

		if requestID := GetRequestID(r); len(requestID) != 0 {
			baseAttributes = append(baseAttributes, slog.String(requestIDKey, requestID))
		}

		baseAttributes = append(baseAttributes, traceAttributes(r.Context())...)

		attributes := append(
			[]slog.Attr{
				{Key: requestGroupKey, Value: slog.GroupValue(requestAttributes...)},
				{Key: responseGroupKey, Value: slog.GroupValue(responseAttributes...)},
			},
			baseAttributes...,
		)

		level := m.config.DefaultLevel
		switch {
		case status >= http.StatusInternalServerError:
			level = m.config.ServerErrorLevel
		case status >= http.StatusBadRequest:
			level = m.config.ClientErrorLevel
		}

		msg := strconv.Itoa(status) + ": " + http.StatusText(status)

		m.logger.LogAttrs(r.Context(), level, msg, attributes...)
	}()

	m.next.ServeHTTP(bw, r)
}

// headerAttributes turns HTTP headers into slog attributes, skipping any header
// present in hidden.
func headerAttributes(header http.Header, hidden map[string]struct{}) []any {
	kv := make([]any, 0, len(header))

	for k, v := range header {
		if _, found := hidden[strings.ToLower(k)]; found {
			continue
		}
		kv = append(kv, slog.Any(k, v))
	}

	return kv
}
