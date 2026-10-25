package constraints

// PrimitiveConstraint is the sealed interface implemented by every
// primitive constraint type. The closed set is enumerated in
// the package doc; new implementations may appear in this package
// only.
//
// Validate(value any) returns nil when the input satisfies the
// constraint. Otherwise it returns one [Violation] per failing
// clause (range, list, pattern, …). Validators do no I/O and use no
// reflection. Each accepts a small fixed set of Go types; see each
// Validate doc for the types it accepts.
type PrimitiveConstraint interface {
	Validate(value any) []Violation

	// ExampleValue returns a minimal-valid Go example value for this
	// constraint, in the shape Validate() expects.
	//
	// Contract: for bounded constraints that admit a value,
	// Validate(c.ExampleValue()) returns an empty Violation slice. A
	// constraint that admits none, such as a closed CString list whose
	// Pattern matches no entry, has no example Validate accepts.
	// Unbounded primitives return a documented sentinel (e.g. "example",
	// 0, "2020-01-01").
	ExampleValue() any

	// isPrimitive seals the interface to this package.
	isPrimitive()
}
