package volume

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/createVolume"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createVolume.UseCase
	owner   workload.Owner
}

func NewCreateHandler(useCase *createVolume.UseCase, owner workload.Owner) *createHandler {
	return &createHandler{useCase: useCase, owner: owner}
}

// @Summary		Create a volume
// @Description	create a volume in a Docker VM, on its disk, for its containers to keep what they write in
// @Tags			dashboard workload volumes
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"VM UUID"
// @Param			body	body		createVolume.Request	true	"Volume"
// @Success		201		{object}	presenter.Volume
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/volumes [post]
// @Router			/dashboard/my/workload/vms/{uuid}/volumes [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createVolume.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.VMUUID = r.PathValue("uuid")
	request.OwnerUUID = h.owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response.Volume)
	}
}
