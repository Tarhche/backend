package comment

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/createComment"
)

type createHandler struct {
	useCase *createComment.UseCase
}

func NewCreateHandler(useCase *createComment.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Create dashboard comment
// @Description	post a comment as authenticated user
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			body	body	createComment.Request	true	"Comment data"
// @Success		201
// @Failure		400	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/comments [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request createComment.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID

	response, err := h.useCase.Execute(&request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
