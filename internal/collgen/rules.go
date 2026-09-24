package main

import (
	"archive/zip"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The one thing read from ICU's collation rules rather than its tables.
//
// The export leaves the conjoining Hangul jamo out of every tailoring's trie,
// and ships only the root's jamo table, so a tailoring that changes a jamo
// loses the change. ICU4X lives with that; Node does not have to, and the
// search collations do change them: a trailing consonant is made equal to the
// leading one, and a double consonant to the pair it doubles, so that a search
// finds a syllable however its consonants were typed.
//
// Those rules have one shape, a reset to a run of jamo followed by jamo equal
// to it:
//
//	&ᄀᄀ =ᄁ =ᆩ
//
// which says ᄁ and ᆩ weigh exactly what ᄀᄀ weighs. That is an input the
// collator can apply by itself -- weigh the run -- so it is carried as the run
// and not as weights. Anything else touching a jamo is refused rather than
// guessed at. Korean's "searchjl" collation uses weights of its own, which only
// ICU's rule compiler can allocate, and is left out: see PLAN.md.

// icuType turns a BCP 47 collation type, as an import names it, into the
// name the source files use.
func icuType(bcp string) string {
	switch bcp {
	case "phonebk":
		return "phonebook"
	case "trad":
		return "traditional"
	case "dict":
		return "dictionary"
	}
	return bcp
}

func isJamo(r rune) bool { return r >= 0x1100 && r <= 0x11ff }

// readRuleSources reads every collation source file, by locale.
func readRuleSources(z *zip.ReadCloser) (map[string]string, error) {
	const dir = "data/coll/"
	out := map[string]string{}
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, dir) || !strings.HasSuffix(f.Name, ".txt") {
			continue
		}
		body, err := readAll(f)
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(path.Base(f.Name), ".txt")] = string(body)
	}
	return out, nil
}

// searchJamo returns the jamo a collation type makes equal to runs of jamo,
// following its imports.
func searchJamo(sources map[string]string, locale, kind string) (map[rune]string, error) {
	out := map[rune]string{}
	if err := collectJamo(sources, locale, kind, out, 0); err != nil {
		return nil, fmt.Errorf("%s-u-co-%s: %w", locale, kind, err)
	}
	for r, run := range out {
		for _, x := range run {
			if _, ok := out[x]; ok {
				return nil, fmt.Errorf("%U is equal to a run containing %U, which is itself remapped", r, x)
			}
		}
	}
	return out, nil
}

func collectJamo(sources map[string]string, locale, kind string, out map[rune]string, depth int) error {
	if depth > 8 {
		return fmt.Errorf("imports nest too deeply")
	}
	body, ok := sources[locale]
	if !ok {
		return fmt.Errorf("no collation source for %s", locale)
	}
	rules, err := ruleText(body, kind)
	if err != nil {
		return err
	}
	var reset []rune
	for _, tok := range tokenize(rules) {
		switch tok.op {
		case "import":
			imp, impKind := tok.text, "standard"
			if base, co, ok := strings.Cut(imp, "-u-co-"); ok {
				imp, impKind = base, icuType(co)
			}
			if imp == "und" {
				imp = "root"
			}
			if err := collectJamo(sources, strings.ReplaceAll(imp, "-", "_"), impKind, out, depth+1); err != nil {
				return err
			}
		case "&":
			reset = []rune(tok.text)
		default:
			text := []rune(tok.text)
			touches := false
			for _, r := range text {
				touches = touches || isJamo(r)
			}
			resetJamo := len(reset) > 0
			for _, r := range reset {
				resetJamo = resetJamo && isJamo(r)
			}
			if !touches && !resetJamo {
				continue
			}
			if tok.op != "=" || len(text) != 1 || !isJamo(text[0]) || !resetJamo {
				return fmt.Errorf("a jamo rule of a shape not understood: &%s %s%s",
					string(reset), tok.op, tok.text)
			}
			// A reset means what the rules so far make it mean, so a jamo in
			// it that an earlier rule made equal to a run is that run.
			var run strings.Builder
			for _, r := range reset {
				if earlier, ok := out[r]; ok {
					run.WriteString(earlier)
				} else {
					run.WriteRune(r)
				}
			}
			out[text[0]] = run.String()
		}
	}
	return nil
}

// ruleText returns a collation type's rules from a source file: the strings
// of its Sequence, joined and unescaped.
func ruleText(body, kind string) (string, error) {
	start := regexp.MustCompile(`(?m)^        ` + regexp.QuoteMeta(kind) + `\{`).FindStringIndex(body)
	if start == nil {
		return "", fmt.Errorf("no %s collation", kind)
	}
	rest := body[start[1]:]
	seq := strings.Index(rest, "Sequence{")
	if seq < 0 {
		return "", fmt.Errorf("the %s collation has no rules", kind)
	}
	rest = rest[seq+len("Sequence{"):]
	var b strings.Builder
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		if rest == "" {
			return "", fmt.Errorf("the %s rules never end", kind)
		}
		if rest[0] == '}' {
			return b.String(), nil
		}
		if rest[0] != '"' {
			return "", fmt.Errorf("unexpected %q in the %s rules", rest[:1], kind)
		}
		s, n, err := quoted(rest)
		if err != nil {
			return "", err
		}
		b.WriteString(s)
		rest = rest[n:]
	}
}

// quoted reads one ICU resource string, with its escapes, and returns it and
// how much of the input it took.
func quoted(s string) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); {
		switch c := s[i]; c {
		case '"':
			return b.String(), i + 1, nil
		case '\\':
			if i+1 >= len(s) {
				return "", 0, fmt.Errorf("a string ends in a backslash")
			}
			switch s[i+1] {
			case 'u', 'U':
				n := 4
				if s[i+1] == 'U' {
					n = 8
				}
				if i+2+n > len(s) {
					return "", 0, fmt.Errorf("a short escape")
				}
				v, err := strconv.ParseUint(s[i+2:i+2+n], 16, 32)
				if err != nil {
					return "", 0, err
				}
				b.WriteRune(rune(v))
				i += 2 + n
			default:
				// Any other escaped character is itself.
				b.WriteByte(s[i+1])
				i += 2
			}
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			b.WriteRune(r)
			i += size
		}
	}
	return "", 0, fmt.Errorf("a string never ends")
}

type ruleToken struct {
	op   string // "&", "<", "<<", "<<<", "<<<<", "=", or "import"
	text string
}

// tokenize takes collation rules apart into resets and relations. It is not a
// rule compiler: settings in brackets other than imports are skipped, and
// quoting is honoured only so that a quoted operator is not read as one.
func tokenize(rules string) []ruleToken {
	var out []ruleToken
	var cur *ruleToken
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	runes := []rune(rules)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '[':
			depth, j := 0, i
			for ; j < len(runes); j++ {
				if runes[j] == '[' {
					depth++
				} else if runes[j] == ']' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			setting := string(runes[i+1 : min(j, len(runes))])
			if name, ok := strings.CutPrefix(setting, "import "); ok {
				flush()
				out = append(out, ruleToken{op: "import", text: strings.TrimSpace(name)})
			} else if cur != nil {
				// "[before 1]" and "[last regular]" belong to a reset.
				cur.text += "[" + setting + "]"
			}
			i = j
		case r == '&':
			flush()
			cur = &ruleToken{op: "&"}
		case r == '<' || r == '=':
			flush()
			op := string(r)
			for i+1 < len(runes) && runes[i+1] == r && r == '<' {
				op += "<"
				i++
			}
			if i+1 < len(runes) && runes[i+1] == '*' {
				op += "*"
				i++
			}
			cur = &ruleToken{op: op}
		case r == '\'':
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				j++
			}
			if cur != nil {
				cur.text += string(runes[i+1 : min(j, len(runes))])
			}
			i = j
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
		default:
			if cur != nil {
				cur.text += string(r)
			}
		}
	}
	flush()
	return out
}
