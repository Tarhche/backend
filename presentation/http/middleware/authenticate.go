package middleware

import (
	"net/http"
)

type Authenticate struct {
}

var _ http.Handler = &Authenticate{}

func NewAuthenticateMiddleware() *Authenticate {
	return &Authenticate{}
}

func (a *Authenticate) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

}
