package stack

import (
	"net/http"

	getstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/getStack"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type showHandler struct {
	useCase *getstack.UseCase
}

func NewShowHandler(useCase *getstack.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get a stack
// @Description	a stack and the containers compose made for it, read from its vm as they are now
// @Tags			workload stacks
// @Produce		json
// @Param			uuid	path		string	true	"Stack UUID"
// @Param			owner	query		string	false	"Only this owner's stack"
// @Success		200		{object}	getstack.Response
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getstack.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
