package websocket

import (
	"net/http"
)

type websocket struct {
	handler http.Handler
}

func NewWebsocket(handler http.Handler) *websocket {
	return &websocket{
		handler: handler,
	}
}

func (w *websocket) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.handler.ServeHTTP(rw, r)
}
