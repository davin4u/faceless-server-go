package namegen

import (
	"context"
	"regexp"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/db"
)

type takenStub struct{ taken map[string]bool }

func (s *takenStub) Get(ctx context.Context, q string, a ...any) (db.Row, error) {
	if s.taken[a[0].(string)] {
		return db.Row{"1": int64(1)}, nil
	}
	return nil, nil
}
func (s *takenStub) All(ctx context.Context, q string, a ...any) ([]db.Row, error)  { return nil, nil }
func (s *takenStub) Run(ctx context.Context, q string, a ...any) (db.Result, error) { return db.Result{}, nil }
func (s *takenStub) Exec(ctx context.Context, q string) error                       { return nil }
func (s *takenStub) Tx(ctx context.Context, fn func(tx db.Tx) error) error          { return nil }
func (s *takenStub) Close() error                                                   { return nil }
func (s *takenStub) InsertIgnore(_, _, _ string) string                             { return "" }
func (s *takenStub) NowEpoch() string                                               { return "" }
func (s *takenStub) Dialect() string                                                { return "sqlite" }

var nameRE = regexp.MustCompile(`^[A-Z][a-zA-Z]+ [A-Z][a-zA-Z]+ \d{2}$`)

func TestGenerateDisplayName_Format(t *testing.T) {
	s := &takenStub{taken: map[string]bool{}}
	for i := 0; i < 50; i++ {
		n, err := GenerateDisplayName(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		if !nameRE.MatchString(n) {
			t.Errorf("name %q does not match expected format", n)
		}
	}
}

func TestGenerateDisplayName_Uniqueness(t *testing.T) {
	// Pre-occupy a few; just ensure none returned matches occupied set.
	s := &takenStub{taken: map[string]bool{"Cosmic Penguin 01": true}}
	for i := 0; i < 100; i++ {
		n, _ := GenerateDisplayName(context.Background(), s)
		if n == "Cosmic Penguin 01" {
			t.Error("returned a name that was supposed to be taken")
		}
	}
}
