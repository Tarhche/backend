package authorize

import "github.com/khanzadimahdi/testproject/application/oauth"

// Response says what to do with an authorization request: put it to the
// person, or send the application away with a reason.
type Response struct {
	// RequestToken carries the request to the page that asks the person about
	// it, and is empty when there is nothing to ask.
	RequestToken string

	// RedirectURI and State are where the application is answered, and what it
	// gave us to recognise its own request by.
	RedirectURI string
	State       string

	// Error is what the application is told when the request cannot be put to
	// anybody, and it is told at the address it registered. A failure that
	// leaves no safe address to answer at comes back as an error instead, and
	// is shown where the browser already is.
	Error *oauth.Error
}
