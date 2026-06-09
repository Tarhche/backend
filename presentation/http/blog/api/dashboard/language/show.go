package language

import (
	"encoding/json"
	"errors"
	"net/http"

	getlanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguage"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getlanguage.UseCase
}

func NewShowHandler(useCase *getlanguage.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get language
// @Description	fetch language by code
// @Tags			dashboard languages
// @Accept			json
// @Produce		json
// @Param			code	path		string	true	"Language code"
// @Success		200		{object}	getlanguage.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/languages/{code} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	response, err := h.useCase.Execute(code)

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
