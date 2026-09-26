package intl

import "strings"

// HostLocale is the locale the machine is set to, as ICU's default locale
// finds it (uprv_getDefaultLocaleID). On Windows it is the user's locale;
// elsewhere it is LC_ALL, LC_MESSAGES or LANG, its codeset dropped, "C" and
// "POSIX" being en_US_POSIX, "en-US-u-va-posix". What cannot be read is
// "en-US".
//
// V8 takes ICU's default for Intl's default locale but for that fallback,
// which it makes "en-US" (Isolate::DefaultLocale); an engine that answers
// as Node does does the same.
func HostLocale() Locale {
	if loc, ok := localeFromICUID(hostLocaleID()); ok {
		return loc
	}
	loc, _ := ParseLocale("en-US")
	return loc
}

// localeFromICUID reads an ICU locale ID, "de_DE" or "en_US_POSIX", as a
// locale, a POSIX variant becoming "-u-va-posix" as ICU writes it in
// BCP 47.
func localeFromICUID(id string) (Locale, bool) {
	if id == "" {
		return Locale{}, true
	}
	tag := strings.ReplaceAll(id, "_", "-")
	// "ab__CD" is a language and a variant.
	tag = strings.ReplaceAll(tag, "--", "-")
	c, err := NewCanonicalizer(Embedded, CanonicalizeOptions{})
	if err != nil {
		return Locale{}, false
	}
	loc, err := c.Canonicalize(tag)
	return loc, err == nil
}
