package websocket

import (
	"net/http"
)

// websocketHandler is the blog API's entry point for the websocket endpoint. It
// keeps the route visible alongside the other api handlers and carries its
// openapi annotation, while the protocol itself lives in the infrastructure
// websocket it wraps.
type websocketHandler struct {
	handler http.Handler
}

// Ensure websocketHandler implements http.Handler.
var _ http.Handler = &websocketHandler{}

func NewWebsocketHandler(handler http.Handler) *websocketHandler {
	return &websocketHandler{
		handler: handler,
	}
}

// @Summary		Websocket endpoint
// @Description	upgrades the connection to a websocket. the client sends requests as {"id", "subject", "payload"} and receives replies as {"request_id", "payload"}; the subject must be one the server consumes.
// @Tags		websocket
// @Success		101	{string}	string	"switching protocols"
// @Failure		400	{object}	map[string]interface{}
// @Failure		503	{string}	string	"the service is shutting down"
// @Router		/ws [get]
func (h *websocketHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(rw, r)
}
