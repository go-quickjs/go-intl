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
	"os"

	"github.com/go-quickjs/go-intl/internal/datapack"
)

func main() {
	pack, err := datapack.Build(os.DirFS("data"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	const target = "data.pack"
	if err := os.WriteFile(target+".tmp", pack, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		fmt.Fprintln(os.Stderr, "packgen:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "packgen: %d bytes\n", len(pack))
}
