package bmmgen

import (
	"strings"
	"unicode"
)

// acronyms holds the set of 2- and 3-letter (and a few 4-letter)
// segments preserved as fully-uppercase in Go identifiers when they
// appear as a segment of a BMM ALL_CAPS or snake_case name.
//
// Rule (per Phase 2 spec § Naming):
//   - Split BMM name on '_'.
//   - For each segment: lowercase it, then capitalise the first rune.
//   - Exception: if the original (uppercase-snake) segment is in the
//     acronym set, keep it uppercase verbatim.
//   - Concatenate.
var acronyms = map[string]bool{
	"DV":   true,
	"ID":   true,
	"URI":  true,
	"URL":  true,
	"JSON": true,
	"XML":  true,
	"RM":   true,
	"AOM":  true,
	"UID":  true,
	"HL7":  true,
	"EHR":  true,
	"ISO":  true,
	"UCUM": true,
	"OID":  true,
}

// PascalCase converts a BMM class or other identifier name to its Go
// PascalCase equivalent. ALL_CAPS, snake_case, and mixed-case inputs
// are all supported. Common acronyms (see [acronyms]) are preserved
// uppercase per § Naming.
//
// Examples:
//
//	DV_QUANTITY       -> DVQuantity
//	EHR_STATUS        -> EHRStatus
//	OBJECT_VERSION_ID -> ObjectVersionID
//	HIER_OBJECT_ID    -> HierObjectID
//	Iso8601_date      -> ISO8601Date
//	CODE_PHRASE       -> CodePhrase
//	Multiplicity_interval -> MultiplicityInterval
//	X_VERSIONED_PARTY -> XVersionedParty
func PascalCase(name string) string {
	if name == "" {
		return ""
	}
	segs := strings.Split(name, "_")
	var out strings.Builder
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		// Treat the segment as an acronym candidate if (a) it is all
		// uppercase ASCII letters or (b) the lookup hits a case-folded
		// match with a known acronym.
		upper := strings.ToUpper(seg)
		switch {
		case acronyms[upper]:
			out.WriteString(upper)
		case isAllUpper(seg) && (len(seg) <= 4):
			// 2-4 letter ALL_CAPS segment that we don't know about —
			// fall through to Title-style so we don't unintentionally
			// blast every domain noun ("EVENT", "CHAIN") into uppercase.
			out.WriteString(titleSegment(seg))
		default:
			// Special-case alpha+digit-prefixed names like "Iso8601" —
			// fold them to "ISO8601" so callers that read the BMM name
			// can recognise the prefix.
			if t, ok := titleAlphaDigit(seg); ok {
				out.WriteString(t)
			} else {
				out.WriteString(titleSegment(seg))
			}
		}
	}
	return out.String()
}

// titleSegment lowercases the segment then capitalises its first
// rune. Avoids the deprecated [strings.Title].
func titleSegment(seg string) string {
	if seg == "" {
		return ""
	}
	r := []rune(strings.ToLower(seg))
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// titleAlphaDigit recognises segments of the form letters+digits (e.g.
// "Iso8601") and returns the alpha prefix in uppercase plus the digit
// suffix unchanged.
func titleAlphaDigit(seg string) (string, bool) {
	if seg == "" {
		return "", false
	}
	// Find the boundary between leading alphas and following digits.
	i := 0
	for i < len(seg) && isASCIILetter(rune(seg[i])) {
		i++
	}
	if i == 0 || i == len(seg) {
		return "", false
	}
	rest := seg[i:]
	allDigits := true
	for _, r := range rest {
		if r < '0' || r > '9' {
			allDigits = false
			break
		}
	}
	if !allDigits {
		return "", false
	}
	alpha := strings.ToUpper(seg[:i])
	return alpha + rest, true
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isAllUpper(seg string) bool {
	hasLetter := false
	for _, r := range seg {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

// FieldName converts a BMM property name (snake_case) to a Go
// PascalCase struct-field name. JSON tags preserve the original
// snake_case name verbatim — the caller emits it separately.
func FieldName(name string) string {
	return PascalCase(name)
}

// MethodName converts a BMM function name (snake_case) to a Go
// PascalCase method name. Acronyms in segments are preserved per
// the [PascalCase] rules.
//
// Examples:
//
//	add                        -> Add
//	is_strictly_comparable_to  -> IsStrictlyComparableTo
//	creating_system_id         -> CreatingSystemID
//	commit_original_merged_version -> CommitOriginalMergedVersion
func MethodName(name string) string {
	return PascalCase(name)
}

// ParamName converts a BMM parameter name (snake_case) to a Go
// camelCase identifier. The first segment stays lower-cased; later
// segments are PascalCase-style. If the resulting identifier collides
// with a Go reserved word or predeclared identifier, a trailing
// underscore is appended (per the keyword-collision rule documented
// in the bmm-conformance spec).
//
// Examples:
//
//	other                -> other
//	a_diff               -> aDiff
//	an_other_input_uids  -> anOtherInputUids
//	type                 -> type_
func ParamName(name string) string {
	if name == "" {
		return ""
	}
	segs := strings.Split(name, "_")
	var b strings.Builder
	first := true
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		if first {
			b.WriteString(strings.ToLower(seg))
			first = false
			continue
		}
		// Re-use PascalCase logic on a single segment by wrapping it
		// as a standalone token. Acronyms are preserved; otherwise
		// title-case.
		upper := strings.ToUpper(seg)
		switch {
		case acronyms[upper]:
			b.WriteString(upper)
		case isAllUpper(seg) && (len(seg) <= 4):
			b.WriteString(titleSegment(seg))
		default:
			if t, ok := titleAlphaDigit(seg); ok {
				b.WriteString(t)
			} else {
				b.WriteString(titleSegment(seg))
			}
		}
	}
	out := b.String()
	if goReservedIdentifiers[out] {
		out += "_"
	}
	return out
}

// goReservedIdentifiers is the set of Go reserved words and
// predeclared identifiers we defensively rename when they appear as a
// generated parameter name. The set is intentionally broad — Go
// keywords plus the predeclared identifiers that would shadow
// built-ins inside a method body. See [ParamName].
var goReservedIdentifiers = map[string]bool{
	// Keywords (per the Go spec § Keywords).
	"break": true, "default": true, "func": true, "interface": true, "select": true,
	"case": true, "defer": true, "go": true, "map": true, "struct": true,
	"chan": true, "else": true, "goto": true, "package": true, "switch": true,
	"const": true, "fallthrough": true, "if": true, "range": true, "type": true,
	"continue": true, "for": true, "import": true, "return": true, "var": true,
	// Predeclared identifiers worth defending against.
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true,
	"float32": true, "float64": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"true": true, "false": true, "iota": true, "nil": true,
	"append": true, "cap": true, "clear": true, "close": true, "copy": true,
	"delete": true, "len": true, "make": true, "max": true, "min": true,
	"new": true, "panic": true, "print": true, "println": true, "recover": true,
}

// FileBase returns the file-name stem for a BMM top-level (after
// prefix stripping) package path. Leading namespace prefixes are
// stripped to keep the emitted filenames concise:
//   - "org.openehr.rm."           — RM target
//   - "org.openehr.base."         — base classes shared with RM/AOM
//   - "org.openehr.am.aom14."     — AOM 1.4 target
//   - "org.openehr.am.aom_140."   — alternate AOM 1.4 spelling
//   - "org.openehr.am.aom2."      — future AOM 2 target
//   - "org.openehr."              — catch-all fallback
//
// The remaining dotted path becomes underscore-joined.
//
// Examples:
//
//	org.openehr.rm.data_types.quantity                 -> data_types_quantity
//	org.openehr.base.base_types.identification         -> base_types_identification
//	org.openehr.base.foundation_types.primitive_types  -> foundation_types_primitive_types
//	org.openehr.am.aom14.archetype                     -> archetype
//	org.openehr.am.aom14.archetype.constraint_model    -> archetype_constraint_model
//	org.openehr.am.aom14.openehr_archetype_profile     -> openehr_archetype_profile
func FileBase(pkgName string) string {
	prefixes := []string{
		"org.openehr.rm.",
		"org.openehr.base.",
		"org.openehr.am.aom14.",
		"org.openehr.am.aom_140.",
		"org.openehr.am.aom2.",
		"org.openehr.am.",
		"org.openehr.",
	}
	short := pkgName
	for _, p := range prefixes {
		if strings.HasPrefix(short, p) {
			short = short[len(p):]
			break
		}
	}
	return strings.ReplaceAll(short, ".", "_")
}
