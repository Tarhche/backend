package oauth

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	approveauthorization "github.com/khanzadimahdi/testproject/application/oauth/approveAuthorization"
	describeauthorization "github.com/khanzadimahdi/testproject/application/oauth/describeAuthorization"
)

type describeAuthorizationHandler struct {
	useCase *describeauthorization.UseCase
}

// NewDescribeAuthorizationHandler tells the consent page what it is asking
// about. The page is handed a signed request in its url and can read nothing
// out of it for itself, so what it shows comes from here, where the signature
// is checked.
func NewDescribeAuthorizationHandler(useCase *describeauthorization.UseCase) *describeAuthorizationHandler {
	return &describeAuthorizationHandler{
		useCase: useCase,
	}
}

// @Summary		Describe a pending authorization
// @Description	what the application asking is called and what it is asking for
// @Tags		oauth
// @Produce		json
// @Param		request	query		string	true	"The authorization request, as the authorization endpoint signed it"
// @Success		200		{object}	describeauthorization.Response
// @Failure		400		{object}	map[string]interface{}
// @Router		/oauth/authorization [get]
func (h *describeAuthorizationHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &describeauthorization.Request{
		RequestToken: r.URL.Query().Get("request"),
	})
	if err != nil {
		fail(rw, r, err)

		return
	}

	write(rw, http.StatusOK, response)
}

type approveAuthorizationHandler struct {
	useCase *approveauthorization.UseCase
}

// NewApproveAuthorizationHandler records the answer. It sits behind
// authentication like any other route: who is approving is whoever the
// request's own token is for, and never what the page says it is.
func NewApproveAuthorizationHandler(useCase *approveauthorization.UseCase) *approveAuthorizationHandler {
	return &approveAuthorizationHandler{
		useCase: useCase,
	}
}

// @Summary		Answer a pending authorization
// @Description	approves or refuses an application, and says where to send the browser next
// @Tags		oauth
// @Accept		json
// @Produce		json
// @Param		body	body		approveauthorization.Request	true	"The request being answered"
// @Success		200		{object}	approveauthorization.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		401		{object}	map[string]interface{}
// @Router		/oauth/authorization [post]
func (h *approveAuthorizationHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request approveauthorization.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		write(rw, http.StatusBadRequest, errorResponse{
			Error:            "invalid_request",
			ErrorDescription: "the answer is not valid json",
		})

		return
	}

	request.UserUUID = auth.UUIDFromContext(r.Context())
	request.ImpersonatorUUID = auth.ImpersonatorFromContext(r.Context())

	response, err := h.useCase.Execute(r.Context(), &request)
	if err != nil {
		fail(rw, r, err)

		return
	}

	write(rw, http.StatusOK, response)
}
