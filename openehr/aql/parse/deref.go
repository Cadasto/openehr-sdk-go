package parse

// deref.go — pointer-twin normalisation for the parse-side sealed
// vocabularies, the [aql.DerefValue] / [aql.DerefWhere] of [SelectExpr] and
// [LimitExpr] (REQ-119).
//
// All four sealed interfaces in the AQL subsystem have value-receiver methods,
// so the pointer form of every shape satisfies the interface alongside the
// value form, and a typed-nil pointer panics when a method is called on it.
// Any code deciding behaviour from a concrete shape MUST normalise first, or
// the same rule binds one carrier and not the other. The dispatch-site
// tripwire in emit_parity_test.go fails a type assertion or switch on these
// vocabularies whose enclosing function never derefs.

import "reflect"

// DerefSelectExpr normalises a [SelectExpr] to the shape it denotes, reporting
// false when it denotes none — an untyped nil, or a nil pointer. Consumers
// type-switching over a [SelectExpr] (its godoc names that idiom) MUST route
// through this first when the tree may be hand-assembled; [ParseQuery] itself
// only ever populates value shapes.
func DerefSelectExpr(e SelectExpr) (SelectExpr, bool) { return derefAs(e) }

// DerefLimitExpr is [DerefSelectExpr] for the LIMIT / OFFSET vocabulary.
func DerefLimitExpr(l LimitExpr) (LimitExpr, bool) { return derefAs(l) }

// derefAs is the one normalisation body behind both wrappers — spelled
// identically to the aql package's twin (which the two vocabularies there use)
// so the four sealed sets get the same answer from one rule per package.
func derefAs[T any](v T) (T, bool) {
	var zero T
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return zero, false // untyped nil interface
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return zero, false
		}
		rv = rv.Elem()
	}
	inner, ok := rv.Interface().(T)
	return inner, ok
}
