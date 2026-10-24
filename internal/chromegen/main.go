// Command chromegen writes the data of the chrome package: an overlay of
// the intl package's data for each ICU a Chrome from 140 builds with, and
// the table of which Chrome takes which.
//
//	go run ./internal/chromegen [-cache dir]
//
// The Chromes and their ICU are testdata/chrome/pins.json (pins.js). For
// each distinct pair of ICU sources and data filter it fetches Chromium's
// source/data at the pin, filters/common.json, the filter a desktop Chrome
// builds its data through, and common/icudtl.dat, the data it built, into
// the cache, checks each against the git id pins.json gives it -- the
// sources' tree id computed from the archive as git computes it, so that
// the archive need not be byte for byte stable -- and runs profilegen over
// them, writing chrome/data/<tree>-<filter>.pack. chrome/data/versions.txt
// then names each Chrome's overlay: a line each, the version, the ICU pin
// and the overlay.
//
// The overlays are a chain, newest first. The newest is an overlay of the
// intl package's data, and each older one is an overlay of the data of the
// one after it. Neighbouring Chromes share most of what differs from ICU
// 78.3 -- Chromium's dictionaries and word rules, its filter -- and so
// each difference is kept once. Chrome's profile reads its overlay laid
// over the newer ones (chrome/packs.go's chain).
//
// ICU 78.3's sources, collation export and Unicode data, which the overlays
// share with the intl package's data, are in the same cache, where regen
// fetches them; regen runs chromegen last, and is the way to run it. Run it
// from the repository root, after the intl package's own data is
// current: the chain begins at it.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type pin struct {
	ICU        string   `json:"icu"`
	ICUVersion string   `json:"icuVersion"`
	DataTree   string   `json:"dataTree"`
	FilterBlob string   `json:"filterBlob"`
	DatBlob    string   `json:"datBlob"`
	Versions   []string `json:"versions"`
}

const gitiles = "https://chromium.googlesource.com/chromium/deps/icu/"

func main() {
	defaultCache := "go-intl-sources"
	if dir, err := os.UserCacheDir(); err == nil {
		defaultCache = filepath.Join(dir, "go-intl-sources")
	}
	cache := flag.String("cache", defaultCache, "where the sources are kept")
	flag.Parse()
	if err := run(*cache); err != nil {
		fmt.Fprintln(os.Stderr, "chromegen:", err)
		os.Exit(1)
	}
}

func run(cache string) error {
	b, err := os.ReadFile(filepath.FromSlash("testdata/chrome/pins.json"))
	if err != nil {
		return fmt.Errorf("run from go-intl's repository root: %w", err)
	}
	var pins struct {
		Pins []pin `json:"pins"`
	}
	if err := json.Unmarshal(b, &pins); err != nil {
		return fmt.Errorf("pins.json: %w", err)
	}
	// ICU 78.3's own inputs, which the overlays share with the intl
	// package's data, are regen's to fetch; chromegen fetches Chrome's.
	for _, name := range []string{"icu4c-78.3-sources.tgz", "icu4x-icuexportdata-78.3.zip", "ucd-17.0.0"} {
		if _, err := os.Stat(filepath.Join(cache, name)); err != nil {
			return fmt.Errorf("%s is not in the cache %s: run go run ./internal/regen -cache %s, which fetches it and then runs chromegen",
				name, cache, cache)
		}
	}
	out := filepath.Join("chrome", "data")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	var versions []string
	// An overlay is named by the sources and the filter, which the data is
	// built from; a pin that shares them builds the same data.
	made := map[string]string{}
	var firsts []pin // each overlay's first pin, in the pins' order
	for _, p := range pins.Pins {
		name := overlayName(p)
		if dat, ok := made[name]; ok && dat != p.DatBlob {
			return fmt.Errorf("%s: the data %s, though another pin builds %s from %s", p.ICU, short(p.DatBlob), short(dat), name)
		}
		if made[name] == "" {
			made[name] = p.DatBlob
			firsts = append(firsts, p)
		}
		for _, v := range p.Versions {
			versions = append(versions, v+" "+p.ICU+" "+name)
		}
	}
	// Newest first, each of the data the one before it made.
	against := "data"
	var chain []string
	for i := len(firsts) - 1; i >= 0; i-- {
		p := firsts[i]
		name := overlayName(p)
		tree, err := fetchTree(cache, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p.ICU, err)
		}
		dat, err := fetchDat(cache, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p.ICU, err)
		}
		fmt.Printf("chromegen: ICU %s at %s, overlay %s\n", p.ICUVersion, short(p.ICU), name)
		work := filepath.Join(cache, "chromium-icu", "work", name)
		cmd := exec.Command("go", "run", "./internal/profilegen", "-tree", tree,
			"-sources", filepath.Join(cache, "icu4c-78.3-sources.tgz"),
			"-export", filepath.Join(cache, "icu4x-icuexportdata-78.3.zip"),
			"-dat", dat, "-ucd", filepath.Join(cache, "ucd-17.0.0"),
			"-against", against, "-out", filepath.Join(out, name+".pack"), "-work", work)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: profilegen: %w", p.ICU, err)
		}
		against = filepath.Join(work, "data")
		chain = append(chain, name)
	}
	// What a previous run wrote for sources no pin names any more goes.
	entries, err := os.ReadDir(out)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if stem, ok := strings.CutSuffix(e.Name(), ".pack"); ok && made[stem] == "" {
			if err := os.Remove(filepath.Join(out, e.Name())); err != nil {
				return err
			}
		}
	}
	if err := os.WriteFile(filepath.Join(out, "versions.txt"), []byte(strings.Join(versions, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("chrome", "packs.go"), packsGo(chain), 0o644)
}

// packsGo is the chrome package's embedding of its data: versions.txt, and
// each overlay as a string, read where it lies, as the intl package's own
// data is, in the chain's order.
func packsGo(chain []string) []byte {
	var b strings.Builder
	b.WriteString("// Code generated by internal/chromegen; DO NOT EDIT.\n\npackage chrome\n\nimport _ \"embed\"\n\n")
	b.WriteString("//go:embed data/versions.txt\nvar versions string\n")
	for i, name := range chain {
		fmt.Fprintf(&b, "\n//go:embed data/%s.pack\nvar pack%d string\n", name, i)
	}
	b.WriteString("\n// chain is the overlays' names, newest first: each is an overlay of the\n" +
		"// data the ones before it make of the intl package's.\nfunc chain() []string {\n\treturn []string{\n")
	for _, name := range chain {
		fmt.Fprintf(&b, "\t\t%q,\n", name)
	}
	b.WriteString("\t}\n}\n")
	b.WriteString("\n// packOf is the overlay of that name.\nfunc packOf(name string) (string, bool) {\n\tswitch name {\n")
	for i, name := range chain {
		fmt.Fprintf(&b, "\tcase %q:\n\t\treturn pack%d, true\n", name, i)
	}
	b.WriteString("\t}\n\treturn \"\", false\n}\n")
	return []byte(b.String())
}

// overlayName is the name of a pin's overlay, its sources' and filter's.
func overlayName(p pin) string { return short(p.DataTree) + "-" + short(p.FilterBlob) }

func short(id string) string { return id[:10] }

// fetchTree is the directory profilegen reads a pin's sources from: data/,
// Chromium's source/data, and filter.json, its filters/common.json, each
// checked against its git id. A tree already fetched and checked is not
// fetched again.
func fetchTree(cache string, p pin) (string, error) {
	dir := filepath.Join(cache, "chromium-icu", short(p.DataTree)+"-"+short(p.FilterBlob))
	done := filepath.Join(dir, ".checked")
	want := p.DataTree + " " + p.FilterBlob + "\n"
	if b, err := os.ReadFile(done); err == nil && string(b) == want {
		return dir, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	archive, err := get(gitiles + "+archive/" + p.ICU + "/source/data.tar.gz")
	if err != nil {
		return "", err
	}
	id, err := extract(archive, filepath.Join(dir, "data"))
	if err != nil {
		return "", err
	}
	if id != p.DataTree {
		return "", fmt.Errorf("source/data is the git tree %s, pins.json says %s", id, p.DataTree)
	}
	encoded, err := get(gitiles + "+/" + p.ICU + "/filters/common.json?format=TEXT")
	if err != nil {
		return "", err
	}
	filter, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return "", fmt.Errorf("filters/common.json: %w", err)
	}
	if id := hex.EncodeToString(object("blob", filter)); id != p.FilterBlob {
		return "", fmt.Errorf("filters/common.json is the git blob %s, pins.json says %s", id, p.FilterBlob)
	}
	if err := os.WriteFile(filepath.Join(dir, "filter.json"), filter, 0o644); err != nil {
		return "", err
	}
	return dir, os.WriteFile(done, []byte(want), 0o644)
}

// fetchDat is the file a pin's common/icudtl.dat is kept in, checked
// against its git id.
func fetchDat(cache string, p pin) (string, error) {
	file := filepath.Join(cache, "chromium-icu", "icudtl-"+short(p.DatBlob)+".dat")
	if b, err := os.ReadFile(file); err == nil && hex.EncodeToString(object("blob", b)) == p.DatBlob {
		return file, nil
	}
	encoded, err := get(gitiles + "+/" + p.ICU + "/common/icudtl.dat?format=TEXT")
	if err != nil {
		return "", err
	}
	dat, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return "", fmt.Errorf("common/icudtl.dat: %w", err)
	}
	if id := hex.EncodeToString(object("blob", dat)); id != p.DatBlob {
		return "", fmt.Errorf("common/icudtl.dat is the git blob %s, pins.json says %s", id, p.DatBlob)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	return file, os.WriteFile(file, dat, 0o644)
}

func get(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Minute}
	var last error
	for i := 0; i < 4; i++ {
		if i > 0 {
			time.Sleep(time.Duration(i) * 2 * time.Second)
		}
		resp, err := client.Get(url)
		if err != nil {
			last = err
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && resp.StatusCode == http.StatusOK {
			return b, nil
		}
		last = fmt.Errorf("%s: %s", url, resp.Status)
		if err != nil {
			last = err
		}
	}
	return nil, last
}

// An entry is a file or directory of a tree, as git keeps it.
type entry struct {
	mode string // "100644", "100755", "40000"
	id   []byte
}

// extract writes a gzipped tar of a tree into dir and returns the tree's
// git id, which it computes from the archive: each file's blob, with the
// mode its header gives, and each directory's tree.
func extract(archive []byte, dir string) (string, error) {
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	trees := map[string]map[string]entry{"": {}}
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		name := strings.TrimSuffix(path.Clean(strings.TrimPrefix(h.Name, "./")), "/")
		if name == "." || name == "" {
			continue
		}
		if strings.HasPrefix(name, "../") || path.IsAbs(name) {
			return "", fmt.Errorf("an archive entry outside its tree: %s", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			addDir(trees, name)
		case tar.TypeReg:
			b, err := io.ReadAll(tr)
			if err != nil {
				return "", err
			}
			parent, base := path.Split(name)
			parent = strings.TrimSuffix(parent, "/")
			addDir(trees, parent)
			mode := "100644"
			if h.Mode&0o111 != 0 {
				mode = "100755"
			}
			trees[parent][base] = entry{mode: mode, id: object("blob", b)}
			p := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(p, b, 0o644); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("%s: an archive entry of type %c", h.Name, h.Typeflag)
		}
	}
	return hex.EncodeToString(treeID(trees, "")), nil
}

func addDir(trees map[string]map[string]entry, name string) {
	for name != "" {
		if _, ok := trees[name]; ok {
			return
		}
		trees[name] = map[string]entry{}
		name = path.Dir(name)
		if name == "." {
			name = ""
		}
	}
}

// treeID is a directory's git tree id: its entries sorted as git sorts
// them, a directory as if its name ended in "/".
func treeID(trees map[string]map[string]entry, name string) []byte {
	entries := trees[name]
	for child := range trees {
		if child != "" && path.Dir(child) == name || name == "" && child != "" && !strings.Contains(child, "/") {
			entries[path.Base(child)] = entry{mode: "40000", id: treeID(trees, child)}
		}
	}
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	key := func(n string) string {
		if entries[n].mode == "40000" {
			return n + "/"
		}
		return n
	}
	sort.Slice(names, func(i, j int) bool { return key(names[i]) < key(names[j]) })
	var body []byte
	for _, n := range names {
		body = append(body, entries[n].mode+" "+n+"\x00"...)
		body = append(body, entries[n].id...)
	}
	return object("tree", body)
}

// object is a git object's id.
func object(kind string, body []byte) []byte {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d\x00", kind, len(body))
	h.Write(body)
	return h.Sum(nil)
}
