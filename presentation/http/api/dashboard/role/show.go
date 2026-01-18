package role

import (
	"encoding/json"
	"errors"
	"net/http"

	getrole "github.com/khanzadimahdi/testproject/application/dashboard/role/getRole"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getrole.UseCase
}

func NewShowHandler(useCase *getrole.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(UUID)

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
