package element

import (
	"encoding/json"
	"errors"
	"net/http"

	createElement "github.com/khanzadimahdi/testproject/application/dashboard/element/createElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	createElementUseCase *createElement.UseCase
}

func NewCreateHandler(createElementUseCase *createElement.UseCase) *createHandler {
	return &createHandler{
		createElementUseCase: createElementUseCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createElement.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.createElementUseCase.CreateElement(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
