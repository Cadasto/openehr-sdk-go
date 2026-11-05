package template

import (
	"fmt"
	"math"
	"strconv"
)

// Multiplicity is the min/max interval that OPT uses for both
// existence and occurrences blocks. Fields are unexported to keep
// intervals immutable from outside the package; [NewMultiplicity]
// constructs values.
//
// A Multiplicity counts whole items, so every bounded side of a
// Multiplicity is closed: [NewMultiplicity] turns an end the OPT excludes
// into the closed end it stands for. An OPT interval ">0..<3" gives Lower 1
// and Upper 2, and the bound of any side that is not unbounded is always
// inclusive.
type Multiplicity struct {
	lower          int
	upper          int
	lowerUnbounded bool
	upperUnbounded bool
}

// IntervalBounds is an AOM 1.4 integer interval as an OPT writes it, its two
// bounds and its four flags, which [NewMultiplicity] closes into a
// [Multiplicity]. A flag is false unless set, so each side is bounded and
// excludes its end unless the fields say otherwise.
type IntervalBounds struct {
	// Lower and Upper are the bounds of the interval.
	Lower, Upper int
	// LowerIncluded and UpperIncluded report whether the interval includes
	// the bound of that side.
	LowerIncluded, UpperIncluded bool
	// LowerUnbounded and UpperUnbounded report whether that side of the
	// interval has no bound.
	LowerUnbounded, UpperUnbounded bool
}

// NewMultiplicity returns the Multiplicity of the AOM 1.4 integer interval b,
// closed on each bounded side: an end the interval excludes becomes the
// closed end it stands for, so the bounds 0 and 3 with both ends excluded
// give Lower 1 and Upper 2. An unbounded side keeps the bound it is given,
// which its included flag does not move. NewMultiplicity returns an error
// for an interval whose closed ends are inverted, one that closing leaves
// empty, such as ">2..<3", and an excluded end whose closed end an int
// cannot hold.
func NewMultiplicity(b IntervalBounds) (*Multiplicity, error) {
	m := &Multiplicity{
		lower:          b.Lower,
		upper:          b.Upper,
		lowerUnbounded: b.LowerUnbounded,
		upperUnbounded: b.UpperUnbounded,
	}
	// A Multiplicity counts whole items, so an excluded end on a bounded
	// side becomes the closed end it stands for: ">0" is 1 and "<3" is 2.
	// An unbounded side has no bound to move, whatever its included flag
	// says. An int cannot hold the closed end of "> the largest int" or
	// "< the smallest int", so such an end is refused instead of being
	// left to wrap around to the other end of the int range.
	if !b.LowerUnbounded && !b.LowerIncluded {
		if m.lower == math.MaxInt {
			return nil, fmt.Errorf("closed lower bound overflows int: excluded lower=%d", b.Lower)
		}
		m.lower++
	}
	if !b.UpperUnbounded && !b.UpperIncluded {
		if m.upper == math.MinInt {
			return nil, fmt.Errorf("closed upper bound overflows int: excluded upper=%d", b.Upper)
		}
		m.upper--
	}
	// Reject inverted intervals so downstream walkers can assume
	// `Lower <= Upper` whenever both bounds are present. The check runs
	// on the closed ends, so an interval that closing leaves empty, such
	// as ">2..<3", is refused here too; the message adds the interval as
	// written so the reader recognises it. Unbounded sides are skipped:
	// they have no concrete value to compare.
	if !m.lowerUnbounded && !m.upperUnbounded && m.lower > m.upper {
		return nil, fmt.Errorf("inverted interval lower=%d > upper=%d (written %s)",
			m.lower, m.upper, writtenBounds(b.Lower, b.Upper, b.LowerIncluded, b.UpperIncluded))
	}
	return m, nil
}

// writtenBounds spells a bounded interval in ADL notation, with the
// bounds the OPT wrote and each end marked by its included flag:
// ">2..<3" when both ends are excluded, "2..3" when both are closed.
func writtenBounds(lower, upper int, lowerIncluded, upperIncluded bool) string {
	lowerMark, upperMark := "", ""
	if !lowerIncluded {
		lowerMark = ">"
	}
	if !upperIncluded {
		upperMark = "<"
	}
	return fmt.Sprintf("%s%d..%s%d", lowerMark, lower, upperMark, upper)
}

// Lower returns the lower bound of the interval.
func (m Multiplicity) Lower() int { return m.lower }

// Upper returns the upper bound of the interval.
func (m Multiplicity) Upper() int { return m.upper }

// LowerUnbounded reports whether the lower bound is "*" / unbounded
// in the OPT XML.
func (m Multiplicity) LowerUnbounded() bool { return m.lowerUnbounded }

// UpperUnbounded reports whether the upper bound is "*" / unbounded
// in the OPT XML.
func (m Multiplicity) UpperUnbounded() bool { return m.upperUnbounded }

// Cardinality tags an attribute of a compiled template as single-valued or
// multi-valued.
type Cardinality int

const (
	// Single corresponds to xsi:type="C_SINGLE_ATTRIBUTE".
	Single Cardinality = iota
	// Multiple corresponds to xsi:type="C_MULTIPLE_ATTRIBUTE".
	Multiple
)

// String returns "single" or "multiple"; out-of-range values render
// as "cardinality(N)" for diagnostic readability.
func (c Cardinality) String() string {
	switch c {
	case Single:
		return "single"
	case Multiple:
		return "multiple"
	default:
		return "cardinality(" + strconv.Itoa(int(c)) + ")"
	}
}

// IsValid reports whether c is one of the recognised Cardinality
// constants. Useful for guard assertions in walker code that build
// Cardinality values from external input.
func (c Cardinality) IsValid() bool {
	return c == Single || c == Multiple
}

// ArchetypeTerm is one <term_definitions code="...">...<items
// id="...">value</items></term_definitions> record from an
// archetype root. Items are open-keyed by id (typical keys: "text",
// "description", "comment"). The OPT carries a single canonical
// language per document; multi-language ontology is not supported yet.
type ArchetypeTerm struct {
	// Code is the at-code this definition applies to (e.g. "at0000").
	Code string
	// Items maps the items/@id attribute (e.g. "text") to its
	// character-data value. Every accessor read returns a defensive copy,
	// so callers may mutate the returned map.
	Items map[string]string
}

// TermBinding is one <term_bindings terminology="..."><items
// code="...">...</items></term_bindings> entry from an archetype
// root. The code may be a bare at-code ("at0013") or an AQL-like
// path ("/data[at0002]/events[at0006]/..."); callers should treat
// it opaquely until compile-time resolution lands.
type TermBinding struct {
	// Terminology is the value of the terminology attribute on the
	// containing <term_bindings> block (e.g. "SNOMED-CT", "LOINC").
	Terminology string
	// NodeOrPath is the items/@code attribute: either an at-code
	// or an AQL-like locator path.
	NodeOrPath string
	// Target is the bound external terminology code.
	Target CodedTermRef
}

// CodedTermRef is the openEHR CODE_PHRASE shape used inside
// term_bindings: a terminology id + a code string. It is a flat record
// here rather than the rm type, because the template package is
// stdlib-only.
type CodedTermRef struct {
	TerminologyID string
	CodeString    string
}
