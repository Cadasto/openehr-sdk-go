package constraints

import "strings"

// CodeReference constrains an RM CODE_PHRASE value whose codes come from a
// reference set (C_CODE_REFERENCE). It is the code phrase constraint it
// refines, whose fields and methods it carries, and ReferenceSetURI, the URI
// that names the reference set, such as "terminology:SNOMED-CT".
//
// The reference set is held in an external terminology, which a validator
// does not consult, so Validate judges a value by the embedded [CodePhrase]
// alone, and ExampleValue is the embedded one's. [CodePhraseOf] reads a
// CodeReference and a CodePhrase alike, for a consumer that has no use for
// the reference set, and [CodeReference.ReferenceSetTerminology] reads the
// terminology the URI names.
type CodeReference struct {
	CodePhrase
	ReferenceSetURI string
}

// ReferenceSetTerminology returns the terminology id ReferenceSetURI names,
// for a reader that needs the terminology of a reference set whose code
// phrase names none. A URI terminology:<name>[/<version>], with any query or
// fragment after it, names <name>, or <name>(<version>) where it gives a
// version: the form of an openEHR TERMINOLOGY_ID. The scheme is matched in
// any case, and the name and version are kept as written.
//
// The second result is false for a URI of another scheme, one that names no
// terminology, and one that does not take that form: a name or version that
// is empty, or holds a slash, a space, a control character or a parenthesis,
// which the id could not carry. The openEHR specifications do not say which
// terminology such a URI names; this is the SDK's reading.
func (c CodeReference) ReferenceSetTerminology() (id string, ok bool) {
	scheme, rest, found := strings.Cut(c.ReferenceSetURI, ":")
	if !found || !strings.EqualFold(scheme, "terminology") {
		return "", false
	}
	rest, _, _ = strings.Cut(rest, "#")
	rest, _, _ = strings.Cut(rest, "?")
	name, version, versioned := strings.Cut(rest, "/")
	if !terminologyIDPart(name) {
		return "", false
	}
	if !versioned {
		return name, true
	}
	if !terminologyIDPart(version) {
		return "", false
	}
	return name + "(" + version + ")", true
}

// terminologyIDPart reports whether s can stand as the name or the version
// of a terminology id read from a terminology: URI: it is not empty and holds
// no slash, space, ASCII control character or parenthesis.
func terminologyIDPart(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return r <= ' ' || r == 0x7f || r == '/' || r == '(' || r == ')'
	})
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
