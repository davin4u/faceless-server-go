// Package push sends data-only FCM wake-up notifications. The rest of the
// server depends on the Sender interface; when FCM is unconfigured the Noop
// implementation is used and all sends are no-ops.
package push

import (
	"context"
	"errors"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

var ErrInvalidToken = errors.New("invalid fcm token")

type Sender interface {
	SendMessageWake(ctx context.Context, toUser, fromUser string)
	SendCallWake(ctx context.Context, toUser, fromUser, callType string)
}

type Noop struct{}

func (Noop) SendMessageWake(context.Context, string, string)      {}
func (Noop) SendCallWake(context.Context, string, string, string) {}

type fcmClient interface {
	Send(ctx context.Context, token string, data map[string]string) error
}

type realClient struct{ m *messaging.Client }

func (c realClient) Send(ctx context.Context, token string, data map[string]string) error {
	_, err := c.m.Send(ctx, &messaging.Message{
		Token:   token,
		Data:    data,
		Android: &messaging.AndroidConfig{Priority: "high"},
	})
	if err != nil {
		// Only prune on a definitively dead token. IsInvalidArgument can also
		// indicate a malformed payload, so do NOT prune on it.
		if messaging.IsUnregistered(err) {
			return ErrInvalidToken
		}
		return err
	}
	return nil
}

type fcmSender struct {
	client    fcmClient
	tokensFor func(ctx context.Context, userID string) ([]string, error)
	prune     func(ctx context.Context, token string)
}

func (s *fcmSender) fanout(ctx context.Context, toUser string, data map[string]string) {
	tokens, err := s.tokensFor(ctx, toUser)
	if err != nil {
		slog.Error("push.tokens.error", "user", toUser, "err", err)
		return
	}
	for _, tok := range tokens {
		if err := s.client.Send(ctx, tok, data); err != nil {
			if errors.Is(err, ErrInvalidToken) {
				s.prune(ctx, tok)
				slog.Info("push.token.pruned", "user", toUser)
			} else {
				slog.Error("push.send.error", "user", toUser, "err", err)
			}
		}
	}
}

// NOTE: FCM reserves certain data-payload keys — "from", "notification",
// "message_type", and anything prefixed with "google"/"gcm". Using any of them
// makes FCM reject the ENTIRE message ("Invalid data payload key: from"), so the
// device never wakes. Keep the sender id under "from_user", never "from".
func (s *fcmSender) SendMessageWake(ctx context.Context, toUser, fromUser string) {
	s.fanout(ctx, toUser, map[string]string{"type": "message", "from_user": fromUser})
}

func (s *fcmSender) SendCallWake(ctx context.Context, toUser, fromUser, callType string) {
	s.fanout(ctx, toUser, map[string]string{"type": "call", "from_user": fromUser, "callType": callType})
}

// New builds a real FCM Sender from a service-account JSON file. tokensFor and
// prune are injected so push has no direct dependency on the db package.
func New(ctx context.Context, credentialsFile string,
	tokensFor func(ctx context.Context, userID string) ([]string, error),
	prune func(ctx context.Context, token string)) (Sender, error) {
	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(credentialsFile))
	if err != nil {
		return nil, err
	}
	m, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	return &fcmSender{client: realClient{m: m}, tokensFor: tokensFor, prune: prune}, nil
}
