package exchangecode

// Request is an application collecting the session it was given.
type Request struct {
	Code         string
	ClientID     string
	ClientSecret string
	RedirectURI  string

	// CodeVerifier is what the challenge the request was made with was built
	// from, and is what proves this is the application that asked.
	CodeVerifier string
}
