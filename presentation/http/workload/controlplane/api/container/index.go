package container

import (
	"net/http"

	getcontainers "github.com/khanzadimahdi/testproject/application/workload/controlplane/container/getContainers"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type indexHandler struct {
	useCase *getcontainers.UseCase
}

func NewIndexHandler(useCase *getcontainers.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List containers
// @Description	every container across the running docker vms, each with the vm it is in
// @Tags			workload containers
// @Produce		json
// @Param			owner	query		string	false	"Only this person's docker vms"
// @Param			vm		query		string	false	"Only this docker vm"
// @Success		200		{object}	getcontainers.Response
// @Failure		404		{object}	map[string]interface{}
// @Router			/containers [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getcontainers.Request{OwnerUUID: respond.Owner(r), VMUUID: r.URL.Query().Get("vm")})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
