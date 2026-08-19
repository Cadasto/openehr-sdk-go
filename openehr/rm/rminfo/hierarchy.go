package rminfo

import (
	"slices"
)

// Hierarchy answers class-level questions about the pinned Reference Model —
// abstractness, ancestry, conformance, abstract-class expansion, and where an
// attribute is declared (REQ-048).
//
// It is an optional capability interface beside [Lookup], not an extension of
// it: widening the published Lookup interface would break every external
// implementer of it (idiom.md § Public-API stability). [Default] implements
// Hierarchy, and so does every [Lookup] returned by [New], so a synthetic
// model can exercise shapes the pinned RM does not contain. Assert for it:
//
//	if h, ok := rminfo.Default.(rminfo.Hierarchy); ok { … }
//
// **Known versus false.** Every method reports whether the class is in the
// data set at all, separately from its answer, because a caller refusing a
// query has to say which it hit: an unknown class, an abstract class, and an
// abstract class nothing concrete extends are three different answers. An
// empty ancestor list on a known class means *root*; an empty
// concrete-descendant list on a known class means *dead-end abstract*.
//
// **Closure.** Every class name the class-graph methods return — [Parents],
// [Ancestors], [ConcreteDescendants] — is itself a known class: the generator
// drops BMM ancestors outside the emitted class universe, so the graph has no
// edges pointing at names no method can answer for. That is a property of the
// data (asserted by PROBE-094), deliberately not re-imposed at run time,
// because filtering here would mask a generator defect instead of failing on
// it.
//
// [DeclaredOn] is the exception, and belongs to the attribute layer rather
// than the class graph: it names the BMM class that actually declares the
// attribute, which for one inherited from the un-emitted foundation layer is
// outside the universe. That matches [Lookup.AttributeRMType], which already
// reports primitives and generic parameters (`String`, `Integer`, `T`) — the
// attribute layer is faithful to the BMM, the graph is navigable.
type Hierarchy interface {
	// IsAbstract reports the pinned BMM's is_abstract flag for rmType, so a
	// caller can tell that naming the class denotes its concrete
	// descendants rather than one instantiable class. known=false when the
	// data set does not define the class at all.
	//
	// It is the BMM's answer verbatim, not a local verdict on
	// instantiability (REQ-047 — the BMM file wins). In particular it is
	// NOT the question "can a stored instance carry this name as _type":
	// six classes the pinned RM leaves unflagged are reported non-abstract,
	// two of them BMM interfaces this SDK renders as Go interfaces. A
	// caller that needs the decodable set must consult the type registry in
	// openehr/rm/typereg (REQ-040), which is the authority for that
	// question. Whether abstractness should widen to cover BMM interfaces
	// is open as STRAND-12.
	IsAbstract(rmType string) (abstract, known bool)

	// Parents returns rmType's immediate parent classes in BMM declaration
	// order — the faithful edge set, which a re-serialiser or a schema
	// differ needs and which a transitive closure erases. The order is NOT
	// sorted, precisely because sorting would lose it.
	//
	// known=false for a class the data set does not define; an empty result
	// with known=true is a root.
	Parents(rmType string) (parents []string, known bool)

	// Ancestors returns every ancestor of rmType, transitively and sorted.
	// The set is strict: rmType is never its own ancestor, even in a
	// synthetic model that declares it as its own parent.
	//
	// known=false for a class the data set does not define; an empty result
	// with known=true is a root — a different answer.
	Ancestors(rmType string) (ancestors []string, known bool)

	// ConformsTo reports whether sub is rmType or descends from it — the
	// relation behind AQL CONTAINS, polymorphic slot fit, and validation
	// walkers. It is reflexive and not symmetric.
	//
	// known=false when EITHER name is undefined, so "no" and "never heard
	// of it" stay distinguishable at the call site.
	ConformsTo(sub, rmType string) (conforms, known bool)

	// ConcreteDescendants returns every class naming rmType denotes: rmType
	// itself when the BMM does not mark it abstract, plus every non-abstract
	// strict descendant, sorted. This is the AQL class-expression expansion.
	//
	// known=false for a class the data set does not define; an empty result
	// with known=true is an abstract class nothing concrete extends.
	// "Concrete" here means "not BMM-abstract" — see [Hierarchy.IsAbstract]
	// for why that is not the same as "storable".
	ConcreteDescendants(rmType string) (descendants []string, known bool)

	// DeclaredOn returns the class whose BMM declaration supplied attrName
	// as seen from rmType — rmType itself, or the ancestor the inheritance
	// fold took the attribute from.
	//
	// The flattened [Lookup.AttributeRMType], [Lookup.IsContainer] and
	// [Lookup.RequiredAttributes] already answer the inheritance-RESOLVED
	// question and are unchanged by this interface; DeclaredOn recovers the
	// site that fold erases, which a reader distinguishing own from
	// inherited attributes (BMM-faithful re-serialisation, schema diffing)
	// cannot otherwise get back. The site and the attribute's reported
	// shape come from one generated record, so they cannot disagree.
	//
	// The site is faithful to the BMM, so it MAY name a class outside the
	// known universe when the attribute is inherited from the foundation
	// layer the RM target does not emit: on the pinned RM the only such
	// site is `Interval`, which declares the bounds DV_INTERVAL,
	// Point_interval and Proper_interval carry. Clamping those to the
	// carrying class would report six attributes as locally declared that
	// no RM class declares — the opposite of what this method is for.
	//
	// ok=false when rmType is undefined or does not carry attrName — never
	// a guessed class.
	DeclaredOn(rmType, attrName string) (declaringClass string, ok bool)
}

// Compile-time guarantee that the backing concrete type satisfies Hierarchy.
// Consumers assert Default.(Hierarchy) at run time; this turns a future
// signature drift into a build break rather than a silently-failing
// assertion.
var _ Hierarchy = (*lookup)(nil)

func (l *lookup) IsAbstract(rmType string) (bool, bool) {
	meta, ok := l.data[rmType]
	if !ok {
		return false, false
	}
	return meta.Abstract, true
}

func (l *lookup) Parents(rmType string) ([]string, bool) {
	meta, ok := l.data[rmType]
	if !ok {
		return nil, false
	}
	// Copy: a caller that sorts the result must not reorder the generated
	// table's BMM declaration order for the next caller.
	return slices.Clone(meta.Parents), true
}

func (l *lookup) Ancestors(rmType string) ([]string, bool) {
	meta, ok := l.data[rmType]
	if !ok {
		return nil, false
	}
	// seen starts holding rmType so the set stays strict and a cycle in a
	// synthetic model terminates instead of walking forever.
	seen := map[string]bool{rmType: true}
	var out []string
	queue := slices.Clone(meta.Parents)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		queue = append(queue, l.data[name].Parents...)
	}
	slices.Sort(out)
	return out, true
}

func (l *lookup) ConformsTo(sub, rmType string) (bool, bool) {
	if _, ok := l.data[sub]; !ok {
		return false, false
	}
	if _, ok := l.data[rmType]; !ok {
		return false, false
	}
	if sub == rmType {
		return true, true
	}
	ancestors, _ := l.Ancestors(sub)
	return slices.Contains(ancestors, rmType), true
}

func (l *lookup) ConcreteDescendants(rmType string) ([]string, bool) {
	meta, ok := l.data[rmType]
	if !ok {
		return nil, false
	}
	children := l.childIndex()
	seen := map[string]bool{rmType: true}
	var out []string
	if !meta.Abstract {
		out = append(out, rmType)
	}
	queue := slices.Clone(children[rmType])
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		if !l.data[name].Abstract {
			out = append(out, name)
		}
		queue = append(queue, children[name]...)
	}
	slices.Sort(out)
	return out, true
}

func (l *lookup) DeclaredOn(rmType, attrName string) (string, bool) {
	meta, ok := l.data[rmType]
	if !ok {
		return "", false
	}
	attr, ok := meta.Attributes[attrName]
	if !ok {
		return "", false
	}
	// An empty site is not a success: synthetic data may omit DeclaredIn,
	// and returning "" as a class name would make the caller ask about a
	// class that cannot exist.
	if attr.DeclaredIn == "" {
		return "", false
	}
	return attr.DeclaredIn, true
}

// childIndex inverts the Parents edges once, lazily — the generated tables
// carry the up-edges only, and [Hierarchy.ConcreteDescendants] needs the
// down-edges. Built on first use rather than at init so importing the package
// stays free (doc.go § Building-block weight), and sorted so a BFS over it is
// deterministic.
func (l *lookup) childIndex() map[string][]string {
	l.childOnce.Do(func() {
		l.children = make(map[string][]string, len(l.data))
		for name, meta := range l.data {
			for _, parent := range meta.Parents {
				l.children[parent] = append(l.children[parent], name)
			}
		}
		for parent := range l.children {
			slices.Sort(l.children[parent])
		}
	})
	return l.children
}
