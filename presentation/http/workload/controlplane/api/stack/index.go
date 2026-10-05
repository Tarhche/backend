package stack

import (
	"net/http"

	getstacks "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/getStacks"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type indexHandler struct {
	useCase *getstacks.UseCase
}

func NewIndexHandler(useCase *getstacks.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List stacks
// @Tags			workload stacks
// @Produce		json
// @Param			owner	query		string	false	"Only the stacks this person owns"
// @Param			vm		query		string	false	"Only the stacks in this vm"
// @Param			page	query		int		false	"Page number"	default(1)
// @Success		200		{object}	getstacks.Response
// @Router			/stacks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getstacks.Request{
		OwnerUUID: respond.Owner(r),
		VMUUID:    r.URL.Query().Get("vm"),
		Page:      respond.Page(r),
	})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
