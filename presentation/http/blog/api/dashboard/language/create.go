package language

import (
	"encoding/json"
	"errors"
	"net/http"

	createlanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/createLanguage"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	useCase *createlanguage.UseCase
}

func NewCreateHandler(useCase *createlanguage.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Create language
// @Description	add a new language via dashboard
// @Tags			dashboard languages
// @Accept			json
// @Produce		json
// @Param			body	body		createlanguage.Request	true	"Language data"
// @Success		201		{object}	createlanguage.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/languages [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createlanguage.Request
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
