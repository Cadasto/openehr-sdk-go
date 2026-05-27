package rm

// like_interfaces.go: non-generated declarations of the SDK-GAP-11
// narrow polymorphic interfaces and their RM-shaped accessor methods.
//
// The generator (`internal/bmmgen`) populates `plan.ConcreteSubtypes`
// from the BMM ancestors graph and emits FIELDS typed as `<Parent>Like`
// — but the interface declarations themselves live here so the
// accessor methods (GetValue, GetDefiningCode, …) can be hand-curated
// without forcing the codec generator to grow a per-class method-body
// table. The dispatch path stays registry-driven (REQ-040 / typereg)
// for every wire codec; these methods are pure ergonomic glue on top.
//
// BMM-BUMP AUDIT (ADR 0001 step):
//
//   When a BMM bump introduces a NEW concrete subtype of any class in
//   this file's interface set:
//
//     DV_TEXT       DV_URI       AUDIT_DETAILS
//     PARTY_IDENTIFIED   OBJECT_REF
//
//   …the generator's field-lifting will auto-include the new subtype
//   in `plan.ConcreteSubtypes` and consumers' fields will switch type
//   on regen — but this file MUST be updated by hand to add:
//
//     1. a marker-method line (`func (NewSubtype) is<Parent>Like() {}`),
//     2. the accessor method bodies for every method on the parent's
//        narrow interface (e.g. `GetValue() string`,
//        `GetDefiningCode() (CodePhrase, bool)` for `DVTextLike`).
//
//   Otherwise the new subtype will fail to satisfy `<Parent>Like` and
//   the codec emission won't compile.
//
//   The closed type-switches in `like_accessors.go` need the same
//   audit; PROBE-038 round-trip cases under
//   `openehr/serialize/canjson/polymorphic_decode_test.go` should be
//   extended at the same time.

// --- DVTextLike -----------------------------------------------------

// DVTextLike is the SDK-GAP-11 narrow polymorphic interface for DV_TEXT.
// Concrete-typed RM slots declared as DV_TEXT (LOCATABLE.name,
// LOCATABLE.null_reason, …) admit Liskov substitution by any descendant
// per the openEHR RM; the wire decoder dispatches via typereg using
// this interface so subtype payloads survive the decode → re-marshal
// round-trip without field loss.
//
// Members today: DVText, DVCodedText. Accessors expose the parent
// `value` rendition and the optional `defining_code` (present iff the
// runtime concrete type is DVCodedText).
type DVTextLike interface {
	isDVTextLike()
	// GetValue returns the displayable text rendition of the
	// underlying DV_TEXT / DV_CODED_TEXT payload. Always non-empty on
	// valid wire data.
	GetValue() string
	// GetDefiningCode returns the optional terminology binding when
	// the underlying concrete type is DV_CODED_TEXT; the second
	// return is false on bare DV_TEXT.
	GetDefiningCode() (CodePhrase, bool)
}

func (d DVText) isDVTextLike()                            {}
func (d DVText) GetValue() string                         { return d.Value }
func (d DVText) GetDefiningCode() (CodePhrase, bool)      { return CodePhrase{}, false }
func (d DVCodedText) isDVTextLike()                       {}
func (d DVCodedText) GetValue() string                    { return d.Value }
func (d DVCodedText) GetDefiningCode() (CodePhrase, bool) { return d.DefiningCode, true }

// --- DVURILike ------------------------------------------------------

// DVURILike is the SDK-GAP-11 narrow polymorphic interface for DV_URI.
// Members: DVURI, DVEHRURI. Accessor methods land in Phase 2; the
// interface declaration + markers ship now so generated fields
// compile.
type DVURILike interface {
	isDVURILike()
}

func (d DVURI) isDVURILike()    {}
func (d DVEHRURI) isDVURILike() {}

// --- AuditDetailsLike -----------------------------------------------

// AuditDetailsLike is the SDK-GAP-11 narrow polymorphic interface for
// AUDIT_DETAILS. Members: AuditDetails, Attestation. Accessor methods
// land in Phase 2.
type AuditDetailsLike interface {
	isAuditDetailsLike()
}

func (a AuditDetails) isAuditDetailsLike() {}
func (a Attestation) isAuditDetailsLike()  {}

// --- PartyIdentifiedLike --------------------------------------------

// PartyIdentifiedLike is the SDK-GAP-11 narrow polymorphic interface
// for PARTY_IDENTIFIED. Members: PartyIdentified, PartyRelated.
// Accessor methods land in Phase 2.
type PartyIdentifiedLike interface {
	isPartyIdentifiedLike()
}

func (p PartyIdentified) isPartyIdentifiedLike() {}
func (p PartyRelated) isPartyIdentifiedLike()    {}

// --- ObjectRefLike --------------------------------------------------

// ObjectRefLike is the SDK-GAP-11 narrow polymorphic interface for
// OBJECT_REF. Members: ObjectRef, AccessGroupRef, LocatableRef,
// PartyRef. Accessor methods land in Phase 2.
type ObjectRefLike interface {
	isObjectRefLike()
}

func (o ObjectRef) isObjectRefLike()      {}
func (a AccessGroupRef) isObjectRefLike() {}
func (l LocatableRef) isObjectRefLike()   {}
func (p PartyRef) isObjectRefLike()       {}
