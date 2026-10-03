package api

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/removeNetwork"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type ensureNetworkHandler struct {
	useCase *ensureNetwork.UseCase
}

func NewEnsureNetworkHandler(useCase *ensureNetwork.UseCase) *ensureNetworkHandler {
	return &ensureNetworkHandler{useCase: useCase}
}

// ServeHTTP takes vm.NetworkSpec and answers the vm.Network there is now.
func (h *ensureNetworkHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var body vm.NetworkSpec
	if !decode(rw, r, &body) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &ensureNetwork.Request{Name: r.PathValue("name"), Masquerade: body.Masquerade})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		reply(rw, http.StatusOK, response.Network)
	}
}

type removeNetworkHandler struct {
	useCase *removeNetwork.UseCase
}

func NewRemoveNetworkHandler(useCase *removeNetwork.UseCase) *removeNetworkHandler {
	return &removeNetworkHandler{useCase: useCase}
}

// ServeHTTP takes a network away, or answers vm.ErrNetworkInUse.
func (h *removeNetworkHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &removeNetwork.Request{Name: r.PathValue("name")})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	default:
		done(rw)
	}
}
