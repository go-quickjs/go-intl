package intl

import (
	"os"
	"testing"

	"github.com/go-quickjs/go-intl/internal/datapack"
)

// TestEmbeddedPackIsCurrent holds data.pack, which is what the package
// embeds, to the data directory the generators write. Run
// go run ./internal/packgen after any generator.
func TestEmbeddedPackIsCurrent(t *testing.T) {
	want, err := datapack.Build(os.DirFS("data"))
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != embeddedPack {
		t.Fatal("data.pack does not match data/: run go run ./internal/packgen")
	}
}
