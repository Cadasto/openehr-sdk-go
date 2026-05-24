package constraints

import (
	"fmt"
	"slices"
)

// CodedTermRef is the openEHR CODE_PHRASE shape used inside primitive
// constraint payloads (e.g. DV_QUANTITY.property, DV_ORDINAL.symbol).
// Kept as a flat record so this package stays stdlib-only — the rm
// package's DvCodedText pulls in too much surface for a leaf
// constraint type.
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

// External reports whether the constraint enumerates a closed list
// of codes (false) or only restricts the terminology (true).
func (c CodePhrase) External() bool {
	return len(c.CodeList) == 0
}

// Validate accepts either a [CodedTermRef] value or a bare string
// (treated as the code string under any terminology). When
// Terminology is set and the input carries a different terminology
// id, the result includes a CodeInvalidValue violation. When
// CodeList is non-empty the code MUST appear in it.
func (c CodePhrase) Validate(value any) []Violation {
	var ref CodedTermRef
	switch v := value.(type) {
	case CodedTermRef:
		ref = v
	case string:
		ref = CodedTermRef{Terminology: c.Terminology, CodeString: v}
	default:
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected CodedTermRef or string, got %T", value)}}
	}
	var out []Violation
	if c.Terminology != "" && ref.Terminology != "" && ref.Terminology != c.Terminology {
		out = append(out, Violation{
			Code:   CodeInvalidValue,
			Detail: fmt.Sprintf("terminology %q does not match constraint %q", ref.Terminology, c.Terminology),
		})
	}
	if len(c.CodeList) > 0 && !slices.Contains(c.CodeList, ref.CodeString) {
		out = append(out, Violation{
			Code:   CodeNotInList,
			Detail: fmt.Sprintf("code %q not in allowed list %v", ref.CodeString, c.CodeList),
		})
	}
	return out
}
