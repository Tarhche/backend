package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/createContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createContainer.UseCase
}

func NewCreateHandler(useCase *createContainer.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Create a container
// @Description	create and start a container for the caller, pulling its image first when the VM does not hold it: in the Docker VM vm_uuid names, in a new one vm describes, or with neither in their only Docker VM, or in one made for it when they have none. The answer says which VM it went into and whether it was made for it
// @Tags			dashboard workload containers
// @Accept			json
// @Produce		json
// @Param			body	body		createContainer.Request	true	"Container"
// @Success		201		{object}	createContainer.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Failure		504		{object}	workload.Failure
// @Router			/dashboard/workload/containers [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createContainer.Request
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
