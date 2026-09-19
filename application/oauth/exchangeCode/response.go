package exchangecode

// Response is an ordinary session of ours, in the shape OAuth hands one over
// in. The tokens are the same tokens the dashboard carries: the application
// holds nothing of its own.
type Response struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}
