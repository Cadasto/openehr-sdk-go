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
// tripwire in dispatch_tripwire_test.go fails a type assertion or switch on
// these vocabularies whose enclosing function never derefs, and its
// case-coverage sweep fails a deref switch missing a shape's pair of cases —
// which is what lets these be plain switches instead of reflection (the idiom
// spec bans the latter). One pointer level suffices: a value-receiver method
// set promotes to `*T` but never to `**T`.

// DerefSelectExpr normalises a [SelectExpr] to the shape it denotes, reporting
// false when it denotes none — an untyped nil, or a nil pointer. Consumers
// type-switching over a [SelectExpr] (its godoc names that idiom) MUST route
// through this first when the tree may be hand-assembled; [ParseQuery] itself
// only ever populates value shapes.
func DerefSelectExpr(e SelectExpr) (SelectExpr, bool) {
	switch x := e.(type) {
	case PathExpr, FunctionCall, LiteralExpr, StarExpr:
		return x, true
	case *PathExpr:
		if x != nil {
			return *x, true
		}
	case *FunctionCall:
		if x != nil {
			return *x, true
		}
	case *LiteralExpr:
		if x != nil {
			return *x, true
		}
	case *StarExpr:
		if x != nil {
			return *x, true
		}
	}
	return nil, false // untyped nil, a typed-nil pointer, or an unlearned shape
}

// DerefLimitExpr is [DerefSelectExpr] for the LIMIT / OFFSET vocabulary.
func DerefLimitExpr(l LimitExpr) (LimitExpr, bool) {
	switch x := l.(type) {
	case IntLimit, ParamLimit:
		return x, true
	case *IntLimit:
		if x != nil {
			return *x, true
		}
	case *ParamLimit:
		if x != nil {
			return *x, true
		}
	}
	return nil, false // untyped nil, a typed-nil pointer, or an unlearned shape
}
