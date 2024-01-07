package middleware

import (
	"net/http"
)

type RateLimit struct {
}

var _ http.Handler = &RateLimit{}

func NewAuthoriseMiddleware() *RateLimit {
	return &RateLimit{}
}

func (a *RateLimit) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

}
