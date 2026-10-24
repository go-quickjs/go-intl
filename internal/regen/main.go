// Command regen rebuilds every table go-intl embeds, from nothing but the
// pinned upstream sources, in one step:
//
//	go run ./internal/regen
//
// It downloads each source SOURCES.md pins into a cache, checks its sha256,
// runs every generator with the arguments it takes, and repacks data.pack. A source already in the cache with the
// right checksum is not downloaded again. Run from the repository root;
// on a clean checkout it reproduces data/ and data.pack byte for byte,
// which it reports at the end where git is at hand.
//
// Flags:
//
//	-cache dir   where the sources are kept (default: the user cache
//	             directory's go-intl-sources)
//	-j n         how many generators run at once (default: the CPUs)
//
// The generators that read nothing but what is vendored beside them
// (localegen, normgen) run too, so the whole of data/ is
// rebuilt. zonegen reads data/dates, which dategen writes, and so runs after
// it; packgen runs last. The rest write files of their own and run in
// parallel.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A source is one pinned upstream file: where it is kept in the cache,
// where it is fetched from, and its sha256.
type source struct {
	path, url, sha256 string
}

const (
	icuRelease = "https://github.com/unicode-org/icu/releases/download/release-78.3/"
	tzUpdate   = "https://raw.githubusercontent.com/unicode-org/icu-data/main/tzdata/icunew/2026c/44/"
	ucd        = "https://www.unicode.org/Public/17.0.0/ucd/"
	crates     = "https://static.crates.io/crates/"
)

// The cache's layout, which the generators' arguments name.
const (
	dataZip    = "icu4c-78.3-data.zip"
	sourcesTgz = "icu4c-78.3-sources.tgz"
	exportZip  = "icu4x-icuexportdata-78.3.zip"
	tzDir      = "icu-tz-2026c"
	ucdDir     = "ucd-17.0.0"
	calendar   = "icu_calendar-2.2.1.crate"
	tzProvider = "timezone_provider-0.2.3.crate"
)

func sources() []source {
	s := []source{
		{dataZip, icuRelease + dataZip, "9d8b3899096aeb83e4e21ef8a40fec9e03b28db18c48452efac882ce25a91e27"},
		{sourcesTgz, icuRelease + sourcesTgz, "3a2e7a47604ba702f345878308e6fefeca612ee895cf4a5f222e7955fabfe0c0"},
		{exportZip, icuRelease + exportZip, "eb63a12439f3fd9199886808275900229a5638fb0ee88d3c3c528eca7b811e60"},
		{tzDir + "/zoneinfo64.txt", tzUpdate + "zoneinfo64.txt", "9e4ac14d6217865fd3d94d288063a214c6dc1e53012f6624aeb70a8a517b30a3"},
		{tzDir + "/metaZones.txt", tzUpdate + "metaZones.txt", "55ba858327677222e9526acce34db10bee4fed14ccefc9df5b90452ab76f8c61"},
		{tzDir + "/timezoneTypes.txt", tzUpdate + "timezoneTypes.txt", "38b441f390473e353502a3fbd98f46a479fe3aef77d78fb4eef502623db46039"},
		{tzDir + "/windowsZones.txt", tzUpdate + "windowsZones.txt", "7addd9b95977b860d540d29796a655b8fe7247a0fdf9641f6f759b5442041312"},
		{ucdDir + "/Scripts.txt", ucd + "Scripts.txt", "9f5e50d3abaee7d6ce09480f325c706f485ae3240912527e651954d2d6b035bf"},
		{ucdDir + "/LineBreak.txt", ucd + "LineBreak.txt", "e6a18fa91f8f6a6f8e534b1d3f128c21ada45bfe152eb6b1bcc5e15fd8ac92e6"},
		{ucdDir + "/DerivedGeneralCategory.txt", ucd + "extracted/DerivedGeneralCategory.txt", "d62e5bab70ca74f099343f71224fa051cb1fdd61a1ab45c0488c44cfc0b6102e"},
		{ucdDir + "/PropList.txt", ucd + "PropList.txt", "130dcddcaadaf071008bdfce1e7743e04fdfbc910886f017d9f9ac931d8c64dd"},
		{calendar, crates + "icu_calendar/" + calendar, "a2b2acc6263f494f1df50685b53ff8e57869e47d5c6fe39c23d518ae9a4f3e45"},
		{tzProvider, crates + "timezone_provider/" + tzProvider, "c48f9b04628a2b813051e4dfe97c65281e49625eabd09ec343190e31e399a8c2"},
	}
	return s
}

// A job is one generator: its arguments, and the jobs it waits for.
type job struct {
	name  string
	args  []string
	after []string
}

func jobs(cache string) []job {
	at := func(p string) string { return filepath.Join(cache, filepath.FromSlash(p)) }
	return []job{
		{name: "localegen"},
		{name: "pluralgen", args: []string{at(dataZip)}},
		{name: "normgen"},
		{name: "aliasgen", args: []string{at(dataZip), at(tzDir)}},
		{name: "availgen", args: []string{at(dataZip), at(tzDir)}},
		{name: "tzgen", args: []string{at(tzDir), at(sourcesTgz)}},
		{name: "tzidgen", args: []string{at(tzProvider)}},
		{name: "temporalgen", args: []string{at(calendar)}},
		{name: "numbergen", args: []string{at(dataZip)}},
		{name: "listgen", args: []string{at(dataZip)}},
		{name: "unitgen", args: []string{at(dataZip)}},
		{name: "reltimegen", args: []string{at(dataZip)}},
		{name: "namegen", args: []string{at(dataZip)}},
		{name: "dategen", args: []string{at(dataZip)}},
		{name: "zonegen", args: []string{at(dataZip), at(tzDir)}, after: []string{"dategen"}},
		{name: "rbnfgen", args: []string{at(dataZip)}},
		{name: "collgen", args: []string{at(exportZip), at(dataZip), at(sourcesTgz)}},
		{name: "segmentgen", args: []string{at(sourcesTgz), at(dataZip), at(ucdDir)}},
		{name: "propgen", args: []string{at(ucdDir)}},
	}
}

func main() {
	defaultCache := "go-intl-sources"
	if dir, err := os.UserCacheDir(); err == nil {
		defaultCache = filepath.Join(dir, "go-intl-sources")
	}
	cache := flag.String("cache", defaultCache, "where the upstream sources are kept")
	parallel := flag.Int("j", runtime.NumCPU(), "how many generators run at once")
	flag.Parse()
	if err := run(*cache, max(*parallel, 1)); err != nil {
		fmt.Fprintln(os.Stderr, "regen:", err)
		os.Exit(1)
	}
}

func run(cache string, parallel int) error {
	if b, err := os.ReadFile("go.mod"); err != nil || !bytes.HasPrefix(b, []byte("module github.com/go-quickjs/go-intl\n")) {
		return errors.New("run from go-intl's repository root")
	}
	start := time.Now()
	if err := fetchAll(cache, sources()); err != nil {
		return err
	}
	fmt.Printf("sources ready in %s (%s)\n", cache, time.Since(start).Round(time.Second))
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}

	if err := generate(jobs(cache), parallel); err != nil {
		return err
	}
	if err := goRun("packgen"); err != nil {
		return err
	}
	fmt.Printf("data regenerated in %s\n", time.Since(start).Round(time.Second))
	report()
	return nil
}

// fetchAll downloads what the cache lacks, a few at a time.
func fetchAll(cache string, all []source) error {
	errs := make([]error, len(all))
	limit := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, s := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			errs[i] = fetch(filepath.Join(cache, filepath.FromSlash(s.path)), s)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// fetch puts a source at path unless it is there with the right checksum,
// writing a temporary file first so that a failed download leaves nothing.
func fetch(path string, s source) error {
	if sum, err := fileSHA256(path); err == nil && sum == s.sha256 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fmt.Println("fetching", s.url)
	resp, err := client.Get(s.url)
	if err != nil {
		return fmt.Errorf("%s: %w", s.url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", s.url, resp.Status)
	}
	tmp := path + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s: %w", s.url, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != s.sha256 {
		os.Remove(tmp)
		return fmt.Errorf("%s has sha256 %s, want %s", s.url, got, s.sha256)
	}
	return os.Rename(tmp, path)
}

// client gives up on a server that stops answering, rather than waiting
// for ever: the largest source is some tens of megabytes.
var client = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: time.Minute,
		TLSHandshakeTimeout:   30 * time.Second,
	},
}

// generatorTimeout is how long one generator may take; the slowest takes
// seconds.
const generatorTimeout = 10 * time.Minute

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unpack extracts an npm package's tarball into dir, as dir/package/...,
// unless a previous run extracted this archive, whose sha256 is sum, there
// and nothing has changed a file since. The marker it leaves says which
// archive, and each file's path, size and time of change, as a digest;
// a tree that differs from it, edited or cut short, is extracted again.
func unpack(archive, sum, dir string) error {
	done := filepath.Join(dir, ".unpacked")
	if b, err := os.ReadFile(done); err == nil {
		if tree, err := treeDigest(dir); err == nil && string(b) == marker(sum, tree) {
			return nil
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(z)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(h.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("%s holds %q, outside the package", archive, h.Name)
		}
		target := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
	tree, err := treeDigest(dir)
	if err != nil {
		return err
	}
	return os.WriteFile(done, []byte(marker(sum, tree)), 0o644)
}

func marker(sum, tree string) string { return "archive " + sum + "\ntree " + tree + "\n" }

// treeDigest is a digest of the files under dir but the marker: each one's
// path, size and time of change, in order.
func treeDigest(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == ".unpacked" {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// generate runs the generators, each once what it waits for is done, as
// many at once as parallel allows.
func generate(all []job, parallel int) error {
	done := map[string]chan struct{}{}
	for _, j := range all {
		done[j.name] = make(chan struct{})
	}
	limit := make(chan struct{}, parallel)
	errs := make([]error, len(all))
	var failed sync.Once
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i, j := range all {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done[j.name])
			for _, a := range j.after {
				select {
				case <-done[a]:
				case <-stop:
					return
				}
			}
			select {
			case <-stop:
				return
			default:
			}
			limit <- struct{}{}
			defer func() { <-limit }()
			if errs[i] = goRun(j.name, j.args...); errs[i] != nil {
				failed.Do(func() { close(stop) })
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// goRun runs one generator from the repository root, and says how long it
// took, or what it said when it failed.
func goRun(name string, args ...string) error {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), generatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "./internal/" + name}, args...)...)
	cmd.WaitDelay = 10 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("gave up after %s: %w", generatorTimeout, err)
		}
		return fmt.Errorf("%s: %w\n%s", name, err, out.String())
	}
	last := strings.TrimSpace(out.String())
	if i := strings.LastIndexByte(last, '\n'); i >= 0 {
		last = last[i+1:]
	}
	fmt.Printf("%-12s %6s  %s\n", name, time.Since(start).Round(100*time.Millisecond), last)
	return nil
}

// report says whether what was generated is what the repository holds.
func report() {
	out, err := exec.Command("git", "status", "--porcelain", "--", "data", "data.pack").Output()
	if err != nil {
		return
	}
	if len(bytes.TrimSpace(out)) == 0 {
		fmt.Println("data/ and data.pack are byte for byte what the repository holds")
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	fmt.Printf("%d paths differ from the repository:\n", len(lines))
	for i, l := range lines {
		if i == 20 {
			fmt.Printf("  ... and %d more\n", len(lines)-20)
			break
		}
		fmt.Println(" ", l)
	}
}
