package registerclient

type Response struct {
	ClientID         string `json:"client_id"`
	ClientIDIssuedAt int64  `json:"client_id_issued_at"`

	// ClientSecret is handed over once, here, and is never readable again. It
	// is absent for a public client, which has none.
	ClientSecret string `json:"client_secret,omitempty"`

	// ClientSecretExpiresAt is zero for a secret that does not expire, and
	// [RFC 7591] asks for it whenever a secret was issued, zero included.
	//
	// [RFC 7591]: https://www.rfc-editor.org/rfc/rfc7591
	ClientSecretExpiresAt *int64 `json:"client_secret_expires_at,omitempty"`

	ClientName   string   `json:"client_name,omitempty"`
	ClientURI    string   `json:"client_uri,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`

	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope,omitempty"`
}
