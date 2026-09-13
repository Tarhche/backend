package middleware

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
)

// request is the part of every request these middlewares read: which request a
// refusal answers, and the token it carries.
//
// The token travels in the payload because a websocket handshake from a browser
// carries no Authorization header, and one connection is shared by every
// request on it: the person is established per request, not per socket.
type request struct {
	ID          string `json:"id"`
	AccessToken string `json:"access_token"`
}

// refuse ends a request that never reached a use case. It is the reply a client
// waiting on a stream is given instead of one.
func refuse(ctx context.Context, replyer domain.Replyer, requestID string, reason string) error {
	payload, err := json.Marshal(map[string]any{"errors": domain.ValidationErrors{"access_token": reason}})
	if err != nil {
		return err
	}

	return replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Kind:      domain.ReplyEOF,
		Payload:   payload,
	})
}
