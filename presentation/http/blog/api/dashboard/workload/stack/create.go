package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createStack.UseCase
}

func NewCreateHandler(useCase *createStack.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Deploy a stack
// @Description	deploy a compose project for the caller: in the Docker VM vm_uuid names, in a new one vm describes, or with neither in their only Docker VM, or in one made for it when they have none. The deploy happens after the answer, which says which VM it went into and whether it was made for it
// @Tags			dashboard workload stacks
// @Accept			json
// @Produce		json
// @Param			body	body		createStack.Request	true	"Stack"
// @Success		201		{object}	createStack.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createStack.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = workload.Caller(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response)
	}
}
