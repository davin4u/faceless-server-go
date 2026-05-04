package namegen

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/davin4u/faceless-server-go/internal/db"
)

var (
	rngMu sync.Mutex
	rng   = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// GenerateDisplayName returns "<Adjective> <Noun> <NN>" not currently in users.display_name.
// Tries up to 50 times.
func GenerateDisplayName(ctx context.Context, d db.DB) (string, error) {
	for attempt := 0; attempt < 50; attempt++ {
		rngMu.Lock()
		adj := Adjectives[rng.Intn(len(Adjectives))]
		noun := Nouns[rng.Intn(len(Nouns))]
		num := rng.Intn(99) + 1
		rngMu.Unlock()

		name := fmt.Sprintf("%s %s %02d", adj, noun, num)
		row, err := d.Get(ctx, "SELECT 1 FROM users WHERE display_name = ?", name)
		if err != nil {
			return "", err
		}
		if row == nil {
			return name, nil
		}
	}
	return "", errors.New("failed to generate unique display name")
}
