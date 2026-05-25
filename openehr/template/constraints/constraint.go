package constraints

// PrimitiveConstraint is the sealed interface implemented by every
// REQ-103 primitive constraint type. The closed set is enumerated in
// the package doc; new implementations may appear in this package
// only.
//
// Validate(value any) returns nil when the input satisfies the
// constraint. Otherwise it returns one [Violation] per failing
// clause (range, list, pattern, …). Validators are pure — no I/O, no
// reflection over user types beyond a small fixed coercion table per
// type (see each Validate doc for the accepted Go shapes).
type PrimitiveConstraint interface {
	Validate(value any) []Violation

	// ExampleValue returns a minimal-valid Go example value for this
	// constraint, in the shape Validate() expects. REQ-107.
	//
	// Contract: for bounded constraints, Validate(c.ExampleValue())
	// MUST return an empty Violation slice. Unbounded primitives
	// return a documented sentinel (e.g. "example", 0, "2020-01-01").
	ExampleValue() any

	// isPrimitive seals the interface to this package.
	isPrimitive()
}
