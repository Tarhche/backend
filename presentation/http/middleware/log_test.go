package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultLogConfig(t *testing.T) {
	t.Run("returns default configuration", func(t *testing.T) {
		config := DefaultLogConfig()

		assert.Equal(t, slog.LevelInfo, config.DefaultLevel)
		assert.Equal(t, slog.LevelWarn, config.ClientErrorLevel)
		assert.Equal(t, slog.LevelError, config.ServerErrorLevel)
		assert.False(t, config.WithUserAgent)
		assert.False(t, config.WithRequestBody)
		assert.False(t, config.WithRequestHeader)
		assert.False(t, config.WithResponseBody)
		assert.False(t, config.WithResponseHeader)
		assert.True(t, config.WithClientIP)
	})
}

func TestNewLogMiddleware(t *testing.T) {
	t.Run("creates middleware with default config", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		m := NewLogMiddleware(next, logger)

		assert.NotNil(t, m)
		assert.NotNil(t, m.next)
		assert.Equal(t, logger, m.logger)
		assert.Equal(t, DefaultLogConfig(), m.config)
	})
}

func TestNewLogMiddlewareWithConfig(t *testing.T) {
	t.Run("creates middleware with custom config", func(t *testing.T) {
		config := LogConfig{
			DefaultLevel:     slog.LevelDebug,
			ClientErrorLevel: slog.LevelInfo,
			ServerErrorLevel: slog.LevelWarn,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		m := NewLogMiddlewareWithConfig(next, logger, config)

		assert.NotNil(t, m)
		assert.Equal(t, config, m.config)
	})
}

func TestLogMiddlewareServeHTTP(t *testing.T) {
	t.Run("calls next handler", func(t *testing.T) {
		handlerCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.True(t, handlerCalled)
	})

	t.Run("logs request and response", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "200")
	})

	t.Run("allows handler to write response", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("created"))
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		assert.Equal(t, http.StatusCreated, res.Code)
		assert.Equal(t, "created", res.Body.String())
	})
}

func TestLogMiddlewareLogLevels(t *testing.T) {
	t.Run("uses DefaultLevel for 2xx status", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			DefaultLevel:     slog.LevelInfo,
			ClientErrorLevel: slog.LevelWarn,
			ServerErrorLevel: slog.LevelError,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "200")
	})

	t.Run("uses ClientErrorLevel for 4xx status", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		config := LogConfig{
			DefaultLevel:     slog.LevelInfo,
			ClientErrorLevel: slog.LevelWarn,
			ServerErrorLevel: slog.LevelError,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "404")
	})

	t.Run("uses ServerErrorLevel for 5xx status", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		config := LogConfig{
			DefaultLevel:     slog.LevelInfo,
			ClientErrorLevel: slog.LevelWarn,
			ServerErrorLevel: slog.LevelError,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelError}))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "500")
	})
}

func TestLogMiddlewareRequestAttributes(t *testing.T) {
	t.Run("logs request method", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "POST")
	})

	t.Run("logs request path", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "/api/users")
	})

	t.Run("logs client IP when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithClientIP: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:8080"
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "192.168.1.1")
	})

	t.Run("omits client IP when disabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithClientIP: false,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:8080"
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "192.168.1.1")
	})

	t.Run("logs user agent when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithUserAgent: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "Mozilla/5.0")
	})

	t.Run("omits user agent when disabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithUserAgent: false,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "Mozilla/5.0")
	})
}

func TestLogMiddlewareBodyLogging(t *testing.T) {
	t.Run("records request body length when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithRequestBody: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("test body"))
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		// Verify request length is recorded (body length tracking works)
		assert.Contains(t, logOutput, "request.length")
	})

	t.Run("omits request body when disabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithRequestBody: false,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("secret data"))
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "secret data")
	})

	t.Run("logs response body when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response data"))
		})
		config := LogConfig{
			WithResponseBody: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "response data")
	})

	t.Run("omits response body when disabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("secret response"))
		})
		config := LogConfig{
			WithResponseBody: false,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "secret response")
	})
}

func TestLogMiddlewareHeaderLogging(t *testing.T) {
	t.Run("logs request headers when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithRequestHeader: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Custom-Header", "value")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "X-Custom-Header")
	})

	t.Run("hides authorization header", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithRequestHeader: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "secret-token")
		assert.NotContains(t, logOutput, "Authorization")
	})

	t.Run("hides cookie header", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithRequestHeader: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Cookie", "session=secret")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "session=secret")
		assert.NotContains(t, logOutput, "Cookie")
	})

	t.Run("logs response headers when enabled", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Response-Header", "response-value")
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithResponseHeader: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "X-Response-Header")
	})

	t.Run("hides set-cookie response header", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Set-Cookie", "secret=value")
			w.WriteHeader(http.StatusOK)
		})
		config := LogConfig{
			WithResponseHeader: true,
		}
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddlewareWithConfig(next, logger, config)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.NotContains(t, logOutput, "secret=value")
		assert.NotContains(t, logOutput, "Set-Cookie")
	})
}

func TestLogMiddlewareQueryParameters(t *testing.T) {
	t.Run("logs query parameters", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/search?q=test&limit=10", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "q=test")
	})
}

func TestLogMiddlewareRequestID(t *testing.T) {
	t.Run("logs request ID when available", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		ctx := context.WithValue(context.Background(), requestIDCtxKey, "test-request-id-123")
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "test-request-id-123")
	})

	t.Run("handles missing request ID", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		// Should not panic and should handle gracefully
		assert.NotEmpty(t, buf.String())
	})
}

func TestLogMiddlewareStatusMessages(t *testing.T) {
	tests := []struct {
		status   int
		contains string
	}{
		{http.StatusOK, "200"},
		{http.StatusCreated, "201"},
		{http.StatusBadRequest, "400"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusInternalServerError, "500"},
		{http.StatusBadGateway, "502"},
	}

	for _, tt := range tests {
		t.Run("logs status "+tt.contains, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			buf := &bytes.Buffer{}
			logger := slog.New(slog.NewTextHandler(buf, nil))
			m := NewLogMiddleware(next, logger)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			res := httptest.NewRecorder()

			m.ServeHTTP(res, req)

			logOutput := buf.String()
			assert.Contains(t, logOutput, tt.contains)
		})
	}
}

func TestLogMiddlewareImplementsHandler(t *testing.T) {
	t.Run("Log implements http.Handler", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), logger)
		var _ http.Handler = m
	})
}

func TestLogMiddlewareHost(t *testing.T) {
	t.Run("logs request host", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "example.com"
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "example.com")
	})
}

func TestLogMiddlewareReferer(t *testing.T) {
	t.Run("logs referer header", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/page", nil)
		req.Header.Set("Referer", "https://example.com")
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "https://example.com")
	})
}

func TestLogMiddlewareResponseLength(t *testing.T) {
	t.Run("tracks response body size", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("response"))
		})
		buf := &bytes.Buffer{}
		logger := slog.New(slog.NewTextHandler(buf, nil))
		m := NewLogMiddleware(next, logger)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		res := httptest.NewRecorder()

		m.ServeHTTP(res, req)

		logOutput := buf.String()
		assert.Contains(t, logOutput, "8") // "response" is 8 bytes
	})
}
