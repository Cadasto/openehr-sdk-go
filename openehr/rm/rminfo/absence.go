package rminfo

import "strconv"

// AbsenceReason says why an RM type name is not in the class universe
// (REQ-049) — the structure behind a membership "no", which the generator
// computes on every run and, before this surface, discarded. A name can be
// undeclared by the pinned schemas, declared but in a package the generator
// skips wholesale, declared but named in its class-exclusion set, a primitive
// value rather than a modelled class, or a BMM enumeration: five different
// facts that a single boolean folds into one.
//
// The zero value, [AbsenceNone], means the name IS in the universe. That is
// deliberate: an accessor whose zero value meant "absent" could be misread as
// a membership test with inverted sense.
//
// The set is closed and exhaustive-switchable. A member may be added in a minor
// release — idiom.md § Public-API stability — so a consumer switching on it
// should handle one it does not recognise rather than assume the set it
// compiled against. Compare reasons with == or a switch; apart from
// AbsenceNone's zero the numeric values carry no contract and are never
// ordered.
//
// Where several rules match one name the generator resolves them under a fixed
// precedence — primitive, then enumeration, then excluded class, then excluded
// package — so what a name IS ranks ahead of why it was SKIPPED. Consumers see
// only the winner: this type carries one reason per name.
type AbsenceReason int

const (
	// AbsenceNone means the name is in the class universe — not absent at
	// all. It is the zero value, and the answer for exactly the names
	// [Lookup.KnownRMTypes] lists.
	AbsenceNone AbsenceReason = iota
	// AbsenceUndeclared means no pinned schema of the RM generation target
	// declares the name. It is computed rather than stored: it is the answer
	// for every out-of-universe name no absence table entry backs.
	AbsenceUndeclared
	// AbsenceExcludedPackage means the name is declared, but its BMM package
	// is one the generator skips wholesale (REQ-042 — the ehr_extract
	// package, and the functional/builtins foundation packages). WHICH
	// package is deliberately not reported; that the skip is the reason is.
	AbsenceExcludedPackage
	// AbsenceExcludedClass means the name is declared and its package is
	// emitted, but the class is in the generator's named-class exclusion set.
	AbsenceExcludedClass
	// AbsencePrimitive means the name is declared as, or mapped to, a
	// primitive_types entry — a value the RM uses rather than a class this
	// surface models. It is reported even where the generation target does
	// emit a Go type for the name (REQ-048): not in the universe is not the
	// same as no Go type exists.
	AbsencePrimitive
	// AbsenceEnumeration means the name is declared as a BMM enumeration,
	// not a class.
	AbsenceEnumeration
)

// String names the KIND of absence and nothing else — it never echoes the
// queried name, so a diagnostic built from it cannot leak one. A value outside
// the closed set formats numerically rather than masquerading as a member.
func (r AbsenceReason) String() string {
	switch r {
	case AbsenceNone:
		return "none"
	case AbsenceUndeclared:
		return "undeclared"
	case AbsenceExcludedPackage:
		return "excluded package"
	case AbsenceExcludedClass:
		return "excluded class"
	case AbsencePrimitive:
		return "primitive"
	case AbsenceEnumeration:
		return "enumeration"
	}
	return "absence reason " + strconv.Itoa(int(r))
}

// AbsenceReporter answers why a name is absent from the class universe
// (REQ-049) — the negative space behind the membership question
// [Lookup.KnownRMTypes] answers positively.
//
// It is an optional capability interface beside [Lookup], not an extension of
// it: widening the published Lookup interface — or [Hierarchy] — would break
// every external implementer of it (idiom.md § Public-API stability — prefer a
// new interface plus a runtime type-assertion to introduce optional
// behaviour), the same reasoning that keeps [AttributeLister] separate.
// [Default] implements it, and so does every [Lookup] returned by [New] and
// [NewWithAbsence]. Assert for it:
//
//	if r, ok := rminfo.Default.(rminfo.AbsenceReporter); ok { … }
type AbsenceReporter interface {
	// AbsenceReason reports why rmType is not in the class universe.
	// [AbsenceNone] — the zero value — means rmType IS in the universe.
	//
	// The answer never disagrees with the rest of the surface: a name
	// reporting a real reason reports not-known on every [Lookup] and
	// [Hierarchy] question, and a name reporting AbsenceNone is in
	// [Lookup.KnownRMTypes]. An out-of-universe name no absence entry backs
	// — including the empty string — reports [AbsenceUndeclared].
	AbsenceReason(rmType string) AbsenceReason
}

// Compile-time guarantee that the backing concrete type satisfies
// AbsenceReporter. Consumers assert Default.(AbsenceReporter) at run time;
// this turns a future signature drift on AbsenceReason into a build break
// rather than a silently-failing assertion.
var _ AbsenceReporter = (*lookup)(nil)

// NewWithAbsence constructs a Lookup over caller-supplied class data plus a
// synthetic absence table, so the absence question is reachable without the
// pinned RM (REQ-049). Intended for unit tests, like [New]; production callers
// should use [Default], whose table is generated.
//
// absence maps an out-of-universe class name to the reason it is absent. Where
// data and absence name the same class the UNIVERSE WINS — a class data
// defines always reports [AbsenceNone], whatever absence says about it,
// because membership and absence are the same question and must not disagree.
// A name in neither map reports [AbsenceUndeclared], so passing a nil absence
// table is equivalent to [New]: every out-of-universe name is then undeclared,
// which is a degraded but consistent answer rather than an error. An entry
// whose value is [AbsenceNone] is treated as ABSENT FROM THE TABLE for the
// same consistency reason — the name reports AbsenceUndeclared, and no caller
// input can make the surface report none for a name outside
// [Lookup.KnownRMTypes].
//
// The returned Lookup RETAINS both maps rather than copying them, exactly as
// [New] retains data. Callers MUST NOT mutate either afterwards — see [New]
// for what that costs.
func NewWithAbsence(data map[string]ClassMeta, absence map[string]AbsenceReason) Lookup {
	return &lookup{data: data, absence: absence}
}

func (l *lookup) AbsenceReason(rmType string) AbsenceReason {
	if _, ok := l.data[rmType]; ok {
		return AbsenceNone
	}
	// A stored AbsenceNone is table silence, not an answer: reporting none
	// for a name the class data omits would claim a membership
	// KnownRMTypes does not list.
	if reason, ok := l.absence[rmType]; ok && reason != AbsenceNone {
		return reason
	}
	return AbsenceUndeclared
}
