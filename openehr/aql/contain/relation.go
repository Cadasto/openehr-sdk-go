package contain

import (
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TypeRelation is the containment admissibility relation over RM TYPE NAMES
// (REQ-160): the set of ordered pairs (ancestor, descendant) an AQL CONTAINS
// can connect, and whether the route is by-value or by-reference. "Relation"
// is the mathematical sense — a set of ordered pairs — and the RM type is what
// it relates; it holds no archetype, template, or instance data, and answers
// nothing about whether a query will return rows.
//
// Most of it is derived from the pinned BMM's by-value composition graph, but
// three families of fact are carried as cited overlay data because the BMM
// cannot express them: the version tier (VERSION.data is generic, so no
// introspection pairs a VERSIONED_X with its payload X), the EHR's references
// to its versioned objects, and reference hops an engine resolves such as
// FOLDER.items. See § Overlay edges.
//
// Obtain the default relation with [Default]; extend it with
// [TypeRelation.WithOverlay]. A TypeRelation is immutable after construction
// (the internal verdict memo is concurrency-safe) and safe for concurrent use.
//
// A nil *TypeRelation is the default relation, on the receiver as well as at
// every seam that takes one (REQ-160 § Nil and zero relations): each method
// below answers what [Default] would answer rather than panicking, so a caller
// can hold a nil relation meaning "the default" and still extend or query it.
// The ZERO TypeRelation is a different thing — a real relation that knows no
// classes and answers UnknownClass for every one.
type TypeRelation struct {
	h  rminfo.Hierarchy
	lk rminfo.Lookup
	al rminfo.AttributeLister

	overlays []Edge

	// Derived from the pin, immutable after construction and shared across
	// WithOverlay copies: the by-value composition adjacency (concrete class →
	// concrete by-value successors, canonical spellings) and the known-class
	// map, keyed by the canon-folded name so matching stays case-insensitive
	// even against a mixed-case canonical spelling (Iso8601_timezone and the
	// other foundation classes the pin ships), valued with that spelling.
	byValue map[string][]string
	known   map[string]string

	// memo caches the per-start-node reachability closure on first use, one
	// map per route mode (0: ByReference edges excluded, 1: included). Stored
	// sets are read-only after LoadOrStore. Not shared across WithOverlay
	// copies — a copy's overlay set differs, so it starts a memo of its own.
	memo [2]sync.Map
}

var defaultRel = sync.OnceValue(func() *TypeRelation {
	return build(rminfo.Default, defaultOverlays())
})

// Default returns the default relation — the pinned RM's by-value graph plus
// the REQ-160 overlay edges. It is built once and memoized; there is no
// exported mutator, so the returned value cannot be altered.
func Default() *TypeRelation {
	return defaultRel()
}

// orDefault reads a nil receiver as the default relation (REQ-160 § Nil and
// zero relations). Every exported method funnels through it, so nil — the
// documented spelling of "use the default" wherever a *TypeRelation is passed
// — means the same thing when a method is called on one, instead of panicking
// on caller-constructible input (REQ-025 § No panics). The unexported helpers
// do not repeat the check: they are reachable only through an exported method
// or through build, both of which hold a real relation by the time they call
// one.
func (r *TypeRelation) orDefault() *TypeRelation {
	if r == nil {
		return Default()
	}
	return r
}

// WithOverlay returns a copy of r extended with the given overlay edges
// (REQ-160 § Extensibility). r is unchanged, and a nil r extends the default
// relation. Endpoints are canonicalised; a BMM-known endpoint matches by
// conformance, an unknown one by exact name. An edge with an empty endpoint
// names nothing and is ignored. The result is never nil.
func (r *TypeRelation) WithOverlay(edges ...Edge) *TypeRelation {
	r = r.orDefault()
	if len(edges) == 0 {
		return r
	}
	cp := &TypeRelation{
		h:       r.h,
		lk:      r.lk,
		al:      r.al,
		byValue: r.byValue, // shared, read-only
		known:   r.known,   // shared, read-only
	}
	cp.overlays = make([]Edge, len(r.overlays), len(r.overlays)+len(edges))
	copy(cp.overlays, r.overlays)
	for _, e := range edges {
		if e.From == "" || e.To == "" {
			continue
		}
		cp.overlays = append(cp.overlays, Edge{From: canon(e.From), To: canon(e.To), ByReference: e.ByReference})
	}
	return cp
}

// Containable reports whether rmType is a legal CONTAINS operand at all
// (REQ-160 § Containable operands): Admissible for a class conforming to
// LOCATABLE / VERSIONED_OBJECT / VERSION, for EHR, or for any overlay-edge
// endpoint of this relation; Never for a known non-containable class (a DV_*
// among them); UnknownClass for a class the relation does not know.
func (r *TypeRelation) Containable(rmType string) Verdict {
	r = r.orDefault()
	return r.containable(r.resolve(rmType))
}

// containable is Containable over an already-resolved name and its known flag,
// so the pair question resolves each operand once (see CanContain).
func (r *TypeRelation) containable(c string, isKnown bool) Verdict {
	overlayEndpoint := r.namesOverlayEndpoint(c)
	if !isKnown && !overlayEndpoint {
		return UnknownClass
	}
	if overlayEndpoint || c == "EHR" || r.conformsToAny(c, "LOCATABLE", "VERSIONED_OBJECT", "VERSION") {
		return Admissible
	}
	return Never
}

// CanContain reports the pair verdict for ancestor CONTAINS descendant
// (REQ-160 § Verdicts, § Reachability semantics). The pair question is total:
// UnknownClass if either operand is unknown; otherwise Never if either
// operand's containability is Never; otherwise the route verdict.
func (r *TypeRelation) CanContain(ancestor, descendant string) Verdict {
	r = r.orDefault()
	a, aKnown := r.resolve(ancestor)
	d, dKnown := r.resolve(descendant)
	va, vd := r.containable(a, aKnown), r.containable(d, dKnown)
	if va == UnknownClass || vd == UnknownClass {
		return UnknownClass
	}
	if va == Never || vd == Never {
		return Never
	}
	return r.route(a, d)
}

// ArchetypeMatches reports whether a literal archetype predicate's HRID type
// segment conforms to the declared class (REQ-160 § Archetype/class
// conformance): Admissible when it conforms, Never on a genuine mismatch, and
// UnknownClass when the HRID is unparseable or either the declared class or the
// HRID type segment is not a class the relation knows — a mismatch is only ever
// asserted between two known classes. "Knows" here means the pinned BMM:
// overlay-named classes count as unknown, since conformance cannot be answered
// for them and UnknownClass is the conservative verdict (never a false
// mismatch). HRID decomposition delegates to REQ-120's canonical
// [rm.ParseArchetypeID].
func (r *TypeRelation) ArchetypeMatches(rmType, archetypeID string) Verdict {
	r = r.orDefault()
	aid, err := rm.ParseArchetypeID(archetypeID)
	if err != nil {
		return UnknownClass
	}
	entity, entityKnown := r.resolve(aid.RMEntity())
	declared, declaredKnown := r.resolve(rmType)
	if !entityKnown || !declaredKnown {
		return UnknownClass
	}
	conf, ok := r.h.ConformsTo(entity, declared)
	if !ok {
		return UnknownClass
	}
	if conf {
		return Admissible
	}
	return Never
}

// Unavoidable reports whether EVERY containment route from ancestor to
// descendant passes through via — the relation's reachability recomputed with
// one intermediate class excluded. REQ-164 § The redundant-step ruling names
// this query; REQ-160 § Reachability semantics owns the route vocabulary it is
// asked in, and adds no verdict of its own for it (the answer is a proof or
// the absence of one, not a fourth verdict).
//
// True is a PROOF that `ancestor a CONTAINS via v CONTAINS descendant d`
// selects exactly what `ancestor a CONTAINS descendant d` selects, so the via
// step narrows nothing — the RM fact lint's aql_contains_redundant_step rests
// on (REQ-164 § Path-shape checks).
// Unavoidable("EHR", "COMPOSITION", "OBSERVATION") is true: every route from an
// EHR down to an OBSERVATION passes a COMPOSITION.
// Unavoidable("EHR", "SECTION", "OBSERVATION") is false: an observation sitting
// directly in a composition's content is reached without any section, so
// dropping the section step widens the result.
//
// Every failure to prove answers FALSE, which is the silent direction for the
// consumer (REQ-164 § Path-shape checks — where a fact is not provable the
// check stays silent rather than guessing). Four of them:
//
//   - a class this relation does not know, in any of the three positions;
//   - a class it knows but does not admit as a CONTAINS operand
//     (§ Containable operands) — a DV_* among them;
//   - a via standing for the ancestor's or the descendant's own kinds:
//     excluding it would remove an ENDPOINT of the very question, which is not
//     the question "is there a way round it";
//   - a pair no route connects at all. Nothing is proved by a step that changes
//     an empty result into an empty result, and an impossible containment is
//     REQ-161's aql_impossible_containment to report, never a redundant step.
//
// Routes are read at their WIDEST — ByReference edges included — on both halves
// of the question. A bypass crossing a reference hop is still a bypass on an
// engine that resolves the hop (REQ-160 § Overlay edges), so counting it keeps
// this method from proving a step redundant that such an engine can see is not.
//
// Abstract classes stand for their concrete kinds throughout, exactly as they
// do for [TypeRelation.CanContain]: excluding via excludes every concrete class
// conforming to it.
//
// A nil receiver answers as [Default] does (REQ-160 § Nil and zero relations).
func (r *TypeRelation) Unavoidable(ancestor, via, descendant string) bool {
	r = r.orDefault()
	a, aKnown := r.resolve(ancestor)
	v, vKnown := r.resolve(via)
	d, dKnown := r.resolve(descendant)
	if r.containable(a, aKnown) != Admissible ||
		r.containable(v, vKnown) != Admissible ||
		r.containable(d, dKnown) != Admissible {
		return false
	}

	starts, targets := r.expand(a), r.expand(d)
	blocked := make(map[string]bool)
	for _, c := range r.expand(v) {
		blocked[c] = true
	}
	if intersects(blocked, starts) || intersects(blocked, targets) {
		return false
	}
	if !r.anyReaches(starts, targets, true) {
		return false
	}
	for _, s := range starts {
		if intersects(r.reachableAvoiding(s, blocked), targets) {
			return false // a route round via
		}
	}
	return true
}

// --- construction ---------------------------------------------------------

func build(lk rminfo.Lookup, overlays []Edge) *TypeRelation {
	r := &TypeRelation{lk: lk}
	r.overlays = make([]Edge, 0, len(overlays))
	for _, e := range overlays { // canonicalised like WithOverlay, one rule for both paths
		if e.From == "" || e.To == "" {
			continue
		}
		r.overlays = append(r.overlays, Edge{From: canon(e.From), To: canon(e.To), ByReference: e.ByReference})
	}

	h, hOK := lk.(rminfo.Hierarchy)
	al, alOK := lk.(rminfo.AttributeLister)
	if !hOK || !alOK {
		// Without the optional rminfo capability interfaces the BMM half of
		// the relation cannot be derived. No panics in library code: degrade
		// to overlay-only — known stays empty, so a name only the BMM knows
		// answers UnknownClass (the fail-safe verdict, REQ-160 § Verdicts),
		// while an overlay endpoint keeps matching by exact folded name and
		// stays containable (COMPOSITION, EHR and the rest of the default
		// table among them). rminfo.Default implements both; this arm exists
		// for a caller-substituted Lookup.
		return r
	}
	r.h, r.al = h, al

	r.known = make(map[string]string)
	for _, c := range lk.KnownRMTypes() {
		r.known[canon(c)] = c
	}
	r.byValue = r.deriveByValue()
	return r
}

// resolve folds name (REQ-160 § Reachability semantics — ASCII-case-insensitive
// matching) and maps it to the pin's canonical spelling when known. An unknown
// name — a dialect or consumer-edge class — stays in its folded form, which is
// how overlay endpoints are stored, so exact-name matching remains
// case-insensitive too.
func (r *TypeRelation) resolve(name string) (string, bool) {
	c := canon(name)
	if s, ok := r.known[c]; ok {
		return s, true
	}
	return c, false
}

// deriveByValue computes, for every concrete class, its by-value successor
// concretes (REQ-160 § Reachability semantics). Reference-typed attributes
// (OBJECT_REF and descendants) and primitive/generic attribute types are
// skipped; an attribute typed as an abstract class expands to that class's
// concrete descendants. Only concrete classes are walked — inherited attributes
// are already flattened onto them by rminfo.
func (r *TypeRelation) deriveByValue() map[string][]string {
	adj := make(map[string][]string, len(r.known))
	for _, c := range r.known {
		if abstract, _ := r.h.IsAbstract(c); abstract {
			continue
		}
		succ := make(map[string]bool)
		for _, attr := range r.al.AttributeNames(c) {
			if r.isInfrastructureAttr(c, attr) { // LOCATABLE/PATHABLE housekeeping, not content
				continue
			}
			at, ok := r.lk.AttributeRMType(c, attr)
			if !ok {
				continue
			}
			if _, isKnown := r.known[canon(at)]; !isKnown { // primitive / generic (String, Integer, T)
				continue
			}
			if r.isReference(at) { // OBJECT_REF and descendants terminate reachability
				continue
			}
			ds, _ := r.h.ConcreteDescendants(at)
			for _, d := range ds {
				succ[d] = true
			}
		}
		if len(succ) > 0 {
			adj[c] = keysOf(succ)
		}
	}
	return adj
}

func (r *TypeRelation) isReference(class string) bool {
	conf, known := r.h.ConformsTo(class, "OBJECT_REF")
	return known && conf
}

// isInfrastructureAttr reports whether an attribute is declared on LOCATABLE
// or PATHABLE — housekeeping, not archetyped content (REQ-160 § Reachability
// semantics). On the current pin that is the six LOCATABLE attributes (name,
// uid, links, feeder_audit, archetype_details, archetype_node_id); PATHABLE
// declares no properties there, so its arm is forward-looking pin insurance.
// AQL CONTAINS navigates the archetype/content structure, not this metadata:
// without the exclusion an ELEMENT would reach CLUSTER through
// LOCATABLE.feeder_audit (a FEEDER_AUDIT) → originating_system_audit (a
// FEEDER_AUDIT_DETAILS) → other_details (an ITEM_STRUCTURE), which the RM
// permits but AQL containment does not (acceptance row ELEMENT CONTAINS
// CLUSTER = Never).
func (r *TypeRelation) isInfrastructureAttr(rmType, attr string) bool {
	decl, ok := r.h.DeclaredOn(rmType, attr)
	return ok && (decl == "LOCATABLE" || decl == "PATHABLE")
}

// --- route closure --------------------------------------------------------

// route computes the pair verdict for two containable operands by route
// closure (REQ-160 § Reachability semantics · ByReference): Admissible when a
// route crosses no ByReference edge, ByReference when routes exist but all cross
// a ByReference edge, Never when no route connects the pair.
func (r *TypeRelation) route(a, d string) Verdict {
	starts := r.expand(a)
	targets := r.expand(d)
	if r.anyReaches(starts, targets, false) {
		return Admissible
	}
	if r.anyReaches(starts, targets, true) {
		return ByReference
	}
	return Never
}

// anyReaches reports whether any start node reaches any target node by at
// least one edge.
func (r *TypeRelation) anyReaches(starts, targets []string, allowRef bool) bool {
	for _, s := range starts {
		if intersects(r.reachableFrom(s, allowRef), targets) {
			return true
		}
	}
	return false
}

// reachableFrom returns the set of nodes reachable from start by at least one
// edge (depth >= 1). When allowRef is false, ByReference overlay edges are
// excluded. The closure is memoized on first use per (start, mode); stored
// sets are read-only and MUST NOT be mutated.
func (r *TypeRelation) reachableFrom(start string, allowRef bool) map[string]bool {
	idx := 0
	if allowRef {
		idx = 1
	}
	if v, ok := r.memo[idx].Load(start); ok {
		return v.(map[string]bool)
	}
	reached := make(map[string]bool)
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for _, nb := range r.successors(c, allowRef) {
			reached[nb] = true // nb is at depth >= 1 (a successor of a queued node)
			if !seen[nb] {
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	v, _ := r.memo[idx].LoadOrStore(start, reached)
	return v.(map[string]bool)
}

// reachableAvoiding is [TypeRelation.reachableFrom] over the graph with a set
// of VERTICES removed: a node in blocked is neither reached nor traversed. It
// is the one query [TypeRelation.Unavoidable] needs and the only reader of it.
// Routes are read at their widest (ByReference edges included) — see that
// method for why, and TestUnavoidableCountsAByReferenceBypass for the witness
// that narrowing this call to the non-reference closure changes an answer.
//
// It deliberately neither READS nor WRITES the relation's memo. The memo caches
// the closure of the WHOLE graph per (start, mode); excluding a vertex is a
// different graph, so reading from it would answer the wrong question and
// writing to it would corrupt every later verdict. REQ-160 § Reachability
// semantics has verdicts MAY be memoized, never must, so a fresh walk per call
// is within contract — and this walk runs per lint operand, not per query row.
func (r *TypeRelation) reachableAvoiding(start string, blocked map[string]bool) map[string]bool {
	reached := make(map[string]bool)
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		for _, nb := range r.successors(c, true) {
			if blocked[nb] {
				continue
			}
			reached[nb] = true // nb is at depth >= 1, exactly as in reachableFrom
			if !seen[nb] {
				seen[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return reached
}

// successors returns the by-value and overlay successors of a single node.
func (r *TypeRelation) successors(c string, allowRef bool) []string {
	out := make([]string, 0, len(r.byValue[c]))
	out = append(out, r.byValue[c]...)
	for _, e := range r.overlays {
		if e.ByReference && !allowRef {
			continue
		}
		if r.endpointMatches(c, e.From) {
			out = append(out, r.expand(e.To)...)
		}
	}
	return out
}

// expand returns the concrete descendants of a BMM-known class, or the class
// itself (exact) for a name the pin does not know — a dialect/consumer endpoint.
func (r *TypeRelation) expand(class string) []string {
	if s, ok := r.known[canon(class)]; ok {
		ds, _ := r.h.ConcreteDescendants(s)
		return ds
	}
	return []string{class}
}

// endpointMatches reports whether node matches an overlay-edge endpoint:
// by conformance when the endpoint is BMM-known, by exact name otherwise
// (REQ-160 § Extensibility). Endpoints are stored canon-folded; node arrives
// resolved (a canonical spelling, or the folded form of an unknown name).
func (r *TypeRelation) endpointMatches(node, endpoint string) bool {
	if s, ok := r.known[endpoint]; ok {
		conf, _ := r.h.ConformsTo(node, s)
		return conf
	}
	return node == endpoint
}

// namesOverlayEndpoint reports whether c is (or, for a known class, conforms to)
// an endpoint of any overlay edge of this relation — which makes c a known,
// containable class of the relation (REQ-160 § Containable operands).
func (r *TypeRelation) namesOverlayEndpoint(c string) bool {
	for _, e := range r.overlays {
		if r.endpointMatches(c, e.From) || r.endpointMatches(c, e.To) {
			return true
		}
	}
	return false
}

func (r *TypeRelation) conformsToAny(c string, supers ...string) bool {
	for _, s := range supers {
		if conf, ok := r.h.ConformsTo(c, s); ok && conf {
			return true
		}
	}
	return false
}

// --- helpers --------------------------------------------------------------

// canon folds an ASCII class token to its lookup key — the upper-cased form the
// known map is keyed by (REQ-160 § Reachability semantics —
// ASCII-case-insensitive matching). The key is not itself the pin's canonical
// spelling: known maps the key to that spelling, and the two differ for the
// mixed-case foundation classes the pin ships (Point_interval and kin). Only
// a–z are folded; underscores and digits are left as-is, and a non-ASCII token
// simply fails to match any known class.
func canon(s string) string {
	var b []byte
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c - ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

func intersects(set map[string]bool, names []string) bool {
	for _, n := range names {
		if set[n] {
			return true
		}
	}
	return false
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
