package vm

import (
	"net/http"

	restartvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restartVM"
	startvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/startVM"
	stopvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/stopVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

// asked answers a request that asks a VM for something: accepted, refused, or
// not there.
func asked(rw http.ResponseWriter, r *http.Request, validationErrors domain.ValidationErrors, err error) {
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(validationErrors) > 0:
		respond.Refused(rw, validationErrors)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}

type startHandler struct {
	useCase *startvm.UseCase
}

func NewStartHandler(useCase *startvm.UseCase) *startHandler {
	return &startHandler{useCase: useCase}
}

// @Summary		Start a vm
// @Tags			workload vms
// @Param			uuid	path	string	true	"VM UUID"
// @Param			owner	query	string	false	"Only this owner's vm"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/vms/{uuid}/start [post]
func (h *startHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startvm.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}

type stopHandler struct {
	useCase *stopvm.UseCase
}

func NewStopHandler(useCase *stopvm.UseCase) *stopHandler {
	return &stopHandler{useCase: useCase}
}

// @Summary		Stop a vm
// @Tags			workload vms
// @Param			uuid	path	string	true	"VM UUID"
// @Param			owner	query	string	false	"Only this owner's vm"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/vms/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &stopvm.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}

type restartHandler struct {
	useCase *restartvm.UseCase
}

func NewRestartHandler(useCase *restartvm.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart a vm
// @Tags			workload vms
// @Param			uuid	path	string	true	"VM UUID"
// @Param			owner	query	string	false	"Only this owner's vm"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/vms/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartvm.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}
