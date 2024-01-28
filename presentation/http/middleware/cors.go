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

	a.next.ServeHTTP(rw, r)
}
