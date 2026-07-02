package middleware

import (
	"log/slog"
	"net/http"
	"runtime"
)

type Recovery struct {
	next   http.Handler
	logger *slog.Logger
}

// Ensure Recovery implements the http.Handler interface
var _ http.Handler = &Recovery{}

func NewRecoveryMiddleware(next http.Handler, logger *slog.Logger) *Recovery {
	return &Recovery{
		next:   next,
		logger: logger,
	}
}

func (r *Recovery) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() {
		err := recover()
		if err == http.ErrAbortHandler {
			panic(err)
		}

		if err != nil {
			buf := make([]byte, 2048)
			n := runtime.Stack(buf, false)
			buf = buf[:n]

			r.logger.ErrorContext(req.Context(), "panic recovered", slog.Any("error", err), slog.String("stack", string(buf)))
			w.WriteHeader(500)

			_, _ = w.Write([]byte{})
		}
	}()

	r.next.ServeHTTP(w, req)
}
