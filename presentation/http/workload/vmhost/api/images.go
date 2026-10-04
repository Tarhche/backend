package api

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteImage"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getImages"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/prepareImage"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type prepareImageHandler struct {
	useCase *prepareImage.UseCase
}

func NewPrepareImageHandler(useCase *prepareImage.UseCase) *prepareImageHandler {
	return &prepareImageHandler{useCase: useCase}
}

// ServeHTTP takes vm.PrepareImage and answers the vm.Image made ready.
func (h *prepareImageHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var body vm.PrepareImage
	if !decode(rw, r, &body) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &prepareImage.Request{Image: body.Image})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.Image)
	}
}

type imagesHandler struct {
	useCase *getImages.UseCase
}

func NewImagesHandler(useCase *getImages.UseCase) *imagesHandler {
	return &imagesHandler{useCase: useCase}
}

// ServeHTTP answers every image kept, as a list.
func (h *imagesHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	if err != nil {
		failed(rw, r, err)

		return
	}

	reply(rw, http.StatusOK, response.Images)
}

type deleteImageHandler struct {
	useCase *deleteImage.UseCase
}

func NewDeleteImageHandler(useCase *deleteImage.UseCase) *deleteImageHandler {
	return &deleteImageHandler{useCase: useCase}
}

// ServeHTTP lets go of the image named by its digest.
func (h *deleteImageHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteImage.Request{Digest: r.PathValue("digest")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}
