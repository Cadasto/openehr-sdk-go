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
// Concrete-typed RM slots declared as DV_URI admit Liskov substitution
// by DV_EHR_URI on the wire.
//
// Members today: DVURI, DVEHRURI. The single accessor exposes the
// URI string carried by both.
type DVURILike interface {
	isDVURILike()
	// GetValue returns the URI string. Always non-empty on valid
	// wire data per the RM invariant.
	GetValue() string
}

func (d DVURI) isDVURILike()        {}
func (d DVURI) GetValue() string    { return d.Value }
func (d DVEHRURI) isDVURILike()     {}
func (d DVEHRURI) GetValue() string { return d.Value }

// --- AuditDetailsLike -----------------------------------------------

// AuditDetailsLike is the SDK-GAP-11 narrow polymorphic interface for
// AUDIT_DETAILS. Concrete-typed RM slots declared as AUDIT_DETAILS
// (Version.commit_audit, Contribution.audit, …) admit Liskov
// substitution by ATTESTATION on the wire.
//
// Members today: AuditDetails, Attestation. Accessors mirror the
// AUDIT_DETAILS parent's structurally-required fields — callers
// reaching into Attestation-only fields (Reason, IsPending, Proof, …)
// type-assert to *Attestation as usual.
type AuditDetailsLike interface {
	isAuditDetailsLike()
	// GetSystemID returns the logical EHR system identifier.
	GetSystemID() string
	// GetTimeCommitted returns the commit timestamp.
	GetTimeCommitted() DVDateTime
	// GetChangeType returns the change-type coded value
	// (openEHR Terminology `audit change type`).
	GetChangeType() DVCodedText
	// GetCommitter returns the party that committed the change.
	GetCommitter() PartyProxy
	// GetDescription returns the optional reason-for-committal text
	// envelope; second return is false when not set.
	GetDescription() (DVTextLike, bool)
}

func (a AuditDetails) isAuditDetailsLike()          {}
func (a AuditDetails) GetSystemID() string          { return a.SystemID }
func (a AuditDetails) GetTimeCommitted() DVDateTime { return a.TimeCommitted }
func (a AuditDetails) GetChangeType() DVCodedText   { return a.ChangeType }
func (a AuditDetails) GetCommitter() PartyProxy     { return a.Committer }
func (a AuditDetails) GetDescription() (DVTextLike, bool) {
	if a.Description == nil {
		return nil, false
	}
	return a.Description, true
}
func (a Attestation) isAuditDetailsLike()          {}
func (a Attestation) GetSystemID() string          { return a.SystemID }
func (a Attestation) GetTimeCommitted() DVDateTime { return a.TimeCommitted }
func (a Attestation) GetChangeType() DVCodedText   { return a.ChangeType }
func (a Attestation) GetCommitter() PartyProxy     { return a.Committer }
func (a Attestation) GetDescription() (DVTextLike, bool) {
	if a.Description == nil {
		return nil, false
	}
	return a.Description, true
}

// --- PartyIdentifiedLike --------------------------------------------

// PartyIdentifiedLike is the SDK-GAP-11 narrow polymorphic interface
// for PARTY_IDENTIFIED. Concrete-typed RM slots declared as
// PARTY_IDENTIFIED (EVENT_CONTEXT.health_care_facility,
// Participation.performer, …) admit Liskov substitution by
// PARTY_RELATED on the wire.
//
// Members today: PartyIdentified, PartyRelated. Accessors expose the
// PARTY_IDENTIFIED parent's structurally-relevant fields. PARTY_RELATED
// adds `Relationship`, which callers type-assert for.
type PartyIdentifiedLike interface {
	isPartyIdentifiedLike()
	// GetName returns the optional human-readable name; second
	// return is false when the field is nil.
	GetName() (string, bool)
	// GetIdentifiers returns the (possibly empty) formal identifier
	// list — nil and empty are equivalent at the call site.
	GetIdentifiers() []DVIdentifier
	// GetExternalRef returns the optional reference to external
	// demographic detail; second return is false when nil.
	GetExternalRef() (*PartyRef, bool)
}

func (p PartyIdentified) isPartyIdentifiedLike() {}
func (p PartyIdentified) GetName() (string, bool) {
	if p.Name == nil {
		return "", false
	}
	return *p.Name, true
}
func (p PartyIdentified) GetIdentifiers() []DVIdentifier { return p.Identifiers }
func (p PartyIdentified) GetExternalRef() (*PartyRef, bool) {
	if p.ExternalRef == nil {
		return nil, false
	}
	return p.ExternalRef, true
}
func (p PartyRelated) isPartyIdentifiedLike() {}
func (p PartyRelated) GetName() (string, bool) {
	if p.Name == nil {
		return "", false
	}
	return *p.Name, true
}
func (p PartyRelated) GetIdentifiers() []DVIdentifier { return p.Identifiers }
func (p PartyRelated) GetExternalRef() (*PartyRef, bool) {
	if p.ExternalRef == nil {
		return nil, false
	}
	return p.ExternalRef, true
}

// --- ObjectRefLike --------------------------------------------------

// ObjectRefLike is the SDK-GAP-11 narrow polymorphic interface for
// OBJECT_REF. Concrete-typed RM slots declared as OBJECT_REF admit
// Liskov substitution by ACCESS_GROUP_REF, LOCATABLE_REF, or PARTY_REF
// on the wire.
//
// Members today: ObjectRef, AccessGroupRef, LocatableRef, PartyRef.
// Accessors expose the OBJECT_REF parent's structurally-required
// fields. LOCATABLE_REF adds `Path` (and a typed UIDBasedID id);
// callers reaching for subtype-specific fields type-assert as usual.
type ObjectRefLike interface {
	isObjectRefLike()
	// GetID returns the ObjectID identifier.
	GetID() ObjectID
	// GetNamespace returns the namespace string.
	GetNamespace() string
	// GetType returns the RM class name of the referred object
	// (`PARTY`, `PERSON`, `ANY`, …).
	GetType() string
}

func (o ObjectRef) isObjectRefLike()          {}
func (o ObjectRef) GetID() ObjectID           { return o.ID }
func (o ObjectRef) GetNamespace() string      { return o.Namespace }
func (o ObjectRef) GetType() string           { return o.Type }
func (a AccessGroupRef) isObjectRefLike()     {}
func (a AccessGroupRef) GetID() ObjectID      { return a.ID }
func (a AccessGroupRef) GetNamespace() string { return a.Namespace }
func (a AccessGroupRef) GetType() string      { return a.Type }
func (l LocatableRef) isObjectRefLike()       {}

// LocatableRef shadows ObjectRef.ID with its own typed UIDBasedID id.
// The interface contract is on the OBJECT_REF parent's ID (the
// ObjectID-typed field embedded via ObjectRef); GetID returns that
// inherited field, NOT the LocatableRef-specific UIDBasedID. Callers
// who want the UIDBasedID id type-assert to *LocatableRef.
func (l LocatableRef) GetID() ObjectID      { return l.ObjectRef.ID }
func (l LocatableRef) GetNamespace() string { return l.Namespace }
func (l LocatableRef) GetType() string      { return l.Type }
func (p PartyRef) isObjectRefLike()         {}
func (p PartyRef) GetID() ObjectID          { return p.ID }
func (p PartyRef) GetNamespace() string     { return p.Namespace }
func (p PartyRef) GetType() string          { return p.Type }
