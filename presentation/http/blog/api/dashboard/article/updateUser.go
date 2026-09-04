package article

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	updateuserarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateUserArticle"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type updateUserHandler struct {
	useCase *updateuserarticle.UseCase
}

func NewUpdateUserHandler(useCase *updateuserarticle.UseCase) *updateUserHandler {
	return &updateUserHandler{
		useCase: useCase,
	}
}

// @Summary		Update own article
// @Description	change an article the authenticated user wrote
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param			article	body	updateuserarticle.Request	true	"Article update payload"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/my/articles [put]
func (h *updateUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateuserarticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	request.AuthorUUID = auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
