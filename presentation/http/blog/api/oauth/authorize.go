package oauth

import (
	"net/http"
	"net/url"

	"github.com/khanzadimahdi/testproject/application/oauth/authorize"
)

type authorizeHandler struct {
	useCase *authorize.UseCase

	// consentURL is the page that asks the person, which is the frontend's.
	// The authorization endpoint settles what can be settled without anybody,
	// and hands the rest over.
	consentURL string
}

func NewAuthorizeHandler(useCase *authorize.UseCase, consentURL string) *authorizeHandler {
	return &authorizeHandler{
		useCase:    useCase,
		consentURL: consentURL,
	}
}

// @Summary		Authorization endpoint
// @Description	reads what an application is asking for and sends the browser on to the page that asks the person about it
// @Tags		oauth
// @Produce		json
// @Param		client_id				query	string	true	"The application asking"
// @Param		redirect_uri			query	string	true	"Where it is answered; one it registered"
// @Param		response_type			query	string	true	"code"
// @Param		code_challenge			query	string	true	"The proof-key challenge"
// @Param		code_challenge_method	query	string	true	"S256"
// @Param		state					query	string	false	"Handed back untouched"
// @Param		scope					query	string	false	"mcp"
// @Param		resource				query	string	false	"What the application wants to reach"
// @Success		302
// @Failure		400	{object}	map[string]interface{}
// @Router		/oauth/authorize [get]
func (h *authorizeHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	response, err := h.useCase.Execute(r.Context(), &authorize.Request{
		ClientID:            query.Get("client_id"),
		RedirectURI:         query.Get("redirect_uri"),
		ResponseType:        query.Get("response_type"),
		Scope:               query.Get("scope"),
		State:               query.Get("state"),
		CodeChallenge:       query.Get("code_challenge"),
		CodeChallengeMethod: query.Get("code_challenge_method"),
		Resource:            query.Get("resource"),
	})

	// a failure that leaves no address it would be safe to answer at is shown
	// where the browser already is, rather than sent on to an address the
	// application only claims is its own.
	if err != nil {
		fail(rw, r, err)

		return
	}

	if response.Error != nil {
		redirect(rw, r, response.RedirectURI, url.Values{
			"error":             {response.Error.Code},
			"error_description": {response.Error.Description},
		}, response.State)

		return
	}

	redirect(rw, r, h.consentURL, url.Values{"request": {response.RequestToken}}, "")
}

// redirect sends the browser on, keeping whatever the address already carried
// and putting the state back exactly as the application gave it.
func redirect(rw http.ResponseWriter, r *http.Request, to string, values url.Values, state string) {
	parsed, err := url.Parse(to)
	if err != nil {
		fail(rw, r, err)

		return
	}

	query := parsed.Query()
	for name, value := range values {
		query.Set(name, value[0])
	}

	if len(state) > 0 {
		query.Set("state", state)
	}

	parsed.RawQuery = query.Encode()

	rw.Header().Set("Cache-Control", "no-store")
	http.Redirect(rw, r, parsed.String(), http.StatusFound)
}
