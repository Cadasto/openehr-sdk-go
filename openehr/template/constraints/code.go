package constraints

import (
	"fmt"
	"slices"
)

// CodedTermRef is the openEHR CODE_PHRASE shape used inside primitive
// constraint payloads (e.g. DV_QUANTITY.property, DV_ORDINAL.symbol).
// It is a flat record so this package need not import the rm package,
// whose rm.DVCodedText pulls in too much surface for a leaf constraint
// type.
type CodedTermRef struct {
	Terminology string
	CodeString  string
}

// String renders the ref as `terminology::code` (the openEHR
// shorthand). Empty fields produce `::code` or `terminology::`.
func (c CodedTermRef) String() string {
	return c.Terminology + "::" + c.CodeString
}

// CodePhrase constrains an RM CODE_PHRASE value (C_CODE_PHRASE).
// Terminology is the constrained terminology identifier (e.g.
// "openehr", "SNOMED-CT", "LOINC"); CodeList is the optional closed
// list of allowed codes within that terminology. An empty CodeList
// means "any code under the terminology is acceptable" (External
// constraint).
type CodePhrase struct {
	Terminology string
	CodeList    []string
}

func (CodePhrase) isPrimitive() {}

// ExampleValue returns a minimal-valid [CodedTermRef].
// The first entry of CodeList wins when the list is non-empty (so closed
// enumerations produce a member); otherwise it returns a "local::at0000"
// sentinel under the constrained terminology (or "local" when
// unconstrained) so external constraints surface a recognisable placeholder.
func (c CodePhrase) ExampleValue() any {
	if len(c.CodeList) > 0 {
		term := c.Terminology
		if term == "" {
			term = "local"
		}
		return CodedTermRef{Terminology: term, CodeString: c.CodeList[0]}
	}
	term := c.Terminology
	if term == "" {
		term = "local"
	}
	return CodedTermRef{Terminology: term, CodeString: "at0000"}
}

// External reports whether the constraint enumerates a closed list
// of codes (false) or only restricts the terminology (true).
func (c CodePhrase) External() bool {
	return len(c.CodeList) == 0
}

// Validate accepts either a [CodedTermRef] value or a bare string
// (treated as the code string under any terminology), and checks it
// as [CodePhrase.ValidateCode] does.
func (c CodePhrase) Validate(value any) []Violation {
	switch v := value.(type) {
	case CodedTermRef:
		return c.ValidateCode(v)
	case string:
		return c.ValidateCode(CodedTermRef{Terminology: c.Terminology, CodeString: v})
	default:
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected CodedTermRef or string, got %T", value)}}
	}
}

// ValidateCode checks ref against the constraint and returns what
// [CodePhrase.Validate] returns for the same value. When Terminology is
// set and ref carries a different terminology id, the result includes a
// CodeInvalidValue violation. When CodeList is non-empty the code must
// appear in it, or the result includes a CodeNotInList violation.
//
// It takes the code typed, so a caller that holds a [CodedTermRef] checks
// it without converting it to any, which allocates. A [CodeReference] has
// ValidateCode through the code phrase it carries, and judges a code by
// that code phrase alone.
func (c CodePhrase) ValidateCode(ref CodedTermRef) []Violation {
	var out []Violation
	if c.Terminology != "" && ref.Terminology != "" && ref.Terminology != c.Terminology {
		out = append(out, Violation{
			Code:   CodeInvalidValue,
			Detail: fmt.Sprintf("terminology does not match constraint terminology %q", c.Terminology),
			Value:  Redact(ref.Terminology),
		})
	}
	if len(c.CodeList) > 0 && !slices.Contains(c.CodeList, ref.CodeString) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("code not in allowed list %v", c.CodeList),
			Value:  Redact(ref.CodeString),
		})
	}
	return out
}
