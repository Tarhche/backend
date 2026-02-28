package task

import (
	"encoding/json"
	"net/http"

	runTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
)

const (
	DefaultMaxDiskSize   = 200 << 20 // 200 MB
	DefaultMaxMemorySize = 10 << 20  // 10 MB
	DefaultMaxCpu        = 0.05
)

type runHandler struct {
	useCase *runTask.UseCase
}

func NewRunHandler(useCase *runTask.UseCase) *runHandler {
	return &runHandler{
		useCase: useCase,
	}
}

// @Summary		Run task
// @Description	schedule a new task execution
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			body	body		runTask.Request	true	"Task request"
// @Success		201		{object}	runTask.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/run [post]
func (h *runHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request runTask.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	if request.ResourceLimits.Disk == 0 {
		request.ResourceLimits.Disk = DefaultMaxDiskSize
	}

	if request.ResourceLimits.Memory == 0 {
		request.ResourceLimits.Memory = DefaultMaxMemorySize
	}

	if request.ResourceLimits.Cpu == 0 {
		request.ResourceLimits.Cpu = DefaultMaxCpu
	}

	request.OwnerUUID = "guest"

	response, err := h.useCase.Execute(&request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
