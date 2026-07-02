package node

import (
	"encoding/json"
	"errors"
	"net/http"

	getnode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getnode.UseCase
}

func NewShowHandler(useCase *getnode.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get node
// @Description	retrieve information about a runner node
// @Tags			runner nodes
// @Accept			json
// @Produce		json
// @Param			name	path		string	true	"Node name"
// @Success		200		{object}	getnode.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/nodes/{name} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := getnode.Request{
		Name: r.PathValue("name"),
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
