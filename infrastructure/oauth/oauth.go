// Package oauth signs people in with somebody else's account.
//
// Every provider here does the same dance -- send the browser to be asked,
// take back a code, trade the code for a token, read who answered -- so what
// differs between Google, GitHub and LinkedIn is only where their doors are and
// how they word the answer. The dance is in this file; the wording is in
// theirs.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrExchangeFailed means the provider would not trade the code for a token: it
// was already spent, it was issued to somebody else, or it was never ours.
var ErrExchangeFailed = errors.New("the provider refused the authorization code")

// requestTimeout bounds one call to a provider. A provider that is slow must
// not hold a login open.
const requestTimeout = 10 * time.Second

// Config is what a provider needs in order to speak for this estate: who we
// are to it, and where it sends the browser back.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// IsZero reports a provider that was never configured, and which therefore is
// not offered at all.
func (c Config) IsZero() bool {
	return len(c.ClientID) == 0 || len(c.ClientSecret) == 0
}

// endpoints are a provider's own doors. They are fields rather than constants
// so that a test can stand in front of them.
type endpoints struct {
	authorization string
	token         string
	user          string
	emails        string
}

// httpClient is what a provider talks through. It is an interface so a test
// can answer for the provider without one listening on a socket.
type httpClient interface {
	Do(request *http.Request) (*http.Response, error)
}

func defaultClient() httpClient {
	return &http.Client{Timeout: requestTimeout}
}

// authorizationURL builds the address the browser is sent to. Every provider's
// looks the same; the scopes and the door are what differ.
func authorizationURL(door string, config Config, scope string, state string) string {
	query := url.Values{
		"client_id":     {config.ClientID},
		"redirect_uri":  {config.RedirectURL},
		"response_type": {"code"},
		"scope":         {scope},
		"state":         {state},
	}

	return door + "?" + query.Encode()
}

// exchange trades the code the browser came back with for a token to read the
// person's details with.
//
// The client secret goes in the body rather than the url: a url is written down
// by every proxy and access log on the way, and a secret written down is a
// secret spent.
func exchange(ctx context.Context, client httpClient, door string, config Config, code string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {config.ClientID},
		"client_secret": {config.ClientSecret},
		"redirect_uri":  {config.RedirectURL},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, door, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// GitHub answers in form encoding unless asked otherwise; Google always
	// answers in json and does not mind being asked.
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s", ErrExchangeFailed, response.Status)
	}

	var body struct {
		AccessToken      string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}

	if err := decode(response.Body, &body); err != nil {
		return "", err
	}

	// a refusal arrives as a perfectly good 200 with an error in it
	if len(body.Error) > 0 {
		return "", fmt.Errorf("%w: %s", ErrExchangeFailed, body.Error)
	}

	if len(body.AccessToken) == 0 {
		return "", ErrExchangeFailed
	}

	return body.AccessToken, nil
}

// get reads json from a provider's api as the person who just signed in.
func get(ctx context.Context, client httpClient, door string, accessToken string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, door, nil)
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", ErrExchangeFailed, response.Status)
	}

	return decode(response.Body, into)
}

// decode reads a bounded amount of json: what a provider sends is not this
// estate's to trust with its memory.
func decode(body io.Reader, into any) error {
	return json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(into)
}
