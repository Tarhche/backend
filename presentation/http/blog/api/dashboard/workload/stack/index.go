package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStacks"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getStacks.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getStacks.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List stacks
// @Description	a page of stacks, without their compose files, in one VM with ?vm=: anybody's on the workload routes, the caller's own on the my routes
// @Tags			dashboard workload stacks
// @Produce		json
// @Param			page	query		int		false	"Page"	default(1)
// @Param			vm		query		string	false	"Only the stacks deployed into this VM"
// @Success		200		{object}	getStacks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/stacks [get]
// @Router			/dashboard/my/workload/stacks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getStacks.Request{
		Page:      workload.Page(r),
		VMUUID:    r.URL.Query().Get("vm"),
		OwnerUUID: h.owner(r),
	})

	if workload.Failed(rw, r, err) {
		return
	}

	workload.JSON(rw, http.StatusOK, response)
}
