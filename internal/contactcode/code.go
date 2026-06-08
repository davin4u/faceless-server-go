// Package contactcode generates 8-character XXXX-XXXX user contact codes from a
// reduced alphabet (A-Z + 2-9, removing visually confusing I/O/0/1).
package contactcode

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/davin4u/faceless-server-go/internal/db"
)

const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// randomCode returns a fresh XXXX-XXXX string from the reduced charset.
func randomCode() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	var raw [8]byte
	for i := 0; i < 8; i++ {
		raw[i] = charset[int(b[i])%len(charset)]
	}
	return string(raw[:4]) + "-" + string(raw[4:]), nil
}

// Generate returns a fresh code that is not present in users.contact_code or
// retired_codes.code. Tries up to 10 times before giving up.
func Generate(ctx context.Context, d db.DB) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode()
		if err != nil {
			return "", err
		}

		row, err := d.Get(ctx, "SELECT 1 FROM users WHERE contact_code = ?", code)
		if err != nil {
			return "", err
		}
		if row != nil {
			continue
		}
		row, err = d.Get(ctx, "SELECT 1 FROM retired_codes WHERE code = ?", code)
		if err != nil {
			return "", err
		}
		if row != nil {
			continue
		}
		return code, nil
	}
	return "", errors.New("failed to generate unique contact code")
}

// GenerateInvitation returns a code not present in users.invitation_code.
// Tries up to 10 times before giving up.
func GenerateInvitation(ctx context.Context, d db.DB) (string, error) {
	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode()
		if err != nil {
			return "", err
		}
		row, err := d.Get(ctx, "SELECT 1 FROM users WHERE invitation_code = ?", code)
		if err != nil {
			return "", err
		}
		if row != nil {
			continue
		}
		return code, nil
	}
	return "", errors.New("failed to generate unique invitation code")
}
