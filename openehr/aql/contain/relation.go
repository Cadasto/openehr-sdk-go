package contain

import (
	"sync"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Relation is a containment admissibility relation over the pinned RM plus a
// set of overlay edges (REQ-160). Obtain the default relation with [Default];
// extend it with [Relation.WithOverlay]. A Relation is immutable after
// construction and safe for concurrent use.
type Relation struct {
	h  rminfo.Hierarchy
	l  rminfo.Lookup
	al rminfo.AttributeLister

	overlays []Edge

	// Derived from the pin, immutable after construction and shared across
	// WithOverlay copies: the by-value composition adjacency (concrete class →
	// concrete by-value successors) and the known-class set.
	byValue map[string][]string
	known   map[string]bool
}

var (
	defaultOnce sync.Once
	defaultRel  *Relation
)

// Default returns the default relation — the pinned RM's by-value graph plus
// the REQ-160 overlay edges. It is built once and memoized; callers MUST NOT
// mutate the returned value (there is no exported mutator).
func Default() *Relation {
	defaultOnce.Do(func() {
		defaultRel = build(rminfo.Default, defaultOverlays())
	})
	return defaultRel
}

// WithOverlay returns a copy of r extended with the given overlay edges
// (REQ-160 § Extensibility). r is unchanged. Endpoints are canonicalised; a
// BMM-known endpoint matches by conformance, an unknown one by exact name.
func (r *Relation) WithOverlay(edges ...Edge) *Relation {
	if len(edges) == 0 {
		return r
	}
	cp := &Relation{
		h:       r.h,
		l:       r.l,
		al:      r.al,
		byValue: r.byValue, // shared, read-only
		known:   r.known,   // shared, read-only
	}
	cp.overlays = make([]Edge, len(r.overlays), len(r.overlays)+len(edges))
	copy(cp.overlays, r.overlays)
	for _, e := range edges {
		cp.overlays = append(cp.overlays, Edge{From: canon(e.From), To: canon(e.To), ByReference: e.ByReference})
	}
	return cp
}

// Containable reports whether rmType is a legal CONTAINS operand at all
// (REQ-160 § Containable operands): Admissible for a class conforming to
// LOCATABLE / VERSIONED_OBJECT / VERSION, for EHR, or for any overlay-edge
// endpoint of this relation; Never for a known non-containable class (a DV_*
// among them); UnknownClass for a class the relation does not know.
func (r *Relation) Containable(rmType string) Verdict {
	c := canon(rmType)
	overlayEndpoint := r.namesOverlayEndpoint(c)
	if !r.known[c] && !overlayEndpoint {
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
func (r *Relation) CanContain(ancestor, descendant string) Verdict {
	a, d := canon(ancestor), canon(descendant)
	va, vd := r.Containable(a), r.Containable(d)
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
// asserted between two known classes. HRID decomposition delegates to REQ-120's
// canonical [rm.ParseArchetypeID].
func (r *Relation) ArchetypeMatches(rmType, archetypeID string) Verdict {
	aid, err := rm.ParseArchetypeID(archetypeID)
	if err != nil {
		return UnknownClass
	}
	entity := canon(aid.RMEntity())
	declared := canon(rmType)
	if !r.known[entity] || !r.known[declared] {
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

// --- construction ---------------------------------------------------------

func build(lk rminfo.Lookup, overlays []Edge) *Relation {
	h, _ := lk.(rminfo.Hierarchy)
	al, _ := lk.(rminfo.AttributeLister)
	r := &Relation{h: h, l: lk, al: al, overlays: overlays}

	r.known = make(map[string]bool)
	for _, c := range lk.KnownRMTypes() {
		r.known[c] = true
	}
	r.byValue = r.deriveByValue()
	return r
}

// deriveByValue computes, for every concrete class, its by-value successor
// concretes (REQ-160 § Reachability semantics). Reference-typed attributes
// (OBJECT_REF and descendants) and primitive/generic attribute types are
// skipped; an attribute typed as an abstract class expands to that class's
// concrete descendants. Only concrete classes are walked — inherited attributes
// are already flattened onto them by rminfo.
func (r *Relation) deriveByValue() map[string][]string {
	adj := make(map[string][]string, len(r.known))
	for c := range r.known {
		if abstract, _ := r.h.IsAbstract(c); abstract {
			continue
		}
		succ := make(map[string]bool)
		for _, attr := range r.al.AttributeNames(c) {
			if r.isInfrastructureAttr(c, attr) { // LOCATABLE/PATHABLE housekeeping, not content
				continue
			}
			at, ok := r.l.AttributeRMType(c, attr)
			if !ok || !r.known[at] { // primitive / generic (String, Integer, T) or unknown
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

func (r *Relation) isReference(class string) bool {
	conf, known := r.h.ConformsTo(class, "OBJECT_REF")
	return known && conf
}

// isInfrastructureAttr reports whether an attribute is LOCATABLE/PATHABLE
// housekeeping — name, uid, links, feeder_audit, archetype_details,
// archetype_node_id, and the parent back-reference — rather than archetyped
// content. AQL CONTAINS navigates the archetype/content structure, not this
// metadata: without the exclusion an ELEMENT would reach CLUSTER through
// LOCATABLE.feeder_audit → FEEDER_AUDIT_DETAILS.other_details (an ITEM_STRUCTURE),
// which the RM permits but AQL containment does not (REQ-160 § Reachability
// semantics; acceptance row ELEMENT CONTAINS CLUSTER = Never).
func (r *Relation) isInfrastructureAttr(rmType, attr string) bool {
	decl, ok := r.h.DeclaredOn(rmType, attr)
	return ok && (decl == "LOCATABLE" || decl == "PATHABLE")
}

// --- route closure --------------------------------------------------------

// route computes the pair verdict for two containable operands by route
// closure (REQ-160 § Reachability semantics · ByReference): Admissible when a
// route crosses no ByReference edge, ByReference when routes exist but all cross
// a ByReference edge, Never when no route connects the pair.
func (r *Relation) route(a, d string) Verdict {
	starts := r.expand(a)
	targets := r.expand(d)
	if intersects(r.reachable(starts, false), targets) {
		return Admissible
	}
	if intersects(r.reachable(starts, true), targets) {
		return ByReference
	}
	return Never
}

// reachable returns the set of nodes reachable from starts by at least one edge
// (depth >= 1). When allowRef is false, ByReference overlay edges are excluded.
func (r *Relation) reachable(starts []string, allowRef bool) map[string]bool {
	reached := make(map[string]bool)
	seen := make(map[string]bool, len(starts))
	queue := make([]string, 0, len(starts))
	for _, s := range starts {
		if !seen[s] {
			seen[s] = true
			queue = append(queue, s)
		}
	}
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
	return reached
}

// successors returns the by-value and overlay successors of a single node.
func (r *Relation) successors(c string, allowRef bool) []string {
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
func (r *Relation) expand(class string) []string {
	if r.known[class] {
		ds, _ := r.h.ConcreteDescendants(class)
		return ds
	}
	return []string{class}
}

// endpointMatches reports whether node matches an overlay-edge endpoint:
// by conformance when the endpoint is BMM-known, by exact name otherwise
// (REQ-160 § Extensibility).
func (r *Relation) endpointMatches(node, endpoint string) bool {
	if r.known[endpoint] {
		conf, _ := r.h.ConformsTo(node, endpoint)
		return conf
	}
	return node == endpoint
}

// namesOverlayEndpoint reports whether c is (or, for a known class, conforms to)
// an endpoint of any overlay edge of this relation — which makes c a known,
// containable class of the relation (REQ-160 § Containable operands).
func (r *Relation) namesOverlayEndpoint(c string) bool {
	for _, e := range r.overlays {
		if r.endpointMatches(c, e.From) || r.endpointMatches(c, e.To) {
			return true
		}
	}
	return false
}

func (r *Relation) conformsToAny(c string, supers ...string) bool {
	for _, s := range supers {
		if conf, ok := r.h.ConformsTo(c, s); ok && conf {
			return true
		}
	}
	return false
}

// --- helpers --------------------------------------------------------------

// canon folds an ASCII class token to the BMM's canonical UPPER_SNAKE spelling
// (REQ-160 § Reachability semantics — ASCII-case-insensitive matching). Only
// a–z are folded; underscores and digits are left as-is, and a non-ASCII token
// simply fails to match any known class.
func canon(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
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
