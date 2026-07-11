package rm

// like_accessors.go: parent-struct helpers that complement the
// REQ-052 narrow polymorphic interfaces (DVTextLike, DVURILike,
// AuditDetailsLike, PartyIdentifiedLike, ObjectRefLike).
//
// The interfaces themselves and their Get-prefixed accessor methods
// (GetValue, GetDefiningCode, …) live in like_interfaces.go and are
// the preferred surface for scalar field reads. The helpers in this
// file recover the FULL parent struct from any subtype payload —
// useful at validation sites that consume a concrete `rm.DVText` or
// `rm.AuditDetails` value rather than reading scalars one at a time.
//
// Compat shims (DVTextValueOf, DVURIValueOf) delegate to the new
// interface methods and stay for callers migrating off the
// pre-ergonomics closed type-switch pattern.
//
// BMM-BUMP AUDIT (ADR 0001 step 10):
//
//   When a BMM bump introduces a NEW concrete subtype of any
//   `<Parent>Like`-bearing class, the generator's field-lift picks
//   the new subtype up automatically — but TWO hand-edited files need
//   updates:
//
//     1. like_interfaces.go — add the marker line
//        `func (NewSubtype) is<Parent>Like() {}` AND a method body
//        for every accessor on the parent interface.
//     2. THIS FILE — add a `case *NewSubtype:` arm to the matching
//        closed type-switch so `AsDVText` / `AuditDetailsBase` /
//        `PartyIdentifiedBase` / `ObjectRefBase` recover the parent
//        struct from the new subtype.
//
//   The bump set to audit on:
//
//     DV_TEXT       DV_URI       AUDIT_DETAILS
//     PARTY_IDENTIFIED   OBJECT_REF
//
//   Add a round-trip test for the new subtype under
//   `openehr/serialize/canjson/polymorphic_decode_test.go` so
//   PROBE-038's substitution guarantee covers it.

// DVTextValueOf returns the `value` rendition of any DV_TEXT subtype.
// Returns "" when the interface is nil.
//
// Compat shim: prefer `v.GetValue()` directly — that method now lives
// on [DVTextLike] (see openehr/rm/like_interfaces.go). This helper
// stays for callers migrating off the pre-Phase-1 closed type-switch
// pattern.
func DVTextValueOf(v DVTextLike) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case *DVText:
		if t == nil {
			return ""
		}
		return t.GetValue()
	case *DVCodedText:
		if t == nil {
			return ""
		}
		return t.GetValue()
	}
	return v.GetValue()
}

// AsDVText returns the DVText payload of v (the parent struct
// embedded in every subtype). The second return is true when v is
// non-nil and a known DVTextLike concrete type. Useful at validation
// sites that previously consumed a `rm.DVText` value.
func AsDVText(v DVTextLike) (DVText, bool) {
	switch t := v.(type) {
	case *DVText:
		if t == nil {
			return DVText{}, false
		}
		return *t, true
	case DVText:
		return t, true
	case *DVCodedText:
		if t == nil {
			return DVText{}, false
		}
		return t.DVText, true
	case DVCodedText:
		return t.DVText, true
	}
	return DVText{}, false
}

// DVURIValueOf returns the URI string carried by any DV_URI subtype.
//
// Compat shim: prefer `v.GetValue()` directly — that method now lives
// on [DVURILike] (see openehr/rm/like_interfaces.go). This helper
// stays for callers migrating off the pre-Phase-1 closed type-switch
// pattern.
func DVURIValueOf(v DVURILike) string {
	if v == nil {
		return ""
	}
	return v.GetValue()
}

// AuditDetailsBase returns the AUDIT_DETAILS payload of v (the parent
// struct embedded in ATTESTATION). The second return is true when v
// is non-nil and a known AuditDetailsLike concrete type.
func AuditDetailsBase(v AuditDetailsLike) (AuditDetails, bool) {
	switch t := v.(type) {
	case *AuditDetails:
		if t == nil {
			return AuditDetails{}, false
		}
		return *t, true
	case AuditDetails:
		return t, true
	case *Attestation:
		if t == nil {
			return AuditDetails{}, false
		}
		return t.AuditDetails, true
	case Attestation:
		return t.AuditDetails, true
	}
	return AuditDetails{}, false
}

// PartyIdentifiedBase returns the PARTY_IDENTIFIED payload of v (the
// parent struct embedded in PARTY_RELATED). The second return is true
// when v is non-nil and a known PartyIdentifiedLike concrete type.
func PartyIdentifiedBase(v PartyIdentifiedLike) (PartyIdentified, bool) {
	switch t := v.(type) {
	case *PartyIdentified:
		if t == nil {
			return PartyIdentified{}, false
		}
		return *t, true
	case PartyIdentified:
		return t, true
	case *PartyRelated:
		if t == nil {
			return PartyIdentified{}, false
		}
		return t.PartyIdentified, true
	case PartyRelated:
		return t.PartyIdentified, true
	}
	return PartyIdentified{}, false
}

// ObjectRefBase returns the OBJECT_REF payload of v (the parent
// struct embedded in every OBJECT_REF subtype). Closed type-switch
// over the registered ObjectRefLike concrete types.
func ObjectRefBase(v ObjectRefLike) (ObjectRef, bool) {
	switch t := v.(type) {
	case *ObjectRef:
		if t == nil {
			return ObjectRef{}, false
		}
		return *t, true
	case ObjectRef:
		return t, true
	case *LocatableRef:
		if t == nil {
			return ObjectRef{}, false
		}
		return t.ObjectRef, true
	case LocatableRef:
		return t.ObjectRef, true
	case *AccessGroupRef:
		if t == nil {
			return ObjectRef{}, false
		}
		return t.ObjectRef, true
	case AccessGroupRef:
		return t.ObjectRef, true
	case *PartyRef:
		if t == nil {
			return ObjectRef{}, false
		}
		return t.ObjectRef, true
	case PartyRef:
		return t.ObjectRef, true
	}
	return ObjectRef{}, false
}
