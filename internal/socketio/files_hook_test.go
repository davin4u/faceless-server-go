package socketio

import (
	"context"
	"testing"
)

type recordingDeleter struct {
	msgIDs  []string
	senders []string
}

func (r *recordingDeleter) DeleteByMessage(_ context.Context, messageID, senderID string) {
	r.msgIDs = append(r.msgIDs, messageID)
	r.senders = append(r.senders, senderID)
}

func TestSetFiles_StoresDeleter(t *testing.T) {
	s := &Server{}
	rec := &recordingDeleter{}
	s.SetFiles(rec)
	if s.files == nil {
		t.Fatal("files deleter not set")
	}
	s.files.DeleteByMessage(context.Background(), "m1", "uA")
	if len(rec.msgIDs) != 1 || rec.msgIDs[0] != "m1" || rec.senders[0] != "uA" {
		t.Fatalf("delete not forwarded: %+v", rec)
	}
}
