package contain

// Finding is a containment finding reported by the builder verification
// ([github.com/cadasto/openehr-sdk-go/openehr/aql.Builder.VerifyContainment]).
// Like the lint issue model it carries a value-free Code and a value-bearing
// Detail. It has no Span, Path, or severity field: a builder tree has no
// source text to point into, and severity is fixed per code in the lint
// catalogue.
type Finding struct {
	// Code is a value-free lint issue code (e.g. "aql_impossible_containment").
	Code string
	// Detail is value-bearing free text that may quote class names and HRIDs.
	Detail string
}
