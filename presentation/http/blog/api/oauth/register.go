package oauth

import (
	"encoding/json"
	"net/http"

	registerclient "github.com/khanzadimahdi/testproject/application/oauth/registerClient"
)

type registerHandler struct {
	useCase *registerclient.UseCase
}

func NewRegisterHandler(useCase *registerclient.UseCase) *registerHandler {
	return &registerHandler{
		useCase: useCase,
	}
}

// @Summary		Register an application
// @Description	registers an OAuth client, which is how an MCP client introduces itself before anybody approves it
// @Tags		oauth
// @Accept		json
// @Produce		json
// @Param		body	body		registerclient.Request	true	"Client metadata"
// @Success		201		{object}	registerclient.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router		/oauth/register [post]
func (h *registerHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request registerclient.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(rw, http.StatusBadRequest, errorResponse{
			Error:            "invalid_client_metadata",
			ErrorDescription: "the registration is not valid json",
		})

		return
	}

	response, err := h.useCase.Execute(r.Context(), &request)
	if err != nil {
		fail(rw, r, err)

		return
	}

	write(rw, http.StatusCreated, response)
}
