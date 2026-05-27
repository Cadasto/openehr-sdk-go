package rm

// like_accessors.go: non-generated accessors for the SDK-GAP-11
// narrow polymorphic interfaces (DVTextLike, DVURILike,
// AuditDetailsLike, PartyIdentifiedLike, ObjectRefLike).
//
// The narrow interfaces lift concrete-typed RM slots to admit Liskov
// substitution per the openEHR RM (e.g. LOCATABLE.name DV_TEXT may
// carry a DV_CODED_TEXT instance at runtime). Callers that previously
// reached for a struct field directly (`v.Name.Value`) need a tiny
// accessor that handles both the parent and any subtype shape. These
// helpers are the v0.x migration path; v1.0 may promote them to
// interface methods once the field-vs-method naming clash is resolved.
//
// BMM-BUMP AUDIT (ADR 0001 step):
//
//   When a BMM bump introduces a NEW concrete subtype of any
//   `<Parent>Like`-bearing class, the generator's marker emission picks
//   it up automatically (the new subtype's struct gains the
//   `is<Parent>Like()` method via the BMM ancestors graph). The closed
//   type-switches below DO NOT — each new subtype needs an explicit
//   `case *NewSubtype:` arm here so the helper recovers the parent
//   payload from it. Without the audit, callers using e.g.
//   `rm.AsDVText(name)` against a new DV_TEXT descendant would silently
//   return (DVText{}, false).
//
//   On every BMM bump where `bmmdiff` reports added classes whose
//   ancestors include a class in this set:
//
//     DV_TEXT       DV_URI       AUDIT_DETAILS
//     PARTY_IDENTIFIED   OBJECT_REF
//
//   add a `case *Foo:` arm to the matching helper here and pin a
//   round-trip test for the new subtype under
//   `openehr/serialize/canjson/polymorphic_decode_test.go`.

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

// DVURIValueOf returns the `value` rendition of any DV_URI subtype.
func DVURIValueOf(v DVURILike) string {
	switch t := v.(type) {
	case *DVURI:
		if t == nil {
			return ""
		}
		return t.Value
	case DVURI:
		return t.Value
	case *DVEHRURI:
		if t == nil {
			return ""
		}
		return t.Value
	case DVEHRURI:
		return t.Value
	}
	return ""
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
