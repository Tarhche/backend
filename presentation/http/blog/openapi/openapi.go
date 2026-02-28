package openapi

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	// import the openapi docs
	_ "github.com/khanzadimahdi/testproject/resources/docs/blog/openapi"
)

type OpenAPIHandler struct {
	openAPI http.Handler
}

// Ensure OpenAPIHandler implements http.Handler.
var _ http.Handler = &OpenAPIHandler{}

func NewOpenAPIHandler() *OpenAPIHandler {
	return &OpenAPIHandler{
		openAPI: httpSwagger.Handler(),
	}
}

func (h *OpenAPIHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.openAPI.ServeHTTP(rw, r)
}
