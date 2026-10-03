package api

import (
	"log/slog"
	"net/http"

	"github.com/khanzadimahdi/testproject/presentation/http/middleware"
)

// NewHandler is vmhost's API as it is served: its routes behind what every
// service's handler is behind — recovery from a panic, a request ID, a span,
// and a log line for every request. A request that went well is logged at
// debug, since an orchestrator asks vmhost what it holds several times a
// second; one that did not is logged as every service logs one.
func NewHandler(useCases UseCases, logger *slog.Logger) http.Handler {
	logConfig := middleware.DefaultLogConfig()
	logConfig.DefaultLevel = slog.LevelDebug

	return middleware.NewRecoveryMiddleware(
		middleware.NewRequestIDMiddleware(
			middleware.NewTelemetryMiddleware(
				"/workload/vmhost",
				middleware.NewLogMiddlewareWithConfig(NewRouter(useCases), logger, logConfig),
			),
		),
		logger,
	)
}
