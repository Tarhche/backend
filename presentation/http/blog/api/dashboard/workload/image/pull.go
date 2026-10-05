package image

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/pullImage"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type pullHandler struct {
	useCase *pullImage.UseCase
	owner   workload.Owner
}

func NewPullHandler(useCase *pullImage.UseCase, owner workload.Owner) *pullHandler {
	return &pullHandler{useCase: useCase, owner: owner}
}

// @Summary		Pull an image
// @Description	pull an image into a Docker VM. A pull takes as long as the registry does, and carries on after whoever asked has stopped waiting
// @Tags			dashboard workload images
// @Accept			json
// @Produce		json
// @Param			uuid	path		string				true	"VM UUID"
// @Param			body	body		pullImage.Request	true	"The image"
// @Success		201		{object}	presenter.Image
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Failure		504		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/images [post]
// @Router			/dashboard/my/workload/vms/{uuid}/images [post]
func (h *pullHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request pullImage.Request
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
		workload.JSON(rw, http.StatusCreated, response.Image)
	}
}
