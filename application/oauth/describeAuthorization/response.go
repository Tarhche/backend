package describeauthorization

type Response struct {
	ClientID    string   `json:"client_id"`
	ClientName  string   `json:"client_name,omitempty"`
	ClientURI   string   `json:"client_uri,omitempty"`
	RedirectURI string   `json:"redirect_uri"`
	Scopes      []string `json:"scopes"`
}
