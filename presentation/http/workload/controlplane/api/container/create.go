// Package container is the control plane's containers across Docker VMs.
package container

import (
	"net/http"

	createcontainer "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/createContainer"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type createHandler struct {
	useCase *createcontainer.UseCase
}

func NewCreateHandler(useCase *createcontainer.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// refusal is why dockerd made no container, and the Docker VM it was asked
// of, which was made for it when it says so.
type refusal struct {
	Error *noderequest.Error  `json:"error"`
	VM    *presenter.ChosenVM `json:"vm,omitempty"`
}

// @Summary		Create a container
// @Description	create a container in the docker vm the request names, the owner's only one, or one made for it
// @Tags			workload containers
// @Accept			json
// @Produce		json
// @Param			owner	query		string						true	"Whom the container is created for"
// @Param			body	body		createcontainer.Request		true	"The container, and the docker vm"
// @Success		201		{object}	createcontainer.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		409		{object}	map[string]interface{}
// @Failure		422		{object}	map[string]interface{}
// @Router			/containers [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createcontainer.Request
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
	case response.NodeError != nil:
		respond.JSON(rw, respond.StatusOf(response.NodeError.Code), refusal{Error: response.NodeError, VM: response.VM})
	default:
		respond.JSON(rw, http.StatusCreated, response)
	}
}
