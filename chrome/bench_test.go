package chrome

import (
	"testing"

	intl "github.com/go-quickjs/go-intl"
)

// What a profile costs: finding it, and building each service over its
// data rather than over the intl package's. Chrome 157's overlay is laid
// straight over the package's data and Chrome 142's under eight others
// (internal/chromegen), so the two are the shallowest and the deepest.

// profileSources are the sources measured: the package's own, then the
// shallowest profile and the deepest.
func profileSources(tb testing.TB) []struct {
	name string
	src  intl.Source
} {
	tb.Helper()
	out := []struct {
		name string
		src  intl.Source
	}{{"embedded", intl.Embedded}}
	for _, v := range []string{"157", "142"} {
		p, err := ForVersion(v)
		if err != nil {
			tb.Fatal(err)
		}
		out = append(out, struct {
			name string
			src  intl.Source
		}{"chrome" + v, p.Source})
	}
	return out
}

func BenchmarkForVersion(b *testing.B) {
	for _, v := range []string{"157", "150", "142"} {
		b.Run(v, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := ForVersion(v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkProfileServices(b *testing.B) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		b.Fatal(err)
	}
	sources := profileSources(b)
	for _, svc := range profileServices {
		for _, s := range sources {
			b.Run(svc.name+"/"+s.name, func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := svc.build(s.src, loc); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
