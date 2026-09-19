package oauth

import (
	"context"
	"errors"
)

// ErrUnknownProvider means nobody here signs people in that way: the name is
// not one of the providers this estate was configured with.
var ErrUnknownProvider = errors.New("unknown identity provider")

// Identity is what another estate says about somebody who has just proved
// themselves to it.
//
// ID is the only part worth trusting for good: it is that estate's own name for
// the person, and it does not change when they rename themselves or move house.
// An email is worth believing only when the provider says it verified it, which
// is what Verified is for -- an unverified address is an address somebody typed,
// and signing in with it would be signing in as whoever owns it here.
type Identity struct {
	Provider string
	ID       string
	Email    string
	Verified bool
	Name     string
	Avatar   string
}

// Provider is one way of having somebody else vouch for who a person is.
//
// The dance is always the same, which is why one interface covers Google,
// GitHub and whatever comes next: send the browser somewhere to be asked,
// receive a code back, and exchange that code for who answered. What differs is
// only where to send them and how to read the answer, which is each
// implementation's business and nobody else's.
type Provider interface {
	// Name is what a request asks for this provider by.
	Name() string

	// AuthorizationURL is where the browser is sent to be asked. The state is
	// handed back untouched when the browser returns, which is how the caller
	// knows the answer is to its own question.
	AuthorizationURL(state string) string

	// Identify exchanges the code the browser came back with for who it stands
	// for. A code is good once, and only to whoever it was issued for.
	Identify(ctx context.Context, code string) (Identity, error)
}

// Providers is every way of signing in this estate knows, under the name a
// request asks for it by. A provider that was not configured is not in here,
// and asking for it is asking for somebody who does not exist.
type Providers map[string]Provider

// Get is the provider a request named, or ErrUnknownProvider.
func (p Providers) Get(name string) (Provider, error) {
	provider, ok := p[name]
	if !ok {
		return nil, ErrUnknownProvider
	}

	return provider, nil
}

// Names is every provider there is, for telling a caller what it may ask for.
func (p Providers) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}

	return names
}
