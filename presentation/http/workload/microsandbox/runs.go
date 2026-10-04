package microsandbox

import (
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// infoHandler answers what the service says about itself.
type infoHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &infoHandler{}

func NewInfoHandler(supervisor *runs.Supervisor) *infoHandler {
	return &infoHandler{supervisor: supervisor}
}

func (h *infoHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, h.supervisor.Info())
}

// listRunsHandler answers a node's runs.
type listRunsHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &listRunsHandler{}

func NewListRunsHandler(supervisor *runs.Supervisor) *listRunsHandler {
	return &listRunsHandler{supervisor: supervisor}
}

func (h *listRunsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	listed, err := h.supervisor.List(query.Get(api.QueryNode), query.Get(api.QueryTask), query.Get(api.QuerySlug))
	if err != nil {
		writeError(rw, r, err)

		return
	}

	writeJSON(rw, http.StatusOK, api.RunList{Runs: listed})
}

// createRunHandler records a run.
type createRunHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &createRunHandler{}

func NewCreateRunHandler(supervisor *runs.Supervisor) *createRunHandler {
	return &createRunHandler{supervisor: supervisor}
}

func (h *createRunHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var spec api.RunSpec
	if err := decode(rw, r, &spec, false); err != nil {
		writeError(rw, r, err)

		return
	}

	run, err := h.supervisor.Create(r.Context(), spec)
	if err != nil {
		writeError(rw, r, err)

		return
	}

	writeJSON(rw, http.StatusCreated, run)
}

// getRunHandler answers one run.
type getRunHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &getRunHandler{}

func NewGetRunHandler(supervisor *runs.Supervisor) *getRunHandler {
	return &getRunHandler{supervisor: supervisor}
}

func (h *getRunHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	run, err := h.supervisor.Get(r.PathValue(api.WildcardRun))
	if err != nil {
		writeError(rw, r, err)

		return
	}

	writeJSON(rw, http.StatusOK, run)
}

// deleteRunHandler forgets a run, killing it first if it is up.
type deleteRunHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &deleteRunHandler{}

func NewDeleteRunHandler(supervisor *runs.Supervisor) *deleteRunHandler {
	return &deleteRunHandler{supervisor: supervisor}
}

func (h *deleteRunHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if err := h.supervisor.Delete(r.Context(), r.PathValue(api.WildcardRun)); err != nil {
		writeError(rw, r, err)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// LifecycleHandlers start, stop, kill and restart runs. Each answers the run
// as the change left it.
type LifecycleHandlers struct {
	Start   http.Handler
	Stop    http.Handler
	Kill    http.Handler
	Restart http.Handler
}

func NewLifecycleHandlers(supervisor *runs.Supervisor) LifecycleHandlers {
	return LifecycleHandlers{
		Start: lifecycleHandler(func(r *http.Request, id string, _ time.Duration) (api.Run, error) {
			return supervisor.Start(r.Context(), id)
		}, false),
		Stop: lifecycleHandler(func(r *http.Request, id string, timeout time.Duration) (api.Run, error) {
			return supervisor.Stop(r.Context(), id, timeout)
		}, true),
		Kill: lifecycleHandler(func(r *http.Request, id string, _ time.Duration) (api.Run, error) {
			return supervisor.Kill(r.Context(), id)
		}, false),
		Restart: lifecycleHandler(func(r *http.Request, id string, timeout time.Duration) (api.Run, error) {
			return supervisor.Restart(r.Context(), id, timeout)
		}, true),
	}
}

// lifecycleHandler changes a run's life. One that stops it reads the stop's
// timeout from its optional body.
func lifecycleHandler(change func(r *http.Request, id string, timeout time.Duration) (api.Run, error), stops bool) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var timeout time.Duration

		if stops {
			var request api.StopRequest
			if err := decode(rw, r, &request, true); err != nil {
				writeError(rw, r, err)

				return
			}

			if request.TimeoutSeconds < 0 {
				writeError(rw, r, &api.Error{Code: api.CodeInvalid, Message: "timeout_seconds cannot be negative"})

				return
			}

			timeout = time.Duration(request.TimeoutSeconds) * time.Second
		}

		run, err := change(r, r.PathValue(api.WildcardRun), timeout)
		if err != nil {
			writeError(rw, r, err)

			return
		}

		writeJSON(rw, http.StatusOK, run)
	}
}

// runStatsHandler answers what a running run is using.
type runStatsHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &runStatsHandler{}

func NewRunStatsHandler(supervisor *runs.Supervisor) *runStatsHandler {
	return &runStatsHandler{supervisor: supervisor}
}

func (h *runStatsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	stats, err := h.supervisor.Stats(r.Context(), r.PathValue(api.WildcardRun))
	if err != nil {
		writeError(rw, r, err)

		return
	}

	writeJSON(rw, http.StatusOK, stats)
}

// nodeStatsHandler answers what a node's running runs are using between them.
type nodeStatsHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &nodeStatsHandler{}

func NewNodeStatsHandler(supervisor *runs.Supervisor) *nodeStatsHandler {
	return &nodeStatsHandler{supervisor: supervisor}
}

func (h *nodeStatsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	stats, err := h.supervisor.NodeStats(r.Context(), r.URL.Query().Get(api.QueryNode))
	if err != nil {
		writeError(rw, r, err)

		return
	}

	writeJSON(rw, http.StatusOK, stats)
}

// pullHandler makes sure an image is cached.
type pullHandler struct {
	supervisor *runs.Supervisor
}

var _ http.Handler = &pullHandler{}

func NewPullHandler(supervisor *runs.Supervisor) *pullHandler {
	return &pullHandler{supervisor: supervisor}
}

func (h *pullHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request api.PullRequest
	if err := decode(rw, r, &request, false); err != nil {
		writeError(rw, r, err)

		return
	}

	if err := h.supervisor.Pull(r.Context(), request.Reference); err != nil {
		writeError(rw, r, err)

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
