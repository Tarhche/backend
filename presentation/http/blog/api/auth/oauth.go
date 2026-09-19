package auth

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/providerredirect"
	"github.com/khanzadimahdi/testproject/application/auth/providers"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type providersHandler struct {
	useCase *providers.UseCase
}

func NewProvidersHandler(useCase *providers.UseCase) *providersHandler {
	return &providersHandler{
		useCase: useCase,
	}
}

// @Summary		Login providers
// @Description	list the accounts elsewhere that may be signed in with
// @Tags			auth
// @Accept			json
// @Produce		json
// @Success		200	{object}	providers.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/auth/oauth [get]
func (h *providersHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}

type providerRedirectHandler struct {
	useCase *providerredirect.UseCase
}

func NewProviderRedirectHandler(useCase *providerredirect.UseCase) *providerRedirectHandler {
	return &providerRedirectHandler{
		useCase: useCase,
	}
}

// @Summary		Login with a provider
// @Description	where to send the browser to sign in with somebody else's account, and the state to compare when it comes back
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			provider	path		string	true	"Provider name"
// @Success		200			{object}	providerredirect.Response
// @Failure		400			{object}	map[string]interface{}
// @Failure		500			{object}	map[string]interface{}
// @Router			/auth/oauth/{provider} [get]
func (h *providerRedirectHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &providerredirect.Request{
		Provider: r.PathValue("provider"),
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
