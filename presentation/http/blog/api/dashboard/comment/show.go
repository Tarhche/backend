package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getComment"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getComment.UseCase
}

func NewShowHandler(useCase *getComment.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get comment
// @Description	retrieve comment by UUID
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Comment UUID"
// @Success		200		{object}	getComment.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/comments/{uuid} [get]
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
