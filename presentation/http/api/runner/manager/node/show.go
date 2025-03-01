package node

import (
	"encoding/json"
	"errors"
	"net/http"

	getnode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getnode.UseCase
}

func NewShowHandler(useCase *getnode.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := getnode.Request{
		Name: r.PathValue("name"),
	}

	response, err := h.useCase.Execute(&request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
