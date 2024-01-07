package middleware

import (
	"net/http"
)

type Authorize struct {
}

var _ http.Handler = &Authorize{}

func NewAuthorizeMiddleware() *Authorize {
	return &Authorize{}
}

func (a *Authorize) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

}
