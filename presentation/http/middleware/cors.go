package middleware

import (
	"net/http"
)

type CORS struct {
	next http.Handler
}

var _ http.Handler = &CORS{}

func NewCORSMiddleware(next http.Handler) *CORS {
	return &CORS{
		next: next,
	}
}

func (a *CORS) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Access-Control-Allow-Origin", "*")
	rw.Header().Set("Access-Control-Allow-Credentials", "true")
	rw.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	rw.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Language-Code, Mcp-Session-Id, Mcp-Protocol-Version, Mcp-Method, Mcp-Name, Last-Event-ID")

	// the session id is a response header an MCP client has to be able to read
	// back, and a browser hands over only what it is told it may.
	rw.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id, WWW-Authenticate")
	rw.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method == http.MethodOptions {
		rw.WriteHeader(http.StatusNoContent)

		return
	}

	a.next.ServeHTTP(rw, r)
}
