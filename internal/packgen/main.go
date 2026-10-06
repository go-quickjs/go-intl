// Command packgen packs the data directory into data.pack, which is what
// the intl package embeds (see internal/datapack).
//
//	go run ./internal/packgen
//
// Run it after any generator that writes under data/. A test fails while
// data.pack does not match the directory.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-quickjs/go-intl/internal/datapack"
	"github.com/go-quickjs/go-intl/internal/writeset"
)

func main() {
	// A file a generator's failed run left staged or kept is not data.
	err := filepath.WalkDir("data", func(path string, d fs.DirEntry, err error) error {
		if err == nil && (strings.HasSuffix(path, writeset.StagedSuffix) || strings.HasSuffix(path, writeset.OldSuffix)) {
			return fmt.Errorf("%s is left from a generator run that failed; restore the data (git checkout -- data) and delete it", path)
		}
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	pack, err := datapack.Build(os.DirFS("data"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	if err := writeset.WriteFile("data.pack", pack); err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "packgen: %d bytes\n", len(pack))
}
