package intl

import (
	"io/fs"

	"github.com/go-quickjs/go-intl/internal/blob"
)

// ICU's default locale.
//
// ICU reads a locale's data in each of its trees -- the locales, the
// currencies, units, languages, regions, zones, collations and break rules
// -- along the locale's fallback. Where a tree has no bundle for the locale,
// nor for any locale it falls back to short of the root, ICU reads its
// default locale's bundles before the root's (ures_open's entryOpen, with
// U_USING_DEFAULT_WARNING). So a locale's answers can depend on the default,
// and the default is the host's: Node takes it from the environment, and
// Chrome from the language it runs in. ICU's own full data has a bundle for
// every language in most trees and so seldom comes to it; a browser's
// filtered data comes to it often. The default is also the locale a zone is
// named in where nothing names one, which is what Date writes in brackets.
//
// A Source answers as ICU does with the host's default locale, HostLocale,
// unless WithDefaultLocale gives it another. Nothing generated depends on
// the default: the services resolve it when they are built.

// WithDefaultLocale returns a source of src's data that answers as ICU does
// with locale as its default locale. An overlay laid over it, or over an
// overlay of it, keeps the default.
func WithDefaultLocale(src Source, locale Locale) Source {
	return &defaultSource{base: src, locale: locale}
}

// DefaultLocale is the default locale a source answers with: the one
// WithDefaultLocale gave it, else the host's (HostLocale).
func DefaultLocale(src Source) Locale {
	if d, ok := src.(defaultLocaler); ok {
		if locale, ok := d.defaultLocale(); ok {
			return locale
		}
	}
	return HostLocale()
}

// A defaultLocaler is a source that knows the default locale it answers
// with, or one laid over such a source.
type defaultLocaler interface {
	defaultLocale() (Locale, bool)
}

// A poolSource opens a shared pool itself, rather than as a file: an
// overlay, whose pool may extend its base's, or a source laid over one.
type poolSource interface {
	openPool(m Marker, version byte) (blob.Shared, error)
}

// defaultSource is WithDefaultLocale's: its base's data, and the default.
type defaultSource struct {
	base   Source
	locale Locale
}

func (s *defaultSource) Open(m Marker, d DataLocale) ([]byte, error) { return s.base.Open(m, d) }

func (s *defaultSource) defaultLocale() (Locale, bool) { return s.locale, true }

// readFile serves its base's files, so that it is a data directory an
// overlay can be laid over where its base is one.
func (s *defaultSource) readFile(name string) ([]byte, error) {
	if f, ok := s.base.(files); ok {
		return f.readFile(name)
	}
	return nil, fs.ErrNotExist
}

func (s *defaultSource) openPool(m Marker, version byte) (blob.Shared, error) {
	return openShared(s.base, m, version)
}
