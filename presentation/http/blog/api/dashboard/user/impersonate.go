package user

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	impersonateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/impersonateUser"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type impersonateHandler struct {
	useCase *impersonateuser.UseCase
}

func NewImpersonateHandler(useCase *impersonateuser.UseCase) *impersonateHandler {
	return &impersonateHandler{
		useCase: useCase,
	}
}

// @Summary		Sign in as a user
// @Description	obtain a session that acts as the given user, saying who is behind it
// @Tags			dashboard users
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"User UUID"
// @Success		200		{object}	impersonateuser.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/users/{uuid}/impersonate [post]
func (h *impersonateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &impersonateuser.Request{
		UserUUID:                     r.PathValue("uuid"),
		ImpersonatorUUID:             auth.FromContext(r.Context()).UUID,
		CallerIsAlreadyImpersonating: len(auth.ImpersonatorFromContext(r.Context())) > 0,
	}

	response, err := h.useCase.Execute(r.Context(), request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
