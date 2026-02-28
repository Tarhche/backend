package element

import (
	"encoding/json"
	"errors"
	"net/http"

	createElement "github.com/khanzadimahdi/testproject/application/dashboard/element/createElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	useCase *createElement.UseCase
}

func NewCreateHandler(useCase *createElement.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Create element
// @Description	add a new element via dashboard
// @Tags			dashboard elements
// @Accept			json
// @Produce		json
// @Param			body	body		createElement.Request	true	"Element data"
// @Success		201		{object}	createElement.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/elements [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createElement.Request
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
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
