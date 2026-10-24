package datawrite

import (
	"fmt"
	"os"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// NewPool starts the pool a set's locales share. With seed "", the pool is
// new. With seed a pool file, it numbers its parts after that pool's
// (blob.SeedPool): every part the seed has keeps its number, so a table
// that holds the same strings as it did when the seed was written is the
// same bytes. That is how a generator's output is compared with what it
// wrote before when the order it meets the strings in has changed, and how
// an overlay's tables are written over the data they differ from.
func NewPool(version byte, seed string) (*blob.Pool, error) {
	if seed == "" {
		return blob.NewPool(version), nil
	}
	b, err := os.ReadFile(seed)
	if err != nil {
		return nil, err
	}
	base, err := blob.ReadShared(b, version)
	if err != nil {
		return nil, fmt.Errorf("the seed %s: %w", seed, err)
	}
	return blob.SeedPool(version, base), nil
}
