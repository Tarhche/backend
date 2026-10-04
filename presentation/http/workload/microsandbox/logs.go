package microsandbox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// logsHandler streams a run's log as newline-delimited JSON, one LogLine on
// each line, flushed as each is written, and follows it while the run is up
// when it is asked to.
type logsHandler struct {
	supervisor *runs.Supervisor
	logger     *slog.Logger
}

var _ http.Handler = &logsHandler{}

func NewLogsHandler(supervisor *runs.Supervisor, logger *slog.Logger) *logsHandler {
	return &logsHandler{supervisor: supervisor, logger: logger}
}

func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	id := r.PathValue(api.WildcardRun)
	query := r.URL.Query()

	var since time.Time

	if value := query.Get(api.QuerySince); len(value) > 0 {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			writeError(rw, r, &api.Error{Code: api.CodeInvalid, Message: fmt.Sprintf("since %q is not a time in RFC 3339", value)})

			return
		}

		since = parsed
	}

	follow := query.Get(api.QueryFollow) == "true"

	// a run that is not there is answered as one, before the stream begins:
	// once it has, the status has been sent.
	if _, err := h.supervisor.Get(id); err != nil {
		writeError(rw, r, err)

		return
	}

	controller := http.NewResponseController(rw)

	// a log being followed is open for as long as the run is up, which no
	// write deadline the server has can know.
	_ = controller.SetWriteDeadline(time.Time{})

	rw.Header().Set("Content-Type", api.ContentTypeNDJSON)
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.WriteHeader(http.StatusOK)

	_ = controller.Flush()

	encoder := json.NewEncoder(rw)
	encoder.SetEscapeHTML(false)

	err := h.supervisor.Logs(r.Context(), id, since, follow, func(line api.LogLine) error {
		if err := encoder.Encode(line); err != nil {
			return err
		}

		return controller.Flush()
	})

	// the status has been sent, so all that is left is to end the stream,
	// which a client reads as the log having been cut short.
	if err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "a run's log stream was cut short", "run", id, "error", err)
	}
}
