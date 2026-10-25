package intl

// nodeData is the package's data with the default locale Node's recordings
// were made with, en-US, whatever the host's is, for the replays of Node's
// recordings the package's own tests make.
var nodeData = func() Source {
	x, err := ParseLocale("en-US")
	if err != nil {
		panic(err)
	}
	return WithDefaultLocale(Embedded, x)
}()
