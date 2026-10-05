package stack

import (
	"net/http"

	restartstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/restartStack"
	startstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/startStack"
	stopstack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/stopStack"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

// asked answers a request that asks a stack for something: accepted, refused,
// or not there.
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
	useCase *startstack.UseCase
}

func NewStartHandler(useCase *startstack.UseCase) *startHandler {
	return &startHandler{useCase: useCase}
}

// @Summary		Start a stack
// @Tags			workload stacks
// @Param			uuid	path	string	true	"Stack UUID"
// @Param			owner	query	string	false	"Only this owner's stack"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/stacks/{uuid}/start [post]
func (h *startHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startstack.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}

type stopHandler struct {
	useCase *stopstack.UseCase
}

func NewStopHandler(useCase *stopstack.UseCase) *stopHandler {
	return &stopHandler{useCase: useCase}
}

// @Summary		Stop a stack
// @Tags			workload stacks
// @Param			uuid	path	string	true	"Stack UUID"
// @Param			owner	query	string	false	"Only this owner's stack"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/stacks/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &stopstack.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}

type restartHandler struct {
	useCase *restartstack.UseCase
}

func NewRestartHandler(useCase *restartstack.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart a stack
// @Tags			workload stacks
// @Param			uuid	path	string	true	"Stack UUID"
// @Param			owner	query	string	false	"Only this owner's stack"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/stacks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartstack.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}
