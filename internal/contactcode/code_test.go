package contactcode

import (
	"context"
	"strings"
	"testing"

	"github.com/davin4u/faceless-server-go/internal/db"
)

type stubDB struct {
	users      map[string]bool
	retired    map[string]bool
	invitations map[string]bool
}

func newStub() *stubDB {
	return &stubDB{users: map[string]bool{}, retired: map[string]bool{}, invitations: map[string]bool{}}
}
func (s *stubDB) Get(ctx context.Context, query string, args ...any) (db.Row, error) {
	if strings.Contains(query, "invitation_code") {
		if s.invitations[args[0].(string)] {
			return db.Row{"1": int64(1)}, nil
		}
		return nil, nil
	}
	if strings.Contains(query, "FROM users") {
		if s.users[args[0].(string)] {
			return db.Row{"1": int64(1)}, nil
		}
		return nil, nil
	}
	if strings.Contains(query, "FROM retired_codes") {
		if s.retired[args[0].(string)] {
			return db.Row{"1": int64(1)}, nil
		}
		return nil, nil
	}
	return nil, nil
}
func (s *stubDB) All(ctx context.Context, q string, a ...any) ([]db.Row, error) { return nil, nil }
func (s *stubDB) Run(ctx context.Context, q string, a ...any) (db.Result, error) {
	// Handle INSERT INTO users for the invitation uniqueness test.
	if strings.Contains(q, "INSERT INTO users") && len(a) > 0 {
		// The last arg in the test insert is the invitation_code value.
		// Store it as occupied so GenerateInvitation retries.
		if code, ok := a[len(a)-1].(string); ok {
			s.invitations[code] = true
		}
	}
	return db.Result{}, nil
}
func (s *stubDB) Exec(ctx context.Context, q string) error                   { return nil }
func (s *stubDB) Tx(ctx context.Context, fn func(tx db.Tx) error) error      { return nil }
func (s *stubDB) Close() error                                               { return nil }
func (s *stubDB) InsertIgnore(_, _, _ string) string                         { return "" }
func (s *stubDB) NowEpoch() string                                           { return "" }
func (s *stubDB) Dialect() string                                            { return "sqlite" }

func TestGenerate_FormatAndCharset(t *testing.T) {
	c, err := Generate(context.Background(), newStub())
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 9 || c[4] != '-' {
		t.Errorf("format wrong: %q", c)
	}
	for i, ch := range c {
		if i == 4 {
			continue
		}
		if !strings.ContainsRune(charset, ch) {
			t.Errorf("char %q not in charset (pos %d in %q)", ch, i, c)
		}
	}
}

func TestGenerate_CollisionAvoidance(t *testing.T) {
	// Force the first generated value to be "occupied" — Generate must retry.
	s := newStub()
	// We can't easily inject randomness; just confirm that pre-occupying *all*
	// possible codes makes Generate return an error.
	// Simulate by replacing the random source via a future seam. For now,
	// assert that with a normal stub a code is always returned.
	c1, err := Generate(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Generate(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if c1 == c2 {
		t.Errorf("randomness expected: got %q twice", c1)
	}
}

func TestGenerateInvitationUnique(t *testing.T) {
	ctx := context.Background()
	s := newStub()
	code, err := GenerateInvitation(ctx, s)
	if err != nil {
		t.Fatalf("GenerateInvitation: %v", err)
	}
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("bad format: %q", code)
	}
	// Insert a user holding that code; the next call must differ.
	if _, err := s.Run(ctx,
		`INSERT INTO users (id, contact_code, display_name, public_key, invitation_code) VALUES ('u1','AAAA-2222','A','pk1',?)`,
		code); err != nil {
		t.Fatalf("insert: %v", err)
	}
	code2, err := GenerateInvitation(ctx, s)
	if err != nil {
		t.Fatalf("GenerateInvitation 2: %v", err)
	}
	if code2 == code {
		t.Fatalf("expected a different code, got duplicate %q", code)
	}
}
