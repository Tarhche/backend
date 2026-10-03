package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getVMLogs"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type vmLogsHandler struct {
	useCase *getVMLogs.UseCase
}

func NewVMLogsHandler(useCase *getVMLogs.UseCase) *vmLogsHandler {
	return &vmLogsHandler{useCase: useCase}
}

// ServeHTTP answers a VM's output as vm.LogLines, one JSON line each: after
// the line numbered QueryAfter, from QuerySince (RFC 3339), and with
// QueryFollow set to "1" what comes as it comes. The answer begins as soon as
// the VM is found — a reader that follows may wait long for the first line —
// so what goes wrong after it began can only end it.
func (h *vmLogsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request, err := logsRequest(r)
	if err != nil {
		failed(rw, r, err)

		return
	}

	controller := http.NewResponseController(rw)
	encoder := json.NewEncoder(rw)
	begun := false

	begin := func() {
		begun = true

		rw.Header().Set("Content-Type", "application/x-ndjson")
		rw.Header().Set("Cache-Control", "no-store")
		rw.WriteHeader(http.StatusOK)

		_ = controller.Flush()
	}

	emit := func(line vm.LogLine) error {
		if err := encoder.Encode(line); err != nil {
			return err
		}

		if request.Follow {
			return controller.Flush()
		}

		return nil
	}

	response, err := h.useCase.Execute(r.Context(), &request, begin, emit)

	switch {
	case err != nil && !begun:
		failed(rw, r, err)
	case err != nil:
		// cut short after it began: the reader sees it end, and asks again
		// after the last line it has.
	case len(response.ValidationErrors) > 0:
		refused(rw, r, response.ValidationErrors)
	}
}

// logsRequest reads what output is asked for off the query.
func logsRequest(r *http.Request) (getVMLogs.Request, error) {
	query := r.URL.Query()

	request := getVMLogs.Request{ID: r.PathValue("id")}

	if after := query.Get(vm.QueryAfter); len(after) > 0 {
		parsed, err := strconv.ParseUint(after, 10, 64)
		if err != nil {
			return request, fmt.Errorf("%w: %s is not a line number", vm.ErrInvalid, vm.QueryAfter)
		}

		request.After = parsed
	}

	if since := query.Get(vm.QuerySince); len(since) > 0 {
		parsed, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			return request, fmt.Errorf("%w: %s is not a time written as RFC 3339", vm.ErrInvalid, vm.QuerySince)
		}

		request.Since = parsed
	}

	switch follow := query.Get(vm.QueryFollow); follow {
	case "", "0", "false":
	case "1", "true":
		request.Follow = true
	default:
		return request, fmt.Errorf("%w: %s is 1 or nothing", vm.ErrInvalid, vm.QueryFollow)
	}

	return request, nil
}
