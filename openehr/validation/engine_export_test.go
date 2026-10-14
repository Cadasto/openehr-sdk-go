package validation

// engine_export_test.go: hands the one walk, which no exported entry runs yet,
// to the package's external tests, so they compare it with the exported
// entries over the roots they already load.

import "github.com/cadasto/openehr-sdk-go/openehr/templatecompile"

// RunEngine runs the one walk over root, with the compiled template c, within
// lim.
var RunEngine = runEngine

// RunEngineRefusing runs the one walk as RunEngine does, and returns too the
// path of each attribute or item the template did not admit, which the walk
// reported once and did not enter, in walk order.
func RunEngineRefusing(root any, c *templatecompile.Compiled, lim Limits) (Result, []string) {
	r, e := engineWalk(root, c, lim)
	if e == nil {
		return r, nil
	}
	return r, e.refused
}
