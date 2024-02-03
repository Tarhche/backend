package element

import (
	"encoding/json"
	"errors"
	"net/http"

	updateElement "github.com/khanzadimahdi/testproject/application/dashboard/element/updateElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateHandler struct {
	updateElementUseCase *updateElement.UseCase
}

func NewUpdateHandler(updateElementUseCase *updateElement.UseCase) *updateHandler {
	return &updateHandler{
		updateElementUseCase: updateElementUseCase,
	}
}

func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateElement.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.updateElementUseCase.UpdateElement(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
