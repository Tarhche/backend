package image

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/getImages"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getImages.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getImages.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List a Docker VM's images
// @Description	the images a Docker VM holds, read from its dockerd as it is now
// @Tags			dashboard workload images
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	getImages.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/images [get]
// @Router			/dashboard/my/workload/vms/{uuid}/images [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getImages.Request{
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
