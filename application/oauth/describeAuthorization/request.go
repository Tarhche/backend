package describeauthorization

type Request struct {
	// RequestToken is the authorization request as the authorization endpoint
	// signed it, which the page asking about it was handed in its url.
	RequestToken string `json:"request"`
}
