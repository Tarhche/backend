package refreshsession

// Request is an application asking for the session it holds to be renewed.
type Request struct {
	RefreshToken string
	ClientID     string
	ClientSecret string
	Scope        string
}
