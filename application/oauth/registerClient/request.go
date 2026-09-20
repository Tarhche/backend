package registerclient

// Request is what an application says about itself when it registers, in the
// shape [RFC 7591] gives it.
//
// [RFC 7591]: https://www.rfc-editor.org/rfc/rfc7591
type Request struct {
	ClientName   string   `json:"client_name"`
	ClientURI    string   `json:"client_uri"`
	RedirectURIs []string `json:"redirect_uris"`

	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}
