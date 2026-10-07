package terminology

import (
	"iter"
	"slices"
)

// ID is the TERMINOLOGY_ID.value of the openEHR Terminology itself, the
// terminology every [Group] and every openEHR-issued [CodeSet] in this
// package is defined in. A DV_CODED_TEXT the SDK builds from a group's code
// carries it as its terminology id. The code sets from other issuers
// (languages, countries, character sets, media types) are not defined in it:
// each carries its own external id, such as "ISO_639-1", given by
// [CodeSet.ExternalID].
const ID = "openehr"

// openEHRIssuer is the issuer the pin names for the code sets openEHR defines
// itself. Their membership is exact; see [CodeSet.Has].
const openEHRIssuer = "openehr"

// Concept is one coded entry of a [Group]: the code and its English rubric.
type Concept struct {
	Code   string
	Rubric string
}

// Group is one closed, source-ordered openEHR terminology group: the value
// set an RM invariant such as EVENT_CONTEXT.Setting_valid names. Every group
// is a package-level variable generated from the pinned terminology file
// (see openehr_gen.go); a nil pointer is inert, with every method reporting
// absence.
//
// A code's rubric is a property of the group, not of the code: the openEHR
// terminology gives
// 532 the rubric "complete" in version lifecycle state and "completed" in
// instruction states. Look codes up on the group that governs them.
type Group struct {
	id, name string
	concepts []Concept
	byCode   map[string]int
	byRubric map[string]int
}

// newGroup builds a group from its openehr_id, its display name and its
// concepts in source order. Only the generated tables call it, and its
// precondition — every concept present, and unique by both code and rubric so
// the two indexes are invertible — is what makes that safe: the generator
// refuses a pin that breaks it (see internal/termgen) and termgen-verify gates
// the result, so newGroup itself trusts its input rather than re-checking it.
func newGroup(id, name string, concepts []Concept) *Group {
	g := &Group{
		id:       id,
		name:     name,
		concepts: concepts,
		byCode:   make(map[string]int, len(concepts)),
		byRubric: make(map[string]int, len(concepts)),
	}
	for i, c := range concepts {
		g.byCode[c.Code] = i
		g.byRubric[c.Rubric] = i
	}
	return g
}

// ID returns the group's openehr_id, e.g. "audit_change_type".
func (g *Group) ID() string {
	if g == nil {
		return ""
	}
	return g.id
}

// Name returns the group's display name, e.g. "audit change type".
func (g *Group) Name() string {
	if g == nil {
		return ""
	}
	return g.name
}

// Len returns the number of concepts in the group.
func (g *Group) Len() int {
	if g == nil {
		return 0
	}
	return len(g.concepts)
}

// All yields the group's concepts in the order the pinned file
// lists them. A nil group yields nothing.
func (g *Group) All() iter.Seq[Concept] {
	if g == nil {
		return func(func(Concept) bool) {}
	}
	return slices.Values(g.concepts)
}

// Has reports whether code is a member of the group. This is the membership
// verdict an RM invariant over the group asks for.
func (g *Group) Has(code string) bool {
	if g == nil {
		return false
	}
	_, ok := g.byCode[code]
	return ok
}

// Rubric returns the pinned rubric for code, and false when code is not a
// member of the group.
func (g *Group) Rubric(code string) (string, bool) {
	if g == nil {
		return "", false
	}
	i, ok := g.byCode[code]
	if !ok {
		return "", false
	}
	return g.concepts[i].Rubric, true
}

// Code returns the code whose rubric is rubric, and false when no member of
// the group carries it. Rubrics are unique within a group in the pin.
func (g *Group) Code(rubric string) (string, bool) {
	if g == nil {
		return "", false
	}
	i, ok := g.byRubric[rubric]
	if !ok {
		return "", false
	}
	return g.concepts[i].Code, true
}

// CodeSet is one closed, source-ordered code set of the pinned terminology: a
// value set whose members are bare codes with no rubric. Every code set is a
// package-level variable generated from the pinned terminology files (see
// openehr_gen.go); a nil pointer is inert, with every method reporting
// absence.
//
// A code set is issued either by openEHR or by an external body. The
// openEHR-issued ones, such as the normal statuses
// DV_ORDERED.Normal_status_validity names, are defined in the openEHR
// terminology itself, whose id is [ID]. The others are the openEHR
// Foundation's snapshot of an ISO or IANA register, published with the
// pinned release: [Languages] (ISO 639-1), [Countries] (ISO 3166-1),
// [CharacterSets] (IANA character sets) and [MediaTypes] (IANA media
// types). A snapshot is not the live register: a code the register holds
// and the pinned set leaves out, such as one added after the release, is not
// a member.
// [CodeSet.Issuer] and [CodeSet.ExternalID] say which kind a set is, and
// [CodeSet.Has] matches codes accordingly.
type CodeSet struct {
	id, name           string
	issuer, externalID string
	codes              []string
	// ignoreCase is set for an issuer other than openEHR. Then index is
	// keyed by each code with its ASCII letters lower-cased.
	ignoreCase bool
	index      map[string]struct{}
}

// newCodeSet builds a code set from its openehr_id, its display name, its
// issuer, its external id and its codes in source order. Only the generated
// tables call it, and its precondition — for an issuer other than openEHR,
// no two codes differ only in ASCII letter case — is what keeps the index
// one entry per code: the generator refuses a pin that breaks it (see
// internal/termgen), so newCodeSet trusts its input.
func newCodeSet(id, name, issuer, externalID string, codes []string) *CodeSet {
	s := &CodeSet{
		id:         id,
		name:       name,
		issuer:     issuer,
		externalID: externalID,
		codes:      codes,
		ignoreCase: lowerASCII(issuer) != openEHRIssuer,
		index:      make(map[string]struct{}, len(codes)),
	}
	for _, c := range codes {
		s.index[s.key(c)] = struct{}{}
	}
	return s
}

// key is the index key for code: the code as given for an openEHR-issued
// set, or with its ASCII letters lower-cased for any other.
func (s *CodeSet) key(code string) string {
	if s.ignoreCase {
		return lowerASCII(code)
	}
	return code
}

// lowerASCII returns s with the ASCII letters A to Z turned into a to z and
// every other byte left as it is. Unlike strings.ToLower it never maps a
// character outside ASCII onto an ASCII letter (the Kelvin sign onto k, say),
// so no look-alike can match a pinned code. It returns s itself, without
// allocating, when s holds no upper-case ASCII letter.
func lowerASCII(s string) string {
	for i := range len(s) {
		if c := s[i]; 'A' <= c && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if c := b[j]; 'A' <= c && c <= 'Z' {
					b[j] = c + ('a' - 'A')
				}
			}
			return string(b)
		}
	}
	return s
}

// ID returns the code set's openehr_id, e.g. "normal_statuses".
func (s *CodeSet) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

// Name returns the code set's display name, e.g. "normal statuses".
func (s *CodeSet) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Issuer returns the body the pin names as the code set's issuer: "openehr"
// for the code sets openEHR defines itself, "ISO" or "IANA" for the external
// ones. A nil code set returns "".
func (s *CodeSet) Issuer() string {
	if s == nil {
		return ""
	}
	return s.issuer
}

// ExternalID returns the identifier the pin gives the code set, e.g.
// "ISO_639-1" for the languages, "IANA_character-sets" for the character
// sets or "openehr_normal_statuses" for the normal statuses. A nil code set
// returns "".
func (s *CodeSet) ExternalID() string {
	if s == nil {
		return ""
	}
	return s.externalID
}

// Len returns the number of codes in the code set.
func (s *CodeSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.codes)
}

// All yields the code set's codes in the order the pinned file
// lists them. A nil code set yields nothing.
func (s *CodeSet) All() iter.Seq[string] {
	if s == nil {
		return func(func(string) bool) {}
	}
	return slices.Values(s.codes)
}

// Has reports whether code is a member of the code set: a code the pinned set
// lists, and nothing else.
//
// For a code set openEHR issues, the comparison is exact, as a group's is:
// NormalStatuses.Has("h") is false. For any other issuer (ISO, IANA) it
// ignores letter case, because those registers do: Languages.Has("en-US") and
// CharacterSets.Has("utf-8") are true. Only the ASCII letters A to Z fold, so
// a character outside ASCII never matches a pinned letter, whatever it looks
// like. An alias the register knows but the pinned set leaves out is not a
// member: CharacterSets.Has("ISO-8859-1") is false beside the listed
// "ISO_8859-1:1987".
func (s *CodeSet) Has(code string) bool {
	if s == nil {
		return false
	}
	_, ok := s.index[s.key(code)]
	return ok
}

// Groups yields every group of the pinned terminology in source order.
func Groups() iter.Seq[*Group] {
	return slices.Values(groups)
}

// CodeSets yields every code set of the pinned terminology in source order:
// the openEHR-issued ones of openehr_terminology.xml first, then the external
// ones of openehr_external_terminologies.xml, each file in its own order.
func CodeSets() iter.Seq[*CodeSet] {
	return slices.Values(codeSets)
}

// GroupByID returns the group whose openehr_id is id (the identifier the
// RM's has_code_for_group_id invariants name), and false when the pinned
// terminology has no such group.
func GroupByID(id string) (*Group, bool) {
	for _, g := range groups {
		if g.ID() == id {
			return g, true
		}
	}
	return nil, false
}

// CodeSetByID returns the code set whose openehr_id is id, such as
// "normal_statuses" or "languages", and false when the pinned terminology has
// no such code set. The id is the openehr_id, not the external id: "languages"
// finds [Languages], "ISO_639-1" finds nothing.
func CodeSetByID(id string) (*CodeSet, bool) {
	for _, s := range codeSets {
		if s.ID() == id {
			return s, true
		}
	}
	return nil, false
}
