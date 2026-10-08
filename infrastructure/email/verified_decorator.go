package email

import (
	"context"
	"errors"
	"log/slog"

	emailverifier "github.com/AfterShip/email-verifier"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// Verifier says whether an address can receive mail; *emailverifier.Verifier is one.
type Verifier interface {
	Verify(email string) (*emailverifier.Result, error)
}

var _ Verifier = emailverifier.NewVerifier()

const (
	reasonInvalidSyntax    = "invalid syntax"
	reasonDisposableDomain = "disposable domain"
	reasonNoMailServer     = "domain has no mail server"
	reasonUnreachable      = "mailbox unreachable"
)

// verifiedDecorator is a domain.Mailer that hands mail to the next mailer only when the
// recipient address is verified to be able to receive it, and logs the mail it
// drops instead of sending it.
type verifiedDecorator struct {
	next     domain.Mailer
	verifier Verifier
	logger   *slog.Logger
	tracer   oteltrace.Tracer
}

var _ domain.Mailer = &verifiedDecorator{}

func NewVerifiedDecorator(next domain.Mailer, verifier Verifier, logger *slog.Logger) *verifiedDecorator {
	return &verifiedDecorator{
		next:     next,
		verifier: verifier,
		logger:   logger,
		tracer:   otel.Tracer("email-verifier"),
	}
}

func (m *verifiedDecorator) SendMail(ctx context.Context, from string, to string, subject string, body []byte) error {
	ctx, span := m.tracer.Start(ctx, "email.verify")
	defer span.End()

	result, err := m.verifier.Verify(to)
	if reason, invalid := reasonNotToSend(result, err); invalid {
		span.SetAttributes(attribute.String("email.verification.reason", reason))
		m.logger.WarnContext(ctx, "mail not sent: the recipient address is not valid",
			slog.String("to", to),
			slog.String("subject", subject),
			slog.String("reason", reason),
		)

		return nil
	}

	if err != nil {
		return trace.RecordError(span, err)
	}

	return m.next.SendMail(ctx, from, to, subject, body)
}

// reasonNotToSend reads the verifier's verdict. A lookup that failed to reach a
// verdict is not one, so that error is left to the caller.
func reasonNotToSend(result *emailverifier.Result, err error) (string, bool) {
	if result == nil {
		return "", false
	}

	if !result.Syntax.Valid {
		return reasonInvalidSyntax, true
	}

	if result.Disposable {
		return reasonDisposableDomain, true
	}

	var lookupErr *emailverifier.LookupError
	if errors.As(err, &lookupErr) && lookupErr.Message == emailverifier.ErrNoSuchHost {
		return reasonNoMailServer, true
	}

	if err != nil {
		return "", false
	}

	if !result.HasMxRecords {
		return reasonNoMailServer, true
	}

	if result.Reachable == "no" {
		return reasonUnreachable, true
	}

	return "", false
}
