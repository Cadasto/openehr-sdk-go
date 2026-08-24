package contain

// Finding is a containment finding surfaced by the builder verification
// (REQ-162, Phase 3). Like the REQ-109 lint issue model it carries a value-free
// Code and a value-bearing Detail; it deliberately has no Span, Path, or
// severity field — a builder tree has no source text to point into, and
// severity is looked up per code in the REQ-161 catalogue.
type Finding struct {
	// Code is a value-free REQ-161 issue code (e.g. "aql_impossible_containment").
	Code string
	// Detail is value-bearing free text that MAY quote class names and HRIDs.
	Detail string
}
