package authorize

// Request is what an application asks for at the authorization endpoint,
// straight from the query string it arrived in.
type Request struct {
	ClientID     string
	RedirectURI  string
	ResponseType string
	Scope        string
	State        string

	CodeChallenge       string
	CodeChallengeMethod string

	Resource string
}
