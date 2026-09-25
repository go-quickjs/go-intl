// Command tzidgen writes the time zone names Temporal takes, as Node's
// Temporal has them: timezone_provider 0.2.3's identifier table, the
// version Node 26.10.0 builds Temporal with (see SOURCES.md).
//
//	go run ./internal/tzidgen <timezone_provider-0.2.3.crate>
//
// The crate is checked against crates.io's checksum before it is read.
// Its src/data/iana_normalizer.rs.data is data baked into Rust source: the
// names of the tz database it was built from, backzone included, sorted,
// and the links from a name to its primary one. Temporal takes a name in
// any case, spells it as the table does, and compares zones by their
// primary names; the zone's offsets are ICU's.
//
// data/temporalzones.bin has the table's tz version on its first line, then
// a line per name, followed by its primary name where it is a link:
//
//	version 2025c
//	Africa/Abidjan
//	Africa/Asmera Africa/Asmara
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// crateSHA256 is crates.io's checksum of timezone_provider 0.2.3.
const crateSHA256 = "c48f9b04628a2b813051e4dfe97c65281e49625eabd09ec343190e31e399a8c2"

const dataPath = "timezone_provider-0.2.3/src/data/iana_normalizer.rs.data"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/tzidgen <timezone_provider-0.2.3.crate>")
		os.Exit(2)
	}
	out, err := build(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "tzidgen:", err)
		os.Exit(1)
	}
	target := filepath.Join("data", "temporalzones.bin")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "tzidgen:", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, target); err != nil {
		fmt.Fprintln(os.Stderr, "tzidgen:", err)
		os.Exit(1)
	}
}

func build(crate string) ([]byte, error) {
	b, err := os.ReadFile(crate)
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != crateSHA256 {
		return nil, fmt.Errorf("%s has checksum %s, want %s", crate, got, crateSHA256)
	}
	src, err := readFile(b, dataPath)
	if err != nil {
		return nil, err
	}
	version, err := stringField(src, "version")
	if err != nil {
		return nil, err
	}
	namesBytes, err := byteString(src, "normalized_identifiers")
	if err != nil {
		return nil, err
	}
	names, err := varZeroVec16(namesBytes)
	if err != nil {
		return nil, err
	}
	linkBytes, err := byteString(src, "non_canonical_identifiers")
	if err != nil {
		return nil, err
	}
	if len(linkBytes)%8 != 0 {
		return nil, fmt.Errorf("the links are %d bytes, not pairs of u32", len(linkBytes))
	}
	primary := map[int]int{}
	for i := 0; i < len(linkBytes); i += 8 {
		from := int(binary.LittleEndian.Uint32(linkBytes[i:]))
		to := int(binary.LittleEndian.Uint32(linkBytes[i+4:]))
		if from >= len(names) || to >= len(names) {
			return nil, fmt.Errorf("a link from %d to %d of %d names", from, to, len(names))
		}
		primary[from] = to
	}
	seen := map[string]bool{}
	var out bytes.Buffer
	fmt.Fprintf(&out, "version %s\n", version)
	for i, n := range names {
		if i > 0 && names[i-1] >= n {
			return nil, fmt.Errorf("%q is out of order", n)
		}
		if lower := strings.ToLower(n); seen[lower] {
			return nil, fmt.Errorf("%q differs from another name only in case", n)
		} else {
			seen[lower] = true
		}
		out.WriteString(n)
		if p, ok := primary[i]; ok {
			out.WriteString(" " + names[p])
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// readFile is one file from the crate, a gzipped tar.
func readFile(crate []byte, path string) (string, error) {
	z, err := gzip.NewReader(bytes.NewReader(crate))
	if err != nil {
		return "", err
	}
	t := tar.NewReader(z)
	for {
		h, err := t.Next()
		if err == io.EOF {
			return "", fmt.Errorf("the crate has no %s", path)
		}
		if err != nil {
			return "", err
		}
		if h.Name == path {
			b, err := io.ReadAll(t)
			return string(b), err
		}
	}
}

// stringField is the string literal of a Cow::Borrowed field.
func stringField(src, field string) (string, error) {
	i := strings.Index(src, field+" :")
	if i < 0 {
		return "", fmt.Errorf("no field %s", field)
	}
	j := strings.Index(src[i:], "Borrowed(\"")
	if j < 0 {
		return "", fmt.Errorf("field %s is not a borrowed string", field)
	}
	rest := src[i+j+len("Borrowed(\""):]
	k := strings.IndexByte(rest, '"')
	if k < 0 {
		return "", fmt.Errorf("field %s does not end", field)
	}
	return rest[:k], nil
}

// byteString is the Rust byte string literal a field's data is baked in.
func byteString(src, field string) ([]byte, error) {
	i := strings.Index(src, field+" :")
	if i < 0 {
		return nil, fmt.Errorf("no field %s", field)
	}
	j := strings.Index(src[i:], "b\"")
	if j < 0 {
		return nil, fmt.Errorf("field %s has no byte string", field)
	}
	s := src[i+j+2:]
	var out []byte
	for k := 0; k < len(s); {
		c := s[k]
		switch {
		case c == '"':
			return out, nil
		case c != '\\':
			out = append(out, c)
			k++
			continue
		}
		if k+1 >= len(s) {
			break
		}
		switch e := s[k+1]; e {
		case 'x':
			if k+4 > len(s) {
				return nil, fmt.Errorf("field %s: a short \\x escape", field)
			}
			v, err := strconv.ParseUint(s[k+2:k+4], 16, 8)
			if err != nil {
				return nil, fmt.Errorf("field %s: %v", field, err)
			}
			out = append(out, byte(v))
			k += 4
		case 'n', 't', 'r', '0', '\\', '"', '\'':
			out = append(out, map[byte]byte{'n': '\n', 't': '\t', 'r': '\r', '0': 0, '\\': '\\', '"': '"', '\'': '\''}[e])
			k += 2
		default:
			return nil, fmt.Errorf("field %s: the escape \\%c", field, e)
		}
	}
	return nil, fmt.Errorf("field %s: the byte string does not end", field)
}

// varZeroVec16 decodes zerovec's VarZeroVec16 of strings: a u16 count,
// the end of each string but the last as u16, then the strings.
func varZeroVec16(b []byte) ([]string, error) {
	if len(b) < 2 {
		return nil, fmt.Errorf("a VarZeroVec of %d bytes", len(b))
	}
	n := int(binary.LittleEndian.Uint16(b))
	head := 2 + 2*(n-1)
	if n == 0 || len(b) < head {
		return nil, fmt.Errorf("a VarZeroVec of %d strings in %d bytes", n, len(b))
	}
	data := b[head:]
	starts := []int{0}
	for i := 0; i < n-1; i++ {
		starts = append(starts, int(binary.LittleEndian.Uint16(b[2+2*i:])))
	}
	starts = append(starts, len(data))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		if starts[i] > starts[i+1] || starts[i+1] > len(data) {
			return nil, fmt.Errorf("string %d runs from %d to %d of %d", i, starts[i], starts[i+1], len(data))
		}
		out[i] = string(data[starts[i]:starts[i+1]])
	}
	return out, nil
}
