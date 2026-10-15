package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/createStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createStack.UseCase
}

func NewCreateHandler(useCase *createStack.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Deploy a stack
// @Description	deploy a stack for the caller from its file, which the platform reads and carries out itself, each service a container of its own: YAML in the shape of a compose file, of which it takes services (at least one), volumes and networks at the top, each volume and network with nothing beneath it; and of a service, image, which it needs, command and entrypoint, environment, ports (short or long form), privileged, restart (no or unless-stopped), depends_on (service_started), volumes (named ones the file declares, or binds of absolute paths), networks, pids_limit, user, runtime and resources.limits.cpus and .memory. Anything else, build, healthcheck, deploy, profiles, extends, configs, secrets, x- keys, anchors and aliases, tags but !!str, directives (%YAML, %TAG), and a top-level version or name among it, is refused, and so is everything else wrong with it, field by field under its path in the file, as compose.<path>: compose.services.web.build. The file is 64 KiB at most, in UTF-8; a key is a string, quoted when YAML would read it as anything else, of 1024 characters at most; a name, of a service, a volume or a network, is 63 characters at most; its services name 1000 of what it declares at most, each network a service joins, each named volume it mounts and each service it depends on; and nothing in it nests deeper than 16 levels. It goes in the Docker VM vm_uuid names, in a new one vm describes, or with neither in a new one made with the defaults. Its graph makes it after the answer, which says which VM it goes into and whether that is made for it
// @Tags			dashboard workload stacks
// @Accept			json
// @Produce		json
// @Param			body	body		createStack.Request	true	"Stack"
// @Success		201		{object}	createStack.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createStack.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = workload.Caller(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response)
	}
}
