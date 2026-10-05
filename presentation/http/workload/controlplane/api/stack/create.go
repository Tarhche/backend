package stack

import (
	"net/http"

	createstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/createStack"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type createHandler struct {
	useCase *createstack.UseCase
}

func NewCreateHandler(useCase *createstack.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Deploy a stack
// @Description	deploy a compose project into the docker vm the request names, the owner's only one, or one made for it
// @Tags			workload stacks
// @Accept			json
// @Produce		json
// @Param			owner	query		string				true	"Whom the stack is deployed for"
// @Param			body	body		createstack.Request	true	"The compose project, and the docker vm"
// @Success		201		{object}	createstack.Response
// @Failure		400		{object}	map[string]interface{}
// @Router			/stacks [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createstack.Request
	if !respond.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = respond.Owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respond.JSON(rw, http.StatusCreated, response)
	}
}
