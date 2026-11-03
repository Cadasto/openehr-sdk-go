package constraints

// CodeReference constrains an RM CODE_PHRASE value whose codes come from a
// reference set (C_CODE_REFERENCE). It is the code phrase constraint it
// refines, whose fields and methods it carries, and ReferenceSetURI, the URI
// that names the reference set, such as "terminology:SNOMED-CT".
//
// The reference set is held in an external terminology, which a validator
// does not consult, so Validate judges a value by the embedded [CodePhrase]
// alone, and ExampleValue is the embedded one's. [CodePhraseOf] reads a
// CodeReference and a CodePhrase alike, for a consumer that has no use for
// the reference set.
type CodeReference struct {
	CodePhrase
	ReferenceSetURI string
}

// CodePhraseOf returns the code phrase constraint pc carries: pc itself when
// it is a [CodePhrase], and the embedded one when it is a [CodeReference].
// The second result is false for any other constraint, and for nil.
func CodePhraseOf(pc PrimitiveConstraint) (CodePhrase, bool) {
	switch c := pc.(type) {
	case CodePhrase:
		return c, true
	case CodeReference:
		return c.CodePhrase, true
	}
	return CodePhrase{}, false
}
