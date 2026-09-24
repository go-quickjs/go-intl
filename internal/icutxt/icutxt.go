// Package icutxt reads ICU's resource bundle source format, the .txt files in
// ICU's data sources: data/locales/root.txt and its siblings.
//
// Generators read CLDR's JSON for almost everything. A few facts CLDR's JSON
// leaves out -- the root's entries for the numbering systems, which CLDR's
// root.xml has and cldr-json drops -- are in these files, which ICU converts
// from the same CLDR release.
//
// The format is a tree of tables whose leaves are strings:
//
//	root{
//	    NumberElements{
//	        arab{
//	            symbols{
//	                decimal{"٫"}
//	            }
//	            patterns{
//	                decimalFormat:alias{"/LOCALE/NumberElements/latn/patterns/decimalFormat"}
//	            }
//	        }
//	    }
//	}
//
// This reads tables, strings, aliases, integers and arrays of strings, which
// is everything the files used so far contain. Binary and other resource types
// are refused rather than skipped.
package icutxt

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A Node is one resource: a table, a string, an alias or an array.
type Node struct {
	Key string
	// Children are a table's entries, in the order written.
	Children []*Node
	// Table is true for a table, even an empty one.
	Table bool
	// Value is a string's text, an alias's target or an integer's digits.
	Value string
	// Alias is true when Value is another resource's path.
	Alias bool
	// Values are an array's strings.
	Values []string
}

// Get follows a path of keys down from a table. It returns nil where a key is
// missing.
func (n *Node) Get(path ...string) *Node {
	cur := n
	for _, key := range path {
		if cur == nil || !cur.Table {
			return nil
		}
		var next *Node
		for _, c := range cur.Children {
			if c.Key == key {
				next = c
				break
			}
		}
		cur = next
	}
	return cur
}

// Parse reads one bundle: a single top-level table, named for its locale.
func Parse(src string) (*Node, error) {
	p := &parser{s: src}
	p.skip()
	n, err := p.item()
	if err != nil {
		return nil, fmt.Errorf("icutxt: %w at offset %d", err, p.i)
	}
	p.skip()
	if p.i != len(p.s) {
		return nil, fmt.Errorf("icutxt: text after the bundle at offset %d", p.i)
	}
	return n, nil
}

type parser struct {
	s string
	i int
}

// skip passes over white space, comments and a byte-order mark.
func (p *parser) skip() {
	for p.i < len(p.s) {
		switch {
		case strings.HasPrefix(p.s[p.i:], "\ufeff"):
			p.i += len("\ufeff")
		case strings.HasPrefix(p.s[p.i:], "//"):
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		case strings.HasPrefix(p.s[p.i:], "/*"):
			end := strings.Index(p.s[p.i+2:], "*/")
			if end < 0 {
				p.i = len(p.s)
				return
			}
			p.i += 2 + end + 2
		case p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\r' || p.s[p.i] == '\n':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

// key reads a resource's name, which may be quoted.
func (p *parser) key() (string, error) {
	if p.peek() == '"' {
		return p.quoted()
	}
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '{' || c == ':' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			break
		}
		p.i++
	}
	if p.i == start {
		return "", fmt.Errorf("a resource with no name")
	}
	return p.s[start:p.i], nil
}

// item reads name[:type]{...}.
func (p *parser) item() (*Node, error) {
	key, err := p.key()
	if err != nil {
		return nil, err
	}
	p.skip()
	kind := ""
	if p.peek() == ':' {
		p.i++
		start := p.i
		for p.i < len(p.s) && p.s[p.i] != '{' && p.s[p.i] != ' ' {
			p.i++
		}
		kind = p.s[start:p.i]
		p.skip()
	}
	if p.peek() != '{' {
		return nil, fmt.Errorf("%s has no body", key)
	}
	p.i++
	p.skip()
	n := &Node{Key: key}
	switch kind {
	case "alias":
		n.Alias = true
		if n.Value, err = p.strings(); err != nil {
			return nil, err
		}
	case "int", "intvector":
		// Integers, one or a comma-separated list, kept as their digits.
		start := p.i
		for p.i < len(p.s) && p.s[p.i] != '}' {
			p.i++
		}
		for _, field := range strings.Split(p.s[start:p.i], ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			if _, err := strconv.Atoi(field); err != nil {
				return nil, fmt.Errorf("%s: %q is not an integer", key, field)
			}
			n.Values = append(n.Values, field)
		}
		if len(n.Values) == 1 && kind == "int" {
			n.Value = n.Values[0]
		}
	case "", "table", "array", "string", "process(uca_rules)", "process(collation)", "process(transliterator)", "process(dependency)":
		// A process type is a string ICU's build does something with; the
		// text is still a string.
		if err := p.body(n); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s is of type %s, which this does not read", key, kind)
	}
	p.skip()
	if p.peek() != '}' {
		return nil, fmt.Errorf("%s is not closed", key)
	}
	p.i++
	return n, nil
}

// body reads what is between the braces of an untyped resource, deciding by
// what it starts with whether it is a string, an array or a table. A quoted
// string followed by a brace is a table's first key, not a value.
func (p *parser) body(n *Node) error {
	switch p.peek() {
	case '}':
		n.Table = true
		return nil
	case '"':
		mark := p.i
		s, err := p.strings()
		if err != nil {
			return err
		}
		p.skip()
		switch p.peek() {
		case '{', ':':
			p.i = mark
			return p.table(n)
		case ',':
			p.i = mark
			return p.array(n)
		}
		n.Value = s
		return nil
	case '{':
		return p.array(n)
	}
	return p.table(n)
}

func (p *parser) table(n *Node) error {
	n.Table = true
	for p.peek() != '}' && p.i < len(p.s) {
		child, err := p.item()
		if err != nil {
			return err
		}
		n.Children = append(n.Children, child)
		p.skip()
	}
	return nil
}

// array reads comma-separated elements, each strings or a nested body. The
// strings are kept in Values and every element, nested or not, in Children.
func (p *parser) array(n *Node) error {
	for p.peek() != '}' && p.i < len(p.s) {
		el := &Node{Key: strconv.Itoa(len(n.Children))}
		switch p.peek() {
		case '"':
			s, err := p.strings()
			if err != nil {
				return err
			}
			el.Value = s
			n.Values = append(n.Values, s)
		case '{':
			p.i++
			p.skip()
			if err := p.body(el); err != nil {
				return err
			}
			p.skip()
			if p.peek() != '}' {
				return fmt.Errorf("an element of %s is not closed", n.Key)
			}
			p.i++
		default:
			return fmt.Errorf("an element of %s is neither a string nor a body", n.Key)
		}
		n.Children = append(n.Children, el)
		p.skip()
		if p.peek() == ',' {
			p.i++
			p.skip()
		}
	}
	return nil
}

// strings reads one or more adjacent quoted strings, which ICU joins.
func (p *parser) strings() (string, error) {
	var b strings.Builder
	for p.peek() == '"' {
		s, err := p.quoted()
		if err != nil {
			return "", err
		}
		b.WriteString(s)
		p.skip()
	}
	return b.String(), nil
}

// quoted reads one quoted string with its escapes.
func (p *parser) quoted() (string, error) {
	p.i++ // the opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch c {
		case '"':
			p.i++
			return b.String(), nil
		case '\\':
			if p.i+1 >= len(p.s) {
				return "", fmt.Errorf("a string ends in a backslash")
			}
			switch e := p.s[p.i+1]; e {
			case 'u', 'U':
				n := 4
				if e == 'U' {
					n = 8
				}
				if p.i+2+n > len(p.s) {
					return "", fmt.Errorf("a short escape")
				}
				v, err := strconv.ParseUint(p.s[p.i+2:p.i+2+n], 16, 32)
				if err != nil {
					return "", err
				}
				b.WriteRune(rune(v))
				p.i += 2 + n
			case 'n':
				b.WriteByte('\n')
				p.i += 2
			case 't':
				b.WriteByte('\t')
				p.i += 2
			default:
				b.WriteByte(e)
				p.i += 2
			}
		default:
			r, size := utf8.DecodeRuneInString(p.s[p.i:])
			b.WriteRune(r)
			p.i += size
		}
	}
	return "", fmt.Errorf("a string never ends")
}
