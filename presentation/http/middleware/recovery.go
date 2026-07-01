package middleware

import (
	"fmt"
	"net/http"
	"runtime"
)

type Recovery struct {
	next http.Handler
}

// Ensure Recovery implements the http.Handler interface
var _ http.Handler = &Recovery{}

func NewRecoveryMiddleware(next http.Handler) *Recovery {
	return &Recovery{
		next: next,
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

			fmt.Printf("panic recovered: %v\n %s", err, buf)
			w.WriteHeader(500)

			_, _ = w.Write([]byte{})
		}
	}()

	r.next.ServeHTTP(w, req)
}
