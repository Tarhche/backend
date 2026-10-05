package volume

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumes"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getVolumes.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getVolumes.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List a Docker VM's volumes
// @Description	the volumes of a Docker VM
// @Tags			dashboard workload volumes
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	getVolumes.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/volumes [get]
// @Router			/dashboard/my/workload/vms/{uuid}/volumes [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVolumes.Request{
		VMUUID:    r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
