// Command collgen writes the collation tables the intl package carries.
//
// It reads three artifacts of the ICU 78.3 release, none vendored, each
// checked against the checksum SOURCES.md pins:
//
//	curl -sLO https://github.com/unicode-org/icu/releases/download/release-78.3/icu4x-icuexportdata-78.3.zip
//	curl -sLO https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-data.zip
//	curl -sLO https://github.com/unicode-org/icu/releases/download/release-78.3/icu4c-78.3-sources.tgz
//	go run ./internal/collgen icu4x-icuexportdata-78.3.zip icu4c-78.3-data.zip icu4c-78.3-sources.tgz
//
// The export holds the collation tables themselves, as ICU builds them from
// CLDR's rules: a trie from each character to its collation element, the
// expansion and context tables it points into, and the settings each tailoring
// carries. Building those tables from the rules is ICU's rule compiler; they
// are inputs to the collation algorithm, not orderings, and nothing here sorts
// anything.
//
// The export does not say how the collation locales inherit from one another,
// and ICU's collation tree is not the ordinary one: Norwegian Bokmål collates
// as Norwegian, Cantonese as traditional Chinese, and simplified and
// traditional Chinese default to different collations. That is in ICU's
// collation sources, data/coll in the data archive -- a LOCALE_DEPS.json of
// aliases and parents, and a default type in the few locales that name one --
// and it is read from there.
//
// The collation types that tailor the conjoining Hangul jamo -- the search
// collations, Korean's searchjl -- are taken from ICU's compiled data
// instead, where the export leaves them incomplete: see compiled.go.
//
// Han characters are ordered by radical and stroke, which is the "unihan"
// flavor of the export. The "implicithan" flavor orders them by code point
// block, as the Unicode Collation Algorithm's default does; ICU4X ships that
// one because it is smaller, but ICU4C and so Node do not: Node sorts U+3400
// before U+9FA0, which only radical-and-stroke order does.
package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	intl "github.com/go-quickjs/go-intl"
	"github.com/go-quickjs/go-intl/internal/colldata"
	"github.com/go-quickjs/go-intl/internal/datawrite"
	"github.com/go-quickjs/go-intl/internal/icudat"
	"github.com/go-quickjs/go-intl/internal/icusrc"
)

const hanFlavor = "unihan"

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/collgen <icu4x-icuexportdata-78.3.zip> <icu4c-78.3-data.zip> <icu4c-78.3-sources.tgz>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3]); err != nil {
		fmt.Fprintln(os.Stderr, "collgen:", err)
		os.Exit(1)
	}
}

func run(exportPath, dataPath, sourcesPath string) error {
	export, err := icusrc.Open(exportPath, icusrc.ExportSHA256)
	if err != nil {
		return err
	}
	sources, err := icusrc.Open(dataPath, icusrc.DataSHA256)
	if err != nil {
		return err
	}

	tables, err := readExport(export)
	if err != nil {
		return err
	}
	tree, defaults, err := readSources(sources)
	if err != nil {
		return err
	}
	dat, err := icudat.Read(sourcesPath)
	if err != nil {
		return err
	}

	// Build everything before writing anything.
	rootFiles, ok := tables["root"]
	if !ok {
		return fmt.Errorf("the export has no root collation")
	}
	root, err := buildRoot(rootFiles)
	if err != nil {
		return fmt.Errorf("root: %w", err)
	}

	installed := map[string]bool{}
	for _, name := range tree.Installed {
		installed[name] = true
	}
	built := map[string][]byte{}
	names := map[string]bool{}
	for name := range tables {
		names[name] = true
	}
	for name := range defaults {
		names[name] = true
	}
	for name := range names {
		if name != "root" && !installed[name] {
			return fmt.Errorf("the export has %s, which ICU's collation sources do not", name)
		}
		compiled, err := compiledCollations(dat, name)
		if err != nil {
			return err
		}
		loc, err := buildLocale(name, tables[name], defaults[name], compiled)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		tag, err := dataTag(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collgen: skipping %s: %v\n", name, err)
			continue
		}
		built[tag] = colldata.EncodeLocale(loc)
	}
	if err := checkDefaults(tables, defaults); err != nil {
		return err
	}

	// Aliases and parents are written as BCP 47 tags, like the file names.
	var bcp colldata.Tree
	for _, p := range tree.Aliases {
		from, err1 := dataTag(p[0])
		to, err2 := dataTag(p[1])
		if err1 != nil || err2 != nil {
			continue
		}
		bcp.Aliases = append(bcp.Aliases, [2]string{from, to})
	}
	for _, p := range tree.Parents {
		from, err1 := dataTag(p[0])
		to, err2 := dataTag(p[1])
		if err1 != nil || err2 != nil {
			continue
		}
		bcp.Parents = append(bcp.Parents, [2]string{from, to})
	}
	for _, name := range tree.Installed {
		if tag, err := dataTag(name); err == nil {
			bcp.Installed = append(bcp.Installed, tag)
		}
	}
	sort.Strings(bcp.Installed)

	out := filepath.Join("data", "collation")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := write(filepath.Join("data", "collationroot.bin"), colldata.EncodeRoot(root)); err != nil {
		return err
	}
	if err := write(filepath.Join("data", "collationtree.bin"), colldata.EncodeTree(&bcp)); err != nil {
		return err
	}
	// The root's collations are "und", where every data set kept per locale
	// has its root.
	if err := datawrite.Locales(out, built); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join("data", "collation.bin")); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintf(os.Stderr, "collgen: %d locales, %d installed, %d aliases, %d parents\n",
		len(built), len(bcp.Installed), len(bcp.Aliases), len(bcp.Parents))
	return nil
}

// A tomlFile is one of the export's files, taken apart into its keys. A table
// header -- the export writes one, [trie] -- prefixes the keys under it.
type tomlFile map[string]string

// exportFiles holds one locale's files, keyed by type and then by suffix:
// "standard" and then "data", "meta", "reord".
type exportFiles map[string]map[string]tomlFile

func readExport(z *zip.ReadCloser) (map[string]exportFiles, error) {
	prefix := "collation/" + hanFlavor + "/"
	out := map[string]exportFiles{}
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".toml") {
			continue
		}
		base := strings.TrimSuffix(path.Base(f.Name), ".toml")
		parts := strings.Split(base, "_")
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s: not locale_type_part", f.Name)
		}
		suffix := parts[len(parts)-1]
		kind := parts[len(parts)-2]
		locale := strings.Join(parts[:len(parts)-2], "_")
		t, err := readTOML(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if out[locale] == nil {
			out[locale] = exportFiles{}
		}
		if out[locale][kind] == nil {
			out[locale][kind] = map[string]tomlFile{}
		}
		out[locale][kind][suffix] = t
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no %s collation files in the export", hanFlavor)
	}
	return out, nil
}

// readTOML reads the small part of TOML the export is written in: comments,
// "key = value" with a number, a boolean or a bracketed list of them that may
// run over several lines, and table headers.
func readTOML(f *zip.File) (tomlFile, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	out := tomlFile{}
	table := ""
	var key string
	var list strings.Builder
	inList := false
	s := bufio.NewScanner(rc)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if inList {
			if strings.HasSuffix(line, "]") {
				list.WriteString(strings.TrimSuffix(line, "]"))
				out[key] = list.String()
				inList = false
				continue
			}
			list.WriteString(line)
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			table = strings.Trim(line, "[]") + "."
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("cannot read %q", line)
		}
		key = table + strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "[") {
			v = strings.TrimPrefix(v, "[")
			if strings.HasSuffix(v, "]") {
				out[key] = strings.TrimSuffix(v, "]")
				continue
			}
			list.Reset()
			list.WriteString(v)
			inList = true
			continue
		}
		out[key] = v
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	if inList {
		return nil, fmt.Errorf("the list %s never ends", key)
	}
	return out, nil
}

func (t tomlFile) number(key string) (int64, error) {
	v, ok := t[key]
	if !ok {
		return 0, fmt.Errorf("no %s", key)
	}
	return strconv.ParseInt(v, 0, 64)
}

func (t tomlFile) numbers(key string) ([]int64, error) {
	v, ok := t[key]
	if !ok {
		return nil, fmt.Errorf("no %s", key)
	}
	var out []int64
	for _, field := range strings.Split(v, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		n, err := strconv.ParseInt(field, 0, 64)
		if err != nil {
			// A 64-bit element written in hexadecimal can exceed the
			// signed range.
			u, uerr := strconv.ParseUint(field, 0, 64)
			if uerr != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			n = int64(u)
		}
		out = append(out, n)
	}
	return out, nil
}

func (t tomlFile) bools(key string) ([]bool, error) {
	v, ok := t[key]
	if !ok {
		return nil, fmt.Errorf("no %s", key)
	}
	var out []bool
	for _, field := range strings.Split(v, ",") {
		switch strings.TrimSpace(field) {
		case "true":
			out = append(out, true)
		case "false":
			out = append(out, false)
		case "":
		default:
			return nil, fmt.Errorf("%s: %q is not a boolean", key, field)
		}
	}
	return out, nil
}

func buildRoot(files exportFiles) (*colldata.Root, error) {
	standard := files["standard"]
	if standard["data"] == nil || standard["prim"] == nil {
		return nil, fmt.Errorf("the root needs its data and its special primaries")
	}
	d, err := buildData(standard["data"])
	if err != nil {
		return nil, err
	}
	root := &colldata.Root{Data: *d}
	last, err := standard["prim"].numbers("last_primaries")
	if err != nil {
		return nil, err
	}
	if len(last) != len(root.LastPrimaries) {
		return nil, fmt.Errorf("%d last primaries, want %d", len(last), len(root.LastPrimaries))
	}
	for i, p := range last {
		root.LastPrimaries[i] = uint16(p)
	}
	numeric, err := standard["prim"].number("numeric_primary")
	if err != nil {
		return nil, err
	}
	root.NumericPrimary = byte(numeric)
	if standard["dia"] == nil {
		return nil, fmt.Errorf("the root has no diacritics table")
	}
	if root.Diacritics, err = diacritics(standard["dia"]); err != nil {
		return nil, err
	}
	if standard["jamo"] == nil {
		return nil, fmt.Errorf("the root has no jamo table")
	}
	jamo, err := standard["jamo"].numbers("ce32s")
	if err != nil {
		return nil, err
	}
	if len(jamo) != 0x100 {
		return nil, fmt.Errorf("%d jamo, want 256", len(jamo))
	}
	for _, v := range jamo {
		root.Jamo = append(root.Jamo, uint32(v))
	}
	return root, nil
}

func buildLocale(name string, files exportFiles, def string, compiled map[string]*colldata.Data) (*colldata.Locale, error) {
	loc := &colldata.Locale{Default: def}
	kinds := make([]string, 0, len(files))
	for kind := range files {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		parts := files[kind]
		meta := parts["meta"]
		if meta == nil {
			return nil, fmt.Errorf("%s has no settings", kind)
		}
		bits, err := meta.number("bits")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", kind, err)
		}
		c := colldata.Collation{Name: kind, Meta: uint32(bits)}
		// The root's standard data is the root table, written separately.
		if parts["data"] != nil && !(name == "root" && kind == "standard") {
			d, err := buildData(parts["data"])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", kind, err)
			}
			c.Data = d
		}
		if d, ok := compiled[kind]; ok {
			c.Data = d
		}
		if parts["reord"] != nil {
			o, err := buildReordering(parts["reord"])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", kind, err)
			}
			c.Reordering = o
		}
		// Only a type that says it tailors the diacritics has its own
		// table; the others are copies of the root's.
		if parts["dia"] != nil && bits&colldata.TailoredDiacriticsBit != 0 {
			if c.Diacritics, err = diacritics(parts["dia"]); err != nil {
				return nil, fmt.Errorf("%s: %w", kind, err)
			}
		}
		loc.Collations = append(loc.Collations, c)
	}
	return loc, nil
}

func buildData(t tomlFile) (*colldata.Data, error) {
	var d colldata.Data
	width, err := t.number("trie.valueWidth")
	if err != nil {
		return nil, err
	}
	if width != 1 {
		return nil, fmt.Errorf("a trie of value width %d, not 32 bits", width)
	}
	kind, err := t.number("trie.type")
	if err != nil {
		return nil, err
	}
	d.Trie.Fast = kind == 0
	high, err := t.number("trie.highStart")
	if err != nil {
		return nil, err
	}
	d.Trie.HighStart = rune(high)
	index, err := t.numbers("trie.index")
	if err != nil {
		return nil, err
	}
	var index16 []uint16
	for _, v := range index {
		index16 = append(index16, uint16(v))
	}
	d.Trie.Index = colldata.MakeU16s(index16)
	values, err := t.numbers("trie.data_32")
	if err != nil {
		return nil, err
	}
	var values32 []uint32
	for _, v := range values {
		values32 = append(values32, uint32(v))
	}
	d.Trie.Data = colldata.MakeU32s(values32)
	if n, err := t.number("trie.indexLength"); err != nil || int(n) != d.Trie.Index.Len() {
		return nil, fmt.Errorf("the trie index has %d entries, not the %d it says", d.Trie.Index.Len(), n)
	}
	if n, err := t.number("trie.dataLength"); err != nil || int(n) != d.Trie.Data.Len() {
		return nil, fmt.Errorf("the trie data has %d entries, not the %d it says", d.Trie.Data.Len(), n)
	}
	ce32s, err := t.numbers("ce32s")
	if err != nil {
		return nil, err
	}
	var ce32s32 []uint32
	for _, v := range ce32s {
		ce32s32 = append(ce32s32, uint32(v))
	}
	d.CE32s = colldata.MakeU32s(ce32s32)
	ces, err := t.numbers("ces")
	if err != nil {
		return nil, err
	}
	var ces64 []uint64
	for _, v := range ces {
		ces64 = append(ces64, uint64(v))
	}
	d.CEs = colldata.MakeU64s(ces64)
	contexts, err := t.numbers("contexts")
	if err != nil {
		return nil, err
	}
	var contexts16 []uint16
	for _, v := range contexts {
		contexts16 = append(contexts16, uint16(v))
	}
	d.Contexts = colldata.MakeU16s(contexts16)
	return &d, nil
}

func diacritics(t tomlFile) ([]uint16, error) {
	values, err := t.numbers("secondaries")
	if err != nil {
		return nil, err
	}
	out := make([]uint16, len(values))
	for i, v := range values {
		out[i] = uint16(v)
	}
	return out, nil
}

func buildReordering(t tomlFile) (*colldata.Reordering, error) {
	var o colldata.Reordering
	high, err := t.number("min_high_no_reorder")
	if err != nil {
		return nil, err
	}
	o.MinHighNoReorder = uint32(high)
	table, err := t.numbers("reorder_table")
	if err != nil {
		return nil, err
	}
	if len(table) != len(o.Table) {
		return nil, fmt.Errorf("a reordering table of %d entries", len(table))
	}
	for i, v := range table {
		o.Table[i] = byte(v)
	}
	ranges, err := t.numbers("reorder_ranges")
	if err != nil {
		return nil, err
	}
	for _, v := range ranges {
		o.Ranges = append(o.Ranges, uint32(v))
	}
	return &o, nil
}

// localeDeps is ICU's LOCALE_DEPS.json for the collation tree.
type localeDeps struct {
	Aliases map[string]string `json:"aliases"`
	Parents map[string]string `json:"parents"`
}

var (
	aliasRE   = regexp.MustCompile(`"%%ALIAS"`)
	defaultRE = regexp.MustCompile(`(?m)^\s*default\{"([a-z0-9]+)"\}`)
)

// readSources reads ICU's collation sources for what the export leaves out:
// which locales exist, how they inherit, and what each one defaults to.
func readSources(z *zip.ReadCloser) (*colldata.Tree, map[string]string, error) {
	const dir = "data/coll/"
	tree := &colldata.Tree{}
	defaults := map[string]string{}
	var deps localeDeps
	found := false
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, dir) || path.Dir(f.Name)+"/" != dir {
			continue
		}
		base := path.Base(f.Name)
		body, err := readAll(f)
		if err != nil {
			return nil, nil, err
		}
		if base == "LOCALE_DEPS.json" {
			if err := json.Unmarshal(stripComments(body), &deps); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			found = true
			continue
		}
		name, ok := strings.CutSuffix(base, ".txt")
		if !ok {
			continue
		}
		// The legacy variant locales -- "de__PHONEBOOK" and the "de_" it
		// is aliased from -- predate BCP 47 and nothing reaches them.
		if strings.Contains(name, "__") || strings.HasSuffix(name, "_") {
			continue
		}
		if aliasRE.Match(body) {
			continue
		}
		if m := defaultRE.FindSubmatch(body); m != nil {
			defaults[name] = string(m[1])
		}
		if name != "root" {
			tree.Installed = append(tree.Installed, name)
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("no %sLOCALE_DEPS.json in the data archive", dir)
	}
	for from, to := range deps.Aliases {
		tree.Aliases = append(tree.Aliases, [2]string{from, to})
	}
	for from, to := range deps.Parents {
		tree.Parents = append(tree.Parents, [2]string{from, to})
	}
	sort.Slice(tree.Aliases, func(i, j int) bool { return tree.Aliases[i][0] < tree.Aliases[j][0] })
	sort.Slice(tree.Parents, func(i, j int) bool { return tree.Parents[i][0] < tree.Parents[j][0] })
	sort.Strings(tree.Installed)
	if defaults["root"] != "standard" {
		return nil, nil, fmt.Errorf("the root's default collation is %q, not standard", defaults["root"])
	}
	return tree, defaults, nil
}

// checkDefaults makes sure every default names a type that exists somewhere.
// It cannot check inheritance without the tree, so it checks the weaker thing:
// a default that names a type no locale has is certainly broken.
func checkDefaults(tables map[string]exportFiles, defaults map[string]string) error {
	have := map[string]bool{}
	for _, files := range tables {
		for kind := range files {
			have[kind] = true
		}
	}
	for name, def := range defaults {
		if !have[def] {
			return fmt.Errorf("%s defaults to %s, which no locale defines", name, def)
		}
	}
	return nil
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// stripComments removes the // lines ICU heads its JSON with.
func stripComments(b []byte) []byte {
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// dataTag turns an ICU locale name into the BCP 47 form data files are named
// by. A name with a variant, which a data locale cannot hold, is refused.
func dataTag(name string) (string, error) {
	if name == "root" {
		return "und", nil
	}
	loc, err := intl.ParseLocale(strings.ReplaceAll(name, "_", "-"))
	if err != nil {
		return "", err
	}
	d := loc.Data()
	if d.String() != loc.String() {
		return "", fmt.Errorf("%s has more than a data locale holds", name)
	}
	return d.String(), nil
}

func write(target string, data []byte) error {
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		os.Remove(temporary)
		return err
	}
	return nil
}
