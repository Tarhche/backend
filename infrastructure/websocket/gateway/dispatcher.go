package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain"
)

// dispatcher validates a client request, registers it so its reply can be
// routed back, and produces it onto the queue. It knows nothing about
// transports.
type dispatcher struct {
	validator     *requestValidator
	registry      RequestRegistry
	producer      domain.Producer
	subjectPrefix string
	logger        *slog.Logger
}

// dispatch reports the server-side id the request was registered under. A
// non-empty id means the registry holds an entry the caller must delete, which
// stays true when dispatch also returns an error.
//
// A request naming a stream registers nothing: it is input to a question
// already asked, not a new one, so there is no id to hand back and no reply to
// route.
func (d *dispatcher) dispatch(ctx context.Context, request *domain.Request) (string, domain.ValidationErrors, error) {
	validationErrors, err := d.validator.validate(request)
	if err != nil {
		d.logger.ErrorContext(ctx, "error on validating request", "error", err)

		return "", nil, err
	}

	if len(validationErrors) > 0 {
		return "", validationErrors, nil
	}

	if len(request.StreamID) > 0 {
		return "", nil, d.dispatchToStream(ctx, request)
	}

	// the client's id is unique only to that client; the rest of the system
	// routes on the server-side one.
	serverSideID, err := d.registry.Add(request.ID)
	if err != nil {
		d.logger.ErrorContext(ctx, "error on adding request to registry", "error", err)

		return "", nil, err
	}

	payload, err := injectRequestID(request.Payload, serverSideID)
	if err != nil {
		d.logger.ErrorContext(ctx, "error on marshalling request", "error", err)

		return serverSideID, nil, err
	}

	// produce, not publish: the request must be handled once, by a single
	// replica, however many are running.
	if err := d.producer.Produce(ctx, d.subjectPrefix+request.Subject, payload); err != nil {
		d.logger.ErrorContext(ctx, "error on publishing request", "error", err)

		return serverSideID, nil, err
	}

	return serverSideID, nil, nil
}

// dispatchToStream carries input to a stream the client already has open. The
// stream's server-side id travels as the payload's own id, so a handler reads
// it exactly as it read the request that opened the stream.
func (d *dispatcher) dispatchToStream(ctx context.Context, request *domain.Request) error {
	serverSideID, err := d.registry.GetServerSideID(request.StreamID)
	if err != nil {
		d.logger.ErrorContext(ctx, "error on resolving a stream id", "error", err, "streamID", request.StreamID)

		return err
	}

	payload, err := injectRequestID(request.Payload, serverSideID)
	if err != nil {
		d.logger.ErrorContext(ctx, "error on marshalling request", "error", err)

		return err
	}

	if err := d.producer.Produce(ctx, d.subjectPrefix+request.Subject, payload); err != nil {
		d.logger.ErrorContext(ctx, "error on publishing request", "error", err)

		return err
	}

	return nil
}

// injectRequestID replaces the client's id in the payload with the server-side one.
func injectRequestID(payload []byte, requestID string) ([]byte, error) {
	var request map[string]any

	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, err
	}

	if request == nil {
		request = make(map[string]any, 1)
	}

	request["id"] = requestID

	return json.Marshal(request)
}
