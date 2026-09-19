package providerredirect

import "github.com/khanzadimahdi/testproject/domain"

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// URL is where to send the browser to be asked who it is.
	URL string `json:"url,omitempty"`

	// State is handed back untouched when the browser returns. Whoever sends
	// the browser keeps it and compares it then: that is how a login that comes
	// back is known to be the one that went out, and not somebody else's.
	State string `json:"state,omitempty"`
}
