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
// terminology the URI names. [CodePhraseWithTerminologyOf] reads the two
// alike too, with that terminology where the code phrase names none, for a
// consumer that needs the terminology a value is coded in.
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

// CodePhraseWithTerminologyOf returns the code phrase constraint pc
// carries, as [CodePhraseOf] does, except that a [CodeReference] whose code
// phrase names no terminology takes the one its URI names
// ([CodeReference.ReferenceSetTerminology]). A code phrase that names a
// terminology keeps it, and one whose URI names none is returned as it is.
//
// It is for a reader that needs the terminology a value is coded in, such
// as one that writes an example value or describes a form input. A
// validator, which does not consult the reference set, reads [CodePhraseOf].
// The second result is false for any other constraint, and for nil.
func CodePhraseWithTerminologyOf(pc PrimitiveConstraint) (CodePhrase, bool) {
	phrase, ok := CodePhraseOf(pc)
	if !ok || phrase.Terminology != "" {
		return phrase, ok
	}
	if ref, isRef := pc.(CodeReference); isRef {
		if id, named := ref.ReferenceSetTerminology(); named {
			phrase.Terminology = id
		}
	}
	return phrase, true
}
