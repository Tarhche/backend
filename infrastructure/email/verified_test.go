package email

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	emailverifier "github.com/AfterShip/email-verifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestVerified_SendMail(t *testing.T) {
	t.Parallel()

	const (
		from    = "info@noreply.nowhere.loc"
		to      = "someone@example.com"
		subject = "Registration"
	)
	body := []byte("<p>hello</p>")

	deliverable := &emailverifier.Result{
		Email:        to,
		Reachable:    "unknown",
		Syntax:       emailverifier.Syntax{Username: "someone", Domain: "example.com", Valid: true},
		HasMxRecords: true,
	}

	t.Run("sends to an address that can receive mail", func(t *testing.T) {
		t.Parallel()

		var (
			next     MockMailer
			verifier MockVerifier
			log      bytes.Buffer
		)

		verifier.On("Verify", to).Once().Return(deliverable, nil)
		defer verifier.AssertExpectations(t)

		next.On("SendMail", mock.Anything, from, to, subject, body).Once().Return(nil)
		defer next.AssertExpectations(t)

		mailer := NewVerified(&next, &verifier, slog.New(slog.NewTextHandler(&log, nil)))

		assert.NoError(t, mailer.SendMail(context.Background(), from, to, subject, body))
		assert.Empty(t, log.String())
	})

	t.Run("returns what the next mailer returns", func(t *testing.T) {
		t.Parallel()

		var (
			next     MockMailer
			verifier MockVerifier
		)

		sendErr := errors.New("relay refused")

		verifier.On("Verify", to).Once().Return(deliverable, nil)
		defer verifier.AssertExpectations(t)

		next.On("SendMail", mock.Anything, from, to, subject, body).Once().Return(sendErr)
		defer next.AssertExpectations(t)

		mailer := NewVerified(&next, &verifier, slog.New(slog.DiscardHandler))

		assert.ErrorIs(t, mailer.SendMail(context.Background(), from, to, subject, body), sendErr)
	})

	t.Run("logs instead of sending when the address is not valid", func(t *testing.T) {
		t.Parallel()

		noSuchHost := &emailverifier.LookupError{Message: emailverifier.ErrNoSuchHost, Details: "lookup example.invalid: no such host"}

		cases := map[string]struct {
			result *emailverifier.Result
			err    error
			reason string
		}{
			"invalid syntax": {
				result: &emailverifier.Result{Email: "not-an-address", Reachable: "unknown"},
				reason: reasonInvalidSyntax,
			},
			"disposable domain": {
				result: &emailverifier.Result{
					Email:      "someone@mailinator.com",
					Reachable:  "unknown",
					Syntax:     emailverifier.Syntax{Username: "someone", Domain: "mailinator.com", Valid: true},
					Disposable: true,
				},
				reason: reasonDisposableDomain,
			},
			"domain does not exist": {
				result: &emailverifier.Result{
					Email:     "someone@example.invalid",
					Reachable: "no",
					Syntax:    emailverifier.Syntax{Username: "someone", Domain: "example.invalid", Valid: true},
				},
				err:    noSuchHost,
				reason: reasonNoMailServer,
			},
			"domain has no mx records": {
				result: &emailverifier.Result{
					Email:     to,
					Reachable: "unknown",
					Syntax:    emailverifier.Syntax{Username: "someone", Domain: "example.com", Valid: true},
				},
				reason: reasonNoMailServer,
			},
			"mailbox reported unreachable": {
				result: &emailverifier.Result{
					Email:        to,
					Reachable:    "no",
					Syntax:       emailverifier.Syntax{Username: "someone", Domain: "example.com", Valid: true},
					HasMxRecords: true,
					SMTP:         &emailverifier.SMTP{HostExists: true},
				},
				reason: reasonUnreachable,
			},
		}

		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				var (
					next     MockMailer
					verifier MockVerifier
					log      bytes.Buffer
				)

				verifier.On("Verify", tc.result.Email).Once().Return(tc.result, tc.err)
				defer verifier.AssertExpectations(t)
				defer next.AssertNotCalled(t, "SendMail", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

				mailer := NewVerified(&next, &verifier, slog.New(slog.NewTextHandler(&log, nil)))

				assert.NoError(t, mailer.SendMail(context.Background(), from, tc.result.Email, subject, body))

				assert.Contains(t, log.String(), "level=WARN")
				assert.Contains(t, log.String(), "mail not sent: the recipient address is not valid")
				assert.Contains(t, log.String(), "to="+tc.result.Email)
				assert.Contains(t, log.String(), "subject="+subject)
				assert.Contains(t, log.String(), "reason=\""+tc.reason+"\"")
			})
		}
	})

	t.Run("a lookup that reached no verdict is an error, not a verdict", func(t *testing.T) {
		t.Parallel()

		var (
			next     MockMailer
			verifier MockVerifier
			log      bytes.Buffer
		)

		lookupErr := errors.New("lookup example.com: i/o timeout")

		verifier.On("Verify", to).Once().Return(&emailverifier.Result{
			Email:     to,
			Reachable: "unknown",
			Syntax:    emailverifier.Syntax{Username: "someone", Domain: "example.com", Valid: true},
		}, lookupErr)
		defer verifier.AssertExpectations(t)
		defer next.AssertNotCalled(t, "SendMail", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)

		mailer := NewVerified(&next, &verifier, slog.New(slog.NewTextHandler(&log, nil)))

		assert.ErrorIs(t, mailer.SendMail(context.Background(), from, to, subject, body), lookupErr)
		assert.Empty(t, log.String())
	})

	t.Run("with the real verifier, no lookup is needed to reject syntax and disposable domains", func(t *testing.T) {
		t.Parallel()

		for _, address := range []string{"not-an-address", "someone@mailinator.com"} {
			var (
				next MockMailer
				log  bytes.Buffer
			)

			mailer := NewVerified(&next, emailverifier.NewVerifier(), slog.New(slog.NewTextHandler(&log, nil)))

			assert.NoError(t, mailer.SendMail(context.Background(), from, address, subject, body))
			next.AssertNotCalled(t, "SendMail", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			assert.Contains(t, log.String(), "to="+address)
		}
	})
}
