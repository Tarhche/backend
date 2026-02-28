package element

import (
	"encoding/json"
	"errors"
	"net/http"

	updateElement "github.com/khanzadimahdi/testproject/application/dashboard/element/updateElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateHandler struct {
	useCase *updateElement.UseCase
}

func NewUpdateHandler(useCase *updateElement.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

// @Summary		Update element
// @Description	modify existing element data
// @Tags			dashboard elements
// @Accept			json
// @Produce		json
// @Param			body	body	updateElement.Request	true	"Element update"
// @Success		200
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/elements [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateElement.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
