package push

import (
	"context"
	"testing"
)

type fakeFCM struct {
	sent    []map[string]string
	tokens  []string
	invalid map[string]bool
}

func (f *fakeFCM) Send(_ context.Context, token string, data map[string]string) error {
	f.tokens = append(f.tokens, token)
	f.sent = append(f.sent, data)
	if f.invalid[token] {
		return ErrInvalidToken
	}
	return nil
}

func TestSendMessageWake_buildsDataAndFansOut(t *testing.T) {
	f := &fakeFCM{invalid: map[string]bool{}}
	pruned := []string{}
	s := &fcmSender{
		client:    f,
		tokensFor: func(_ context.Context, user string) ([]string, error) { return []string{"t1", "t2"}, nil },
		prune:     func(_ context.Context, token string) { pruned = append(pruned, token) },
	}
	s.SendMessageWake(context.Background(), "u2", "u1")
	if len(f.tokens) != 2 {
		t.Fatalf("want 2 sends, got %d", len(f.tokens))
	}
	if f.sent[0]["type"] != "message" || f.sent[0]["from"] != "u1" {
		t.Fatalf("bad data payload: %v", f.sent[0])
	}
	if len(pruned) != 0 {
		t.Fatalf("nothing should be pruned, got %v", pruned)
	}
}

func TestSendMessageWake_prunesInvalidToken(t *testing.T) {
	f := &fakeFCM{invalid: map[string]bool{"bad": true}}
	pruned := []string{}
	s := &fcmSender{
		client:    f,
		tokensFor: func(_ context.Context, _ string) ([]string, error) { return []string{"good", "bad"}, nil },
		prune:     func(_ context.Context, token string) { pruned = append(pruned, token) },
	}
	s.SendMessageWake(context.Background(), "u2", "u1")
	if len(pruned) != 1 || pruned[0] != "bad" {
		t.Fatalf("want prune [bad], got %v", pruned)
	}
}

func TestSendCallWake_payload(t *testing.T) {
	f := &fakeFCM{invalid: map[string]bool{}}
	s := &fcmSender{
		client:    f,
		tokensFor: func(_ context.Context, _ string) ([]string, error) { return []string{"t1"}, nil },
		prune:     func(_ context.Context, _ string) {},
	}
	s.SendCallWake(context.Background(), "u2", "u1", "video")
	if f.sent[0]["type"] != "call" || f.sent[0]["from"] != "u1" || f.sent[0]["callType"] != "video" {
		t.Fatalf("bad call payload: %v", f.sent[0])
	}
}

func TestNoopSenderDoesNothing(t *testing.T) {
	var s Sender = Noop{}
	s.SendMessageWake(context.Background(), "u2", "u1")
	s.SendCallWake(context.Background(), "u2", "u1", "voice")
}
