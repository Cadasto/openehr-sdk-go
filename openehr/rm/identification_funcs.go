package rm

// REQ-120 — RM identifier parsing and derivation.
//
// Hand-written behaviour for the openEHR BASE identification types whose
// derived components bmmgen emits as suppressed stubs (manual_impl.go).
// Each fallible form has a canonical package-level Parse… entry point
// returning (T, error); the BMM-signature methods are best-effort and
// never panic on malformed input (best-effort lexical decomposition;
// use Parse… for validation). This is the single canonical home for
// the identifier lexical forms — see docs/specifications/rm-functions.md
// § REQ-120 and ADR 0011.

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ErrMalformedID is the sentinel returned (wrapped) by the identifier
// Parse… functions when an input string does not match the normative
// openEHR lexical form. Detect with errors.Is(err, rm.ErrMalformedID).
var ErrMalformedID = errors.New("rm: malformed identifier")

// UID concrete-subtype discriminators. The three UID grammars are
// mutually exclusive (openEHR BASE base_types §5.5); test most specific
// first: a UUID, then an ISO OID (dotted decimals), then INTERNET_ID as
// the reverse-domain fallback.
var (
	uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	oidRE  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
)

// detectUID builds the concrete UID for a root/id string by matching
// its lexical shape. Never fails: an unrecognised shape is treated as an
// INTERNET_ID (the reverse-domain fallback), so callers get a typed,
// non-nil UID even for ad-hoc inputs.
func detectUID(s string) UID {
	switch {
	case uuidRE.MatchString(s):
		return Uuid{Value: s}
	case oidRE.MatchString(s):
		return ISOOID{Value: s}
	default:
		return InternetID{Value: s}
	}
}

// UIDValue returns the canonical string value carried by a UID, or ""
// when u is nil. A convenience for callers (e.g. openehr/client/ehr)
// that need the lexical form back from a derived UID without a type
// switch of their own.
func UIDValue(u UID) string {
	switch v := u.(type) {
	case Uuid:
		return v.Value
	case *Uuid:
		return v.Value
	case ISOOID:
		return v.Value
	case *ISOOID:
		return v.Value
	case InternetID:
		return v.Value
	case *InternetID:
		return v.Value
	}
	return ""
}

// ObjectIDValue returns the raw string value carried by any concrete
// OBJECT_ID, and whether id was a recognised, non-nil type. Unlike
// UIDValue, callers can distinguish "" as an actual empty value from ""
// meaning "could not be read" (a nil or unrecognised id) — silently
// treating the latter as an empty value risks emitting an unencodable
// or misleading reference. REQ-120.
func ObjectIDValue(id ObjectID) (value string, ok bool) {
	switch v := id.(type) {
	case HierObjectID:
		return v.Value, true
	case *HierObjectID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	case ObjectVersionID:
		return v.Value, true
	case *ObjectVersionID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	case GenericID:
		return v.Value, true
	case *GenericID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	case ArchetypeID:
		return v.Value, true
	case *ArchetypeID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	case TemplateID:
		return v.Value, true
	case *TemplateID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	case TerminologyID:
		return v.Value, true
	case *TerminologyID:
		if v == nil {
			return "", false
		}
		return v.Value, true
	default:
		return "", false
	}
}

// --- UID_BASED_ID (root '::' extension) ---------------------------------

// uidBasedRoot returns the part left of the first "::" (or the whole
// string when absent), as a concrete UID.
func uidBasedRoot(value string) UID {
	root, _, _ := strings.Cut(value, "::")
	return detectUID(root)
}

// uidBasedExtension returns the part right of the first "::" (the joined
// remainder), or "" when there is no "::".
func uidBasedExtension(value string) string {
	_, ext, _ := strings.Cut(value, "::")
	return ext
}

// Root returns the namespace identifier — the part left of the first
// "::" separator, or the whole value when absent. REQ-120.
func (h *HierObjectID) Root() UID { return uidBasedRoot(h.Value) }

// Extension returns the local identifier — the part right of the first
// "::" separator, or "" when absent. REQ-120.
func (h *HierObjectID) Extension() string { return uidBasedExtension(h.Value) }

// HasExtension reports whether an extension part is present. REQ-120.
func (h *HierObjectID) HasExtension() bool { return uidBasedExtension(h.Value) != "" }

// Root returns the namespace identifier — for an OBJECT_VERSION_ID this
// equals object_id (the part left of the first "::"). REQ-120.
func (o *ObjectVersionID) Root() UID { return uidBasedRoot(o.Value) }

// Extension returns the part right of the first "::" — for a full
// OBJECT_VERSION_ID this is "creating_system_id::version_tree_id".
// REQ-120.
func (o *ObjectVersionID) Extension() string { return uidBasedExtension(o.Value) }

// HasExtension reports whether an extension part is present. REQ-120.
func (o *ObjectVersionID) HasExtension() bool { return uidBasedExtension(o.Value) != "" }

// --- OBJECT_VERSION_ID (object_id '::' creating_system_id '::' version_tree_id) ---

// objectVersionParts splits a raw OBJECT_VERSION_ID value into its three
// "::"-separated segments, best-effort (empty strings for missing parts).
func objectVersionParts(value string) (objectID, creatingSystemID, versionTreeID string) {
	objectID, rest, ok := strings.Cut(value, "::")
	if !ok {
		return objectID, "", ""
	}
	creatingSystemID, versionTreeID, _ = strings.Cut(rest, "::")
	return objectID, creatingSystemID, versionTreeID
}

// ObjectID returns the logical object identifier — the part before the
// first "::". REQ-120.
func (o *ObjectVersionID) ObjectID() UID {
	id, _, _ := objectVersionParts(o.Value)
	return detectUID(id)
}

// CreatingSystemID returns the identifier of the system that created
// this version — the part between the two "::" separators. REQ-120.
func (o *ObjectVersionID) CreatingSystemID() UID {
	_, sys, _ := objectVersionParts(o.Value)
	return detectUID(sys)
}

// VersionTreeID returns the version-tree identifier — the part after the
// last "::". REQ-120.
func (o *ObjectVersionID) VersionTreeID() VersionTreeID {
	_, _, v := objectVersionParts(o.Value)
	return VersionTreeID{Value: v}
}

// IsBranch reports whether this version identifier denotes a branch
// (i.e. its version_tree_id is a 3-part branch form). REQ-120.
func (o *ObjectVersionID) IsBranch() bool {
	v := o.VersionTreeID()
	return v.IsBranch()
}

// ParseObjectVersionID validates s against the OBJECT_VERSION_ID lexical
// form `object_id '::' creating_system_id '::' version_tree_id` (three
// non-empty "::"-separated parts, the last a valid VERSION_TREE_ID) and
// returns the value. Returns ErrMalformedID (wrapped) otherwise.
func ParseObjectVersionID(s string) (ObjectVersionID, error) {
	parts := strings.Split(s, "::")
	if len(parts) != 3 {
		return ObjectVersionID{}, fmt.Errorf("%w: object_version_id %q must be object_id::creating_system_id::version_tree_id", ErrMalformedID, s)
	}
	if slices.Contains(parts, "") {
		return ObjectVersionID{}, fmt.Errorf("%w: object_version_id %q has an empty segment", ErrMalformedID, s)
	}
	if _, err := ParseVersionTreeID(parts[2]); err != nil {
		// Chain the inner error so it is unwrappable; it already wraps
		// ErrMalformedID, so errors.Is(…, ErrMalformedID) still holds and
		// the prefix is not doubled.
		return ObjectVersionID{}, fmt.Errorf("object_version_id %q: %w", s, err)
	}
	return ObjectVersionID{Value: s}, nil
}

// --- VERSION_TREE_ID (trunk_version [ '.' branch_number '.' branch_version ]) ---

// TrunkVersion returns the trunk version number — the first dot-segment.
// REQ-120.
func (v *VersionTreeID) TrunkVersion() string {
	return strings.SplitN(v.Value, ".", 2)[0]
}

// BranchNumber returns the branch number, or "" for a trunk-only id
// (openEHR Void). REQ-120.
func (v *VersionTreeID) BranchNumber() string {
	if p := strings.Split(v.Value, "."); len(p) == 3 {
		return p[1]
	}
	return ""
}

// BranchVersion returns the branch version, or "" for a trunk-only id
// (openEHR Void). REQ-120.
func (v *VersionTreeID) BranchVersion() string {
	if p := strings.Split(v.Value, "."); len(p) == 3 {
		return p[2]
	}
	return ""
}

// IsBranch reports whether this is a 3-part branch identifier. Like the
// other derivation methods it is purely lexical and best-effort (it does
// not validate that the parts are integers ≥ 1); use ParseVersionTreeID
// for well-formedness. REQ-120.
func (v *VersionTreeID) IsBranch() bool {
	return len(strings.Split(v.Value, ".")) == 3
}

// IsFirst reports whether this identifies the first version
// (trunk_version == "1"). REQ-120.
func (v *VersionTreeID) IsFirst() bool {
	return v.TrunkVersion() == "1"
}

// ParseVersionTreeID validates s against the VERSION_TREE_ID lexical
// form (1 or 3 dot-separated integers, each >= 1) and returns the value.
func ParseVersionTreeID(s string) (VersionTreeID, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 1 && len(parts) != 3 {
		return VersionTreeID{}, fmt.Errorf("%w: version_tree_id %q must have 1 or 3 dot-separated parts", ErrMalformedID, s)
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return VersionTreeID{}, fmt.Errorf("%w: version_tree_id %q part %q is not an integer >= 1", ErrMalformedID, s, p)
		}
	}
	return VersionTreeID{Value: s}, nil
}

// --- ARCHETYPE_ID -------------------------------------------------------
// rm_originator '-' rm_name '-' rm_entity '.' concept { '-' specialisation }* '.v' version_id

// archetypeFields splits an ARCHETYPE_ID value at its first and last
// '.' into qualified_rm_entity, domain_concept, and the version field
// ("vN"). ok is false when fewer than two '.' separators are present.
func archetypeFields(value string) (qualified, domain, version string, ok bool) {
	first := strings.IndexByte(value, '.')
	last := strings.LastIndexByte(value, '.')
	if first <= 0 || last <= first {
		return "", "", "", false
	}
	return value[:first], value[first+1 : last], value[last+1:], true
}

// QualifiedRMEntity returns `rm_originator-rm_name-rm_entity`. REQ-120.
func (a *ArchetypeID) QualifiedRMEntity() string {
	q, _, _, _ := archetypeFields(a.Value)
	return q
}

// DomainConcept returns the concept including any specialisation chain,
// e.g. "lab_result-cholesterol". REQ-120.
func (a *ArchetypeID) DomainConcept() string {
	_, d, _, _ := archetypeFields(a.Value)
	return d
}

// VersionID returns the major version, e.g. "1" for a trailing ".v1".
// REQ-120.
func (a *ArchetypeID) VersionID() string {
	_, _, v, _ := archetypeFields(a.Value)
	return strings.TrimPrefix(v, "v")
}

// RMOriginator returns the first hyphen-segment of qualified_rm_entity,
// e.g. "openEHR". REQ-120.
func (a *ArchetypeID) RMOriginator() string { return archetypeQualifiedPart(a.Value, 0) }

// RMName returns the second hyphen-segment, e.g. "EHR". REQ-120.
func (a *ArchetypeID) RMName() string { return archetypeQualifiedPart(a.Value, 1) }

// RMEntity returns the third hyphen-segment, e.g. "OBSERVATION". REQ-120.
func (a *ArchetypeID) RMEntity() string { return archetypeQualifiedPart(a.Value, 2) }

func archetypeQualifiedPart(value string, i int) string {
	q, _, _, ok := archetypeFields(value)
	if !ok {
		return ""
	}
	parts := strings.Split(q, "-")
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

// Specialisation returns the last specialisation segment of the domain
// concept, or "" when the concept is unspecialised. REQ-120.
func (a *ArchetypeID) Specialisation() string {
	d := a.DomainConcept()
	if i := strings.LastIndexByte(d, '-'); i >= 0 {
		return d[i+1:]
	}
	return ""
}

// ParseArchetypeID validates s against the ARCHETYPE_ID lexical form and
// returns the value. Returns ErrMalformedID (wrapped) otherwise.
func ParseArchetypeID(s string) (ArchetypeID, error) {
	q, _, version, ok := archetypeFields(s)
	if !ok {
		return ArchetypeID{}, fmt.Errorf("%w: archetype_id %q must be rm_originator-rm_name-rm_entity.concept.vN", ErrMalformedID, s)
	}
	if len(strings.Split(q, "-")) < 3 {
		return ArchetypeID{}, fmt.Errorf("%w: archetype_id %q qualified_rm_entity must be originator-name-entity", ErrMalformedID, s)
	}
	if len(version) < 2 || version[0] != 'v' {
		return ArchetypeID{}, fmt.Errorf("%w: archetype_id %q version must be .vN", ErrMalformedID, s)
	}
	return ArchetypeID{Value: s}, nil
}

// --- TERMINOLOGY_ID (name [ '(' version ')' ]) --------------------------

// Name returns the terminology name — the part before "(" when a
// parenthesised version is present, else the whole value. REQ-120.
func (t *TerminologyID) Name() string {
	if i := strings.IndexByte(t.Value, '('); i >= 0 && strings.HasSuffix(t.Value, ")") {
		return t.Value[:i]
	}
	return t.Value
}

// VersionID returns the version inside the parentheses, or "" when no
// "(version)" is present. REQ-120.
func (t *TerminologyID) VersionID() string {
	i := strings.IndexByte(t.Value, '(')
	if i < 0 || !strings.HasSuffix(t.Value, ")") {
		return ""
	}
	return t.Value[i+1 : len(t.Value)-1]
}

// ParseTerminologyID validates s against the TERMINOLOGY_ID lexical form
// (`name [ '(' version ')' ]`) and returns the value.
func ParseTerminologyID(s string) (TerminologyID, error) {
	if s == "" {
		return TerminologyID{}, fmt.Errorf("%w: terminology_id is empty", ErrMalformedID)
	}
	if strings.ContainsRune(s, '(') != strings.HasSuffix(s, ")") {
		return TerminologyID{}, fmt.Errorf("%w: terminology_id %q has unbalanced parentheses", ErrMalformedID, s)
	}
	return TerminologyID{Value: s}, nil
}

// --- LOCATABLE_REF ------------------------------------------------------

// AsURI builds the URI form of the reference: the namespace as scheme,
// then the id value, then "/" + path when the path is non-empty (a path
// that already begins with "/" is appended verbatim to avoid a double
// slash). REQ-120.
func (l *LocatableRef) AsURI() string {
	var b strings.Builder
	b.WriteString(l.Namespace)
	b.WriteByte(':')
	b.WriteString(uidBasedIDValue(l.ID))
	if l.Path != nil && *l.Path != "" {
		if !strings.HasPrefix(*l.Path, "/") {
			b.WriteByte('/')
		}
		b.WriteString(*l.Path)
	}
	return b.String()
}

// uidBasedIDValue extracts the string value of a UID_BASED_ID concrete
// type (HIER_OBJECT_ID or OBJECT_VERSION_ID), or "" when nil/unknown.
func uidBasedIDValue(id UIDBasedID) string {
	switch v := id.(type) {
	case HierObjectID:
		return v.Value
	case *HierObjectID:
		return v.Value
	case ObjectVersionID:
		return v.Value
	case *ObjectVersionID:
		return v.Value
	}
	return ""
}
