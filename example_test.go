package intl_test

import (
	"fmt"
	"time"

	intl "github.com/go-quickjs/go-intl"
)

// The README's example. The no-break spaces are CLDR's, and %q shows them.
func Example() {
	loc, err := intl.ParseLocale("de-DE")
	if err != nil {
		panic(err)
	}

	nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "EUR",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", nf.Format(1234.5))
	r, _ := nf.FormatRange(5, 10)
	fmt.Printf("%q\n", r)

	df, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
		TimeZone: "Europe/Berlin", DateStyle: intl.LengthLong, TimeStyle: intl.LengthShort,
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%q\n", df.Format(time.Date(2024, 1, 5, 14, 4, 5, 0, time.UTC)))

	pr, err := intl.NewPluralRules(loc, intl.PluralRulesOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println(pr.Select(1), pr.Select(2))

	col, err := intl.NewCollator(loc, intl.CollatorOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println(col.Compare("Äpfel", "Birnen"))
	// Output:
	// "1.234,50\u00a0€"
	// "5,00–10,00\u00a0€"
	// "5. Januar 2024 um 15:04"
	// one other
	// -1
}
