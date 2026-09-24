package icutxt

import "testing"

func TestParse(t *testing.T) {
	src := "\ufeff// comment\nroot{\n" +
		"    /* a block\n comment */\n" +
		"    NumberElements{\n" +
		"        arab{\n" +
		"            symbols{\n" +
		"                decimal{\"\\u066B\"}\n" +
		"                group{\"٬\"}\n" +
		"            }\n" +
		"            patterns{\n" +
		"                decimalFormat:alias{\"/LOCALE/NumberElements/latn/patterns/decimalFormat\"}\n" +
		"                currencyFormat%noCurrency{\"#,##0.00\"}\n" +
		"            }\n" +
		"        }\n" +
		"        minimumGroupingDigits{\"1\"}\n" +
		"    }\n" +
		"    Version{\"48\" \".1\"}\n" +
		"    eras{ \"BC\", \"AD\", }\n" +
		"    count:int{ 3 }\n" +
		"    empty{}\n" +
		"}\n"
	root, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.Get("NumberElements", "arab", "symbols", "decimal").Value; got != "\u066b" {
		t.Errorf("decimal = %q", got)
	}
	if got := root.Get("NumberElements", "arab", "symbols", "group").Value; got != "\u066c" {
		t.Errorf("group = %q", got)
	}
	alias := root.Get("NumberElements", "arab", "patterns", "decimalFormat")
	if !alias.Alias || alias.Value != "/LOCALE/NumberElements/latn/patterns/decimalFormat" {
		t.Errorf("alias = %+v", alias)
	}
	if got := root.Get("NumberElements", "arab", "patterns", "currencyFormat%noCurrency").Value; got != "#,##0.00" {
		t.Errorf("a key with a percent sign = %q", got)
	}
	if got := root.Get("Version").Value; got != "48.1" {
		t.Errorf("adjacent strings = %q, want them joined", got)
	}
	if got := root.Get("eras").Values; len(got) != 2 || got[1] != "AD" {
		t.Errorf("array = %q", got)
	}
	if got := root.Get("count").Value; got != "3" {
		t.Errorf("int = %q", got)
	}
	if e := root.Get("empty"); e == nil || !e.Table {
		t.Errorf("empty table = %+v", e)
	}
	if root.Get("NumberElements", "nope") != nil {
		t.Error("a missing key was found")
	}
}

// Shapes the real files have: a table whose first key is quoted, an array
// holding a nested array, an integer vector and a processed string.
func TestParseRealShapes(t *testing.T) {
	src := `ja{
    relative{
        "-1"{"yesterday"}
        "0"{"today"}
    }
    DateTimePatterns{
        "H:mm:ss",
        {
            "y/M/d",
            "hanidec",
        }
    }
    calendar-field:intvector{ 1, 2, 3 }
    UCARules:process(uca_rules){"../unidata/UCARules.txt"}
}`
	root, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.Get("relative", "-1").Value; got != "yesterday" {
		t.Errorf("quoted key = %q", got)
	}
	patterns := root.Get("DateTimePatterns")
	if len(patterns.Children) != 2 || patterns.Children[0].Value != "H:mm:ss" ||
		len(patterns.Children[1].Values) != 2 || patterns.Children[1].Values[1] != "hanidec" {
		t.Errorf("nested array = %+v", patterns)
	}
	if got := root.Get("calendar-field").Values; len(got) != 3 || got[2] != "3" {
		t.Errorf("intvector = %q", got)
	}
	if got := root.Get("UCARules").Value; got != "../unidata/UCARules.txt" {
		t.Errorf("process string = %q", got)
	}
}

func TestParseRefusesWhatItCannotRead(t *testing.T) {
	for _, src := range []string{
		`root{ x:bin{ 00ff } }`,
		`root{ x{"unterminated }`,
		`root{ x{"a"} `,
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded", src)
		}
	}
}
