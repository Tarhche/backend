package oauth

import (
	"net/http"

	exchangecode "github.com/khanzadimahdi/testproject/application/oauth/exchangeCode"
	refreshsession "github.com/khanzadimahdi/testproject/application/oauth/refreshSession"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
)

type tokenHandler struct {
	exchange *exchangecode.UseCase
	refresh  *refreshsession.UseCase
}

// NewTokenHandler hands over a session: the one an application was approved
// for, or a renewal of one it already holds.
func NewTokenHandler(exchange *exchangecode.UseCase, refresh *refreshsession.UseCase) *tokenHandler {
	return &tokenHandler{
		exchange: exchange,
		refresh:  refresh,
	}
}

// @Summary		Token endpoint
// @Description	exchanges an authorization code for a session, or renews one
// @Tags		oauth
// @Accept		x-www-form-urlencoded
// @Produce		json
// @Param		grant_type		formData	string	true	"authorization_code or refresh_token"
// @Param		code			formData	string	false	"The code the application was handed"
// @Param		code_verifier	formData	string	false	"What the challenge was built from"
// @Param		redirect_uri	formData	string	false	"The address the code was given for"
// @Param		refresh_token	formData	string	false	"The refresh token being renewed"
// @Param		client_id		formData	string	false	"The application asking"
// @Param		client_secret	formData	string	false	"Its secret, when it has one"
// @Success		200	{object}	exchangecode.Response
// @Failure		400	{object}	map[string]interface{}
// @Failure		401	{object}	map[string]interface{}
// @Router		/oauth/token [post]
func (h *tokenHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		write(rw, http.StatusBadRequest, errorResponse{
			Error:            oauth.ErrorInvalidRequest,
			ErrorDescription: "the request is not a form",
		})

		return
	}

	id, secret := credentials(r)

	switch r.PostFormValue("grant_type") {
	case client.GrantAuthorizationCode:
		response, err := h.exchange.Execute(r.Context(), &exchangecode.Request{
			Code:         r.PostFormValue("code"),
			ClientID:     id,
			ClientSecret: secret,
			RedirectURI:  r.PostFormValue("redirect_uri"),
			CodeVerifier: r.PostFormValue("code_verifier"),
		})
		if err != nil {
			fail(rw, r, err)

			return
		}

		write(rw, http.StatusOK, response)
	case client.GrantRefreshToken:
		response, err := h.refresh.Execute(r.Context(), &refreshsession.Request{
			RefreshToken: r.PostFormValue("refresh_token"),
			ClientID:     id,
			ClientSecret: secret,
			Scope:        r.PostFormValue("scope"),
		})
		if err != nil {
			fail(rw, r, err)

			return
		}

		write(rw, http.StatusOK, response)
	default:
		write(rw, http.StatusBadRequest, errorResponse{
			Error:            oauth.ErrorUnsupportedGrantType,
			ErrorDescription: "this server hands over a session for an authorization code, and renews one for a refresh token",
		})
	}
}

// credentials read which application is asking, however it says so: in the
// form, or in an Authorization header the way client_secret_basic puts it.
func credentials(r *http.Request) (id string, secret string) {
	if id, secret, ok := r.BasicAuth(); ok {
		return id, secret
	}

	return r.PostFormValue("client_id"), r.PostFormValue("client_secret")
}
