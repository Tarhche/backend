package language

import (
	"errors"
	"net/http"

	deletelanguage "github.com/khanzadimahdi/testproject/application/dashboard/language/deleteLanguage"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteHandler struct {
	useCase *deletelanguage.UseCase
}

func NewDeleteHandler(useCase *deletelanguage.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete language
// @Description	remove a language by code
// @Tags			dashboard languages
// @Accept			json
// @Produce		json
// @Param			code	path	string	true	"Language code"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/languages/{code} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	request := &deletelanguage.Request{
		Code: code,
	}

	err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
