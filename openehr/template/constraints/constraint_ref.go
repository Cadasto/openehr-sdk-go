package constraints

import "fmt"

// ConstraintRef constrains a value by reference to a value set that lives
// outside the archetype (CONSTRAINT_REF). Reference is the constraint code
// that names the value set, such as "ac0001"; the node that carries the
// constraint names the RM type it constrains, typically CODE_PHRASE.
//
// The value set is held in an external terminology, which a validator does
// not consult, so [ConstraintRef.Validate] admits any code.
type ConstraintRef struct {
	Reference string
}

func (ConstraintRef) isPrimitive() {}

// ExampleValue returns the placeholder [CodedTermRef] "local::at0000".
// A constraint reference names no terminology and lists no code, so the
// example falls back to the local terminology, as an unconstrained
// [CodePhrase] does.
func (ConstraintRef) ExampleValue() any {
	return CodedTermRef{Terminology: "local", CodeString: "at0000"}
}

// Validate accepts a [CodedTermRef] or a bare string, the inputs
// [CodePhrase.Validate] accepts, and returns no violation for either: the
// value set is external terminology, so any code may belong to it. Any
// other Go type returns one CodeWrongType violation.
func (ConstraintRef) Validate(value any) []Violation {
	switch value.(type) {
	case CodedTermRef, string:
		return nil
	}
	return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected CodedTermRef or string, got %T", value)}}
}
