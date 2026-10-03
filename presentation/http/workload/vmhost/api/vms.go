package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/createVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/deleteVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMStats"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMs"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/killVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/restartVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/startVM"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/stopVM"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type createVMHandler struct {
	useCase *createVM.UseCase
}

func NewCreateVMHandler(useCase *createVM.UseCase) *createVMHandler {
	return &createVMHandler{useCase: useCase}
}

// ServeHTTP takes a vm.Spec and answers vm.Created: the VM is made, and
// nothing is booted.
func (h *createVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createVM.Request
	if !decode(rw, r, &request.Spec) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusCreated, vm.Created{ID: response.ID})
	}
}

type vmsHandler struct {
	useCase *getVMs.UseCase
}

func NewVMsHandler(useCase *getVMs.UseCase) *vmsHandler {
	return &vmsHandler{useCase: useCase}
}

// ServeHTTP answers every VM carrying every label filter given, as a list.
func (h *vmsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMs.Request{Labels: r.URL.Query()[vm.QueryLabel]})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.VMs)
	}
}

type vmHandler struct {
	useCase *getVM.UseCase
}

func NewVMHandler(useCase *getVM.UseCase) *vmHandler {
	return &vmHandler{useCase: useCase}
}

// ServeHTTP answers one vm.VM.
func (h *vmHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVM.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.VM)
	}
}

type startVMHandler struct {
	useCase *startVM.UseCase
}

func NewStartVMHandler(useCase *startVM.UseCase) *startVMHandler {
	return &startVMHandler{useCase: useCase}
}

// ServeHTTP boots a VM and starts its task, answering once it runs.
func (h *startVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startVM.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}

type stopVMHandler struct {
	useCase *stopVM.UseCase
}

func NewStopVMHandler(useCase *stopVM.UseCase) *stopVMHandler {
	return &stopVMHandler{useCase: useCase}
}

// ServeHTTP stops a VM's task, giving it the timeout asked for (QueryTimeout,
// a Go duration) or vm.DefaultStopTimeout to end on its own, and answers once
// its machine is gone.
func (h *stopVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	timeout := vm.DefaultStopTimeout

	if asked := r.URL.Query().Get(vm.QueryTimeout); len(asked) > 0 {
		parsed, err := time.ParseDuration(asked)
		if err != nil {
			failed(rw, r, fmt.Errorf("%w: %s is not a duration: %w", vm.ErrInvalid, vm.QueryTimeout, err))

			return
		}

		timeout = parsed
	}

	response, err := h.useCase.Execute(r.Context(), &stopVM.Request{ID: r.PathValue("id"), Timeout: timeout})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}

type restartVMHandler struct {
	useCase *restartVM.UseCase
}

func NewRestartVMHandler(useCase *restartVM.UseCase) *restartVMHandler {
	return &restartVMHandler{useCase: useCase}
}

// ServeHTTP stops a VM's task and starts it again from the same disks,
// answering once it runs again.
func (h *restartVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartVM.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}

type killVMHandler struct {
	useCase *killVM.UseCase
}

func NewKillVMHandler(useCase *killVM.UseCase) *killVMHandler {
	return &killVMHandler{useCase: useCase}
}

// ServeHTTP kills a VM's task, answering once its machine is gone.
func (h *killVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &killVM.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}

type deleteVMHandler struct {
	useCase *deleteVM.UseCase
}

func NewDeleteVMHandler(useCase *deleteVM.UseCase) *deleteVMHandler {
	return &deleteVMHandler{useCase: useCase}
}

// ServeHTTP takes a VM away, and everything kept for it.
func (h *deleteVMHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteVM.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}

type vmStatsHandler struct {
	useCase *getVMStats.UseCase
}

func NewVMStatsHandler(useCase *getVMStats.UseCase) *vmStatsHandler {
	return &vmStatsHandler{useCase: useCase}
}

// ServeHTTP answers what a VM uses: vm.Stats.
func (h *vmStatsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMStats.Request{ID: r.PathValue("id")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.Stats)
	}
}
