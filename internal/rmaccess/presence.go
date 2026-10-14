package rmaccess

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// The presence readings the generated access calls by name. The reading by Go
// kind (pointer, interface, string, number or boolean, list or hash) and the
// zero test of a class held by value are generated; these are the readings
// that depend on what an attribute is, not on how Go holds it. internal/bmmgen
// names each one (fixedPresence, intervalBoundPresence and contentPresence in
// render_rmaccess.go), so a function renamed or retyped here fails to compile
// in access_gen.go.

// The three readings § REQ-112 fixes. Each replaces the reading the field's Go
// kind or class would give.
//
//   - presentIntervalBound: an interval bound is absent only when its side is
//     marked unbounded and the bound is empty (§ REQ-112 Driver).
//   - presentEHRStatusSubject: EHR_STATUS subject, a PARTY_SELF held by value,
//     is always present (§ REQ-112 Value-typed mandatory presence).
//   - presentArchetypedID and presentArchetypedRMVersion: ARCHETYPED
//     archetype_id and rm_version are always present; whether they are empty
//     is the ARCHETYPED entry's to report.

// presentIntervalBound reports whether an interval bound is present: it is
// absent only when its side is marked unbounded (unbounded) and the bound is
// empty, as [rm.IsEmptyIntervalBound] reads it. A bound on a closed side is
// present whatever it holds, and so is a bound that is not empty beside its
// own open flag.
func presentIntervalBound[T any](unbounded bool, bound T) bool {
	return !unbounded || !rm.IsEmptyIntervalBound(bound)
}

// presentEHRStatusSubject reports EHR_STATUS subject present, always: a
// PARTY_SELF held by value cannot tell an absent subject from an empty one.
func presentEHRStatusSubject(rm.PartySelf) bool { return true }

// presentArchetypedID reports ARCHETYPED archetype_id present, always: an
// absent, a null and an empty archetype_id decode to the same zero value, and
// an empty one is reported by the ARCHETYPED entry at archetype_id/value.
func presentArchetypedID(rm.ArchetypeID) bool { return true }

// presentArchetypedRMVersion reports ARCHETYPED rm_version present, always,
// for the reason presentArchetypedID gives; an empty rm_version is reported
// under Rm_version_valid.
func presentArchetypedRMVersion(string) bool { return true }

// The content rules, keyed by the attribute's declared RM type. The generator
// applies them at an attribute the RM marks mandatory, and only there: at an
// optional attribute a DV_TEXT, a DV_CODED_TEXT or a CODE_PHRASE whose content
// is missing is present, unless it is its zero value or, held through an
// interface, nil or a nil pointer, so the walk enters it and reports what it
// lacks.

// presentText reports whether a DV_TEXT, held directly or through an interface,
// is present: it is not nil, not a nil pointer, and its value is not empty. A
// DV_CODED_TEXT behind the interface is read by the DV_TEXT it extends.
func presentText(v rm.DVTextLike) bool {
	if v == nil {
		return false
	}
	t, ok := rm.AsDVText(v)
	return ok && t.Value != ""
}

// presentCodedText reports whether a DV_CODED_TEXT held by value is present:
// its value or its defining code is not empty.
func presentCodedText(v rm.DVCodedText) bool {
	return v.Value != "" || presentCodePhrase(v.DefiningCode)
}

// presentCodePhrase reports whether a CODE_PHRASE held by value is present:
// its code_string or its terminology is not empty.
func presentCodePhrase(v rm.CodePhrase) bool {
	return v.CodeString != "" || v.TerminologyID.Value != ""
}
