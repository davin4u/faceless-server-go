package db

import "testing"

func TestRow_Get(t *testing.T) {
	r := Row{"id": "u1", "count": int64(5), "nullable": nil}

	if got := r.Str("id"); got != "u1" {
		t.Errorf("Str(id) = %q", got)
	}
	if got := r.Int("count"); got != 5 {
		t.Errorf("Int(count) = %d", got)
	}
	if r.Str("missing") != "" {
		t.Errorf("Str(missing) should be empty")
	}
	if r.Int("nullable") != 0 {
		t.Errorf("Int(nullable) should be 0")
	}
}
