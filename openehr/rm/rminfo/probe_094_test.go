package rminfo_test

// PROBE-094 — the compiled-in RM class graph equals an independent reduction
// of the pinned BMM (REQ-048).
//
// The reduction below reads the pinned schemas through the openehr/bmm loader
// (REQ-045) and re-derives every answer from the raw BMM. It deliberately does
// NOT call internal/bmmgen: comparing the table against the walk that produced
// it would pass on a generator whose walk is itself wrong, which is the one
// failure this probe exists to catch. `make codegen-verify` already covers the
// other direction — that the committed table is what the generator emits.

import (
	"maps"
	"regexp"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

const probeResourcesDir = "../../../" + bmm.DefaultResourcesDir

// probeExcludedClasses mirrors internal/bmmgen's skippedClasses, written out
// HERE rather than imported. That duplication is the point: it confronts the
// generator's skip list from the other side, so a class silently added to or
// dropped from one list fails this probe rather than quietly changing the
// shipped universe.
//
// How much weight each entry carries — measured against
// openehr_base_1.3.0.bmm.json, not assumed, because the honest answer is "less
// than it looks":
//
//   - `Comparable` is the ONLY entry that decides anything today: a
//     class_definitions entry in a package no prefix below excludes.
//   - TUPLE/TUPLE1/TUPLE2/FUNCTION/ROUTINE/PROCEDURE (foundation_types.
//     functional) and Env/Math/Locale/Statistical_evaluator/
//     Quantity_converter (base_types.builtins) are class_definitions entries
//     already covered by probeExcludedPackagePrefixes; they are belt and
//     braces.
//   - Numeric/Ordered/Ordered_Numeric/Container are primitive_types-only, so
//     the reduction never consults this map for them — the IsPrimitive rule
//     excludes them on both sides. They are kept for symmetry with
//     internal/bmmgen/primitives.go, which lists them for the same reason,
//     and drift in either list for THOSE four would go undetected.
//
// The confrontation therefore bites for future class_definitions entries and
// for `Comparable`; it does not bite for the four primitive-only names.
var probeExcludedClasses = map[string]bool{
	"TUPLE": true, "TUPLE1": true, "TUPLE2": true,
	"FUNCTION": true, "ROUTINE": true, "PROCEDURE": true,
	"Env": true, "Math": true, "Locale": true,
	"Statistical_evaluator": true, "Quantity_converter": true,
	"Numeric": true, "Ordered": true, "Ordered_Numeric": true,
	"Comparable": true, "Container": true,
}

// probeExcludedPackagePrefixes mirrors REQ-042's wholesale package skips.
var probeExcludedPackagePrefixes = []string{
	"org.openehr.rm.ehr_extract",
	"org.openehr.base.foundation_types.functional",
	"org.openehr.base.foundation_types.builtins",
	"org.openehr.base.base_types.builtins",
}

// probePrimitiveMappedNames restates, as a literal, the KEY SET of
// internal/bmmgen's primitiveGoType — the 17 names the generator maps straight
// onto a Go primitive. That is not the whole of bmm-conformance.md § Primitive
// type mapping (29 rows): the table also lists the abstract and
// structured/container primitives the generator handles by other means, and it
// omits `Iso8601_timezone`, which the map carries. Written out here for the
// same reason probeExcludedClasses is: the duplication confronts the
// generator's list from the other side.
//
// It is the second half of REQ-049's primitive rule, "declared as (or mapped
// to) a § Primitive type mapping entry": the generator answers *primitive*
// when the merged schema's `primitive_types` map declares the name OR this
// mapping names it. A reduction reading only `primitive_types` would be
// NARROWER than the rule it audits, and a future BMM bump that moved a mapped
// name out of `primitive_types` would then make PROBE-098 read a
// spec-faithful answer as a generation defect.
//
// How much weight the second half carries — measured against the pinned
// schemas, not assumed: `Iso8601_timezone` is the only key below that
// `primitive_types` does not declare, and the universe SHIPS it (a
// class_definitions entry), so exclusionKindOf's in-the-universe arm answers
// first and this map decides nothing on today's corpus. Restating it fixes the
// RULE's shape, not any of the 74 rows the table currently holds.
var probePrimitiveMappedNames = map[string]bool{
	// Basic scalars.
	"Boolean": true, "Integer": true, "Integer64": true,
	"Real": true, "Double": true, "Character": true,
	"String": true, "Octet": true, "Uri": true, "Any": true,

	// ISO 8601 family, including the two abstract aliases.
	"Iso8601_date": true, "Iso8601_time": true, "Iso8601_date_time": true,
	"Iso8601_duration": true, "Iso8601_type": true, "Iso8601_timezone": true,
	"Temporal": true,
}

// bmmReduction is the independently-derived model the probe compares against.
type bmmReduction struct {
	// universe is the class set the reduction expects to be shipped.
	universe map[string]bool
	// abstract, ancestors are read straight off the BMM class.
	abstract     map[string]bool
	bmmAncestors map[string][]string
	// parents is bmmAncestors filtered to universe, declaration order kept.
	parents map[string][]string
	// classOf resolves a name to its BMM class in EITHER map, because a
	// declaration site may be a primitive_types entry (Interval).
	classOf map[string]bmm.Class
	// primitive / enumeration / packageOf carry the three facts the
	// universe rule excludes on, so exclusionReason can account for every
	// name the universe leaves out.
	//
	// primitive holds `primitive_types` DECLARATIONS only — arm (d) of
	// PROBE-098 reads it with exactly that meaning. The other half of
	// REQ-049's primitive rule, the § Primitive type mapping table, is
	// probePrimitiveMappedNames, joined in at exclusionKindOf.
	primitive   map[string]bool
	enumeration map[string]bool
	packageOf   map[string]string
}

func reducePinnedBMM(t *testing.T) *bmmReduction {
	t.Helper()
	schema, err := bmm.LoadAll("openehr_rm_1.2.0", bmm.FSResolver{Root: probeResourcesDir})
	if err != nil {
		t.Fatalf("LoadAll(openehr_rm_1.2.0): %v", err)
	}

	r := &bmmReduction{
		universe:     map[string]bool{},
		abstract:     map[string]bool{},
		bmmAncestors: map[string][]string{},
		parents:      map[string][]string{},
		classOf:      map[string]bmm.Class{},
		primitive:    map[string]bool{},
		enumeration:  map[string]bool{},
		packageOf:    packagePaths(schema),
	}
	maps.Copy(r.classOf, schema.PrimitiveTypes)
	for name := range schema.PrimitiveTypes {
		r.primitive[name] = true
	}
	for name, cls := range schema.ClassDefinitions {
		r.classOf[name] = cls
		r.abstract[name] = cls.IsAbstract()
		r.bmmAncestors[name] = cls.Ancestors()
		if _, isEnum := cls.(*bmm.Enumeration); isEnum {
			r.enumeration[name] = true
			continue
		}
		if probeExcludedClasses[name] {
			continue
		}
		if excludedPackage(r.packageOf[name]) {
			continue
		}
		r.universe[name] = true
	}
	for name := range r.universe {
		var parents []string
		for _, anc := range r.bmmAncestors[name] {
			if r.universe[anc] && !slices.Contains(parents, anc) {
				parents = append(parents, anc)
			}
		}
		r.parents[name] = parents
	}
	return r
}

// packagePaths maps every class name to the dotted package path that lists it.
func packagePaths(schema *bmm.Schema) map[string]string {
	out := map[string]string{}
	var walk func(prefix string, pkgs map[string]*bmm.Package)
	walk = func(prefix string, pkgs map[string]*bmm.Package) {
		for name, pkg := range pkgs {
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			for _, cls := range pkg.Classes {
				out[cls] = path
			}
			walk(path, pkg.Packages)
		}
	}
	walk("", schema.Packages)
	return out
}

// exclusionKind is WHICH rule of the universe rule keeps a name out — the
// typed half of the same account exclusionReason renders as prose. The two are
// derived in one place because PROBE-098 (REQ-049) compares this kind against
// the shipped AbsenceReason, and deriving the answer twice would let the two
// probes disagree about the same name.
//
// The numeric values carry nothing: exclusionKindOf's switch order is what
// fixes the precedence, and the declaration order below mirrors it so a reader
// sees the same order in both places.
type exclusionKind int

const (
	// inTheUniverse and the last two kinds are NOT exclusions — they are
	// the three outcomes exclusionReason reports as unaccounted.
	inTheUniverse exclusionKind = iota
	// The four real exclusions, in REQ-049's fixed precedence order: what
	// a name IS ranks ahead of why it was SKIPPED.
	excludedAsPrimitive
	excludedAsEnumeration
	excludedByClassName
	excludedByPackagePrefix
	notDeclaredAtAll
	unaccountedFor
)

// exclusionKindOf accounts for a name the universe does not contain, under
// REQ-049's fixed precedence: primitive, then enumeration, then excluded class,
// then excluded package.
//
// The primitive arm reads BOTH halves of REQ-049's primitive row — declared as
// a `primitive_types` entry, or mapped to one by § Primitive type mapping —
// because that is the rule the generator applies. Reading only the first half
// would leave this reduction narrower than what it audits.
func (r *bmmReduction) exclusionKindOf(name string) exclusionKind {
	switch {
	case r.universe[name]:
		return inTheUniverse
	case r.primitive[name] || probePrimitiveMappedNames[name]:
		return excludedAsPrimitive
	case r.enumeration[name]:
		return excludedAsEnumeration
	case probeExcludedClasses[name]:
		return excludedByClassName
	case excludedPackage(r.packageOf[name]):
		return excludedByPackagePrefix
	case r.classOf[name] == nil:
		return notDeclaredAtAll
	default:
		return unaccountedFor
	}
}

// exclusionReason renders exclusionKindOf's answer as the prose a failure
// message needs. The reasons are exactly the universe rule's, so
// accounted=false means the name is either a defect in the shipped table or a
// change to the generation target the probe has not been told about.
//
// The two returns are separate because "" is not a usable sentinel: a name that
// IS in the universe and a name that cannot be accounted for are opposite
// outcomes, and collapsing them once let the second pass as the first.
func (r *bmmReduction) exclusionReason(name string) (reason string, accounted bool) {
	switch r.exclusionKindOf(name) {
	case inTheUniverse:
		return "in the universe", false
	case excludedAsPrimitive:
		return "declared as, or mapped to, a primitive (bmm-conformance.md § Primitive type mapping)", true
	case excludedAsEnumeration:
		return "an enumeration", true
	case excludedByClassName:
		return "in the declared excluded-class set", true
	case excludedByPackagePrefix:
		return "in an excluded package (" + r.packageOf[name] + ")", true
	case notDeclaredAtAll:
		// NOT an exclusion: a BMM ancestor the pinned schemas do not define
		// is a REQ-047 divergence to raise upstream, not a licence to drop
		// the edge. Saying so here is the difference between this arm
		// catching that day and blessing it.
		return "NOT DEFINED by the pinned schemas — a REQ-047 divergence, not an exclusion", false
	case unaccountedFor:
		return "no declared exclusion matches", false
	}
	return "no declared exclusion matches", false
}

func excludedPackage(path string) bool {
	for _, p := range probeExcludedPackagePrefixes {
		if path == p || (len(path) > len(p) && path[:len(p)] == p && path[len(p)] == '.') {
			return true
		}
	}
	return false
}

// TestProbe094UniverseEqualsThePinnedBMM — arm (a). Both directions: a missing
// class is an unanswerable question, an extra one is a name the pinned schemas
// do not define.
func TestProbe094UniverseEqualsThePinnedBMM(t *testing.T) {
	r := reducePinnedBMM(t)
	shipped := rminfo.Default.KnownRMTypes()

	for _, name := range shipped {
		if !r.universe[name] {
			reason, _ := r.exclusionReason(name)
			t.Errorf("shipped class %q is not in the reduction: %s", name, reason)
		}
	}
	for name := range r.universe {
		if !slices.Contains(shipped, name) {
			t.Errorf("the pinned BMM defines %q but it is not shipped — unanswerable", name)
		}
	}
	if len(shipped) != len(r.universe) {
		t.Errorf("universe size: shipped %d, reduction %d", len(shipped), len(r.universe))
	}
}

// TestProbe094PerClassFactsEqualThePinnedBMM — arm (b). Abstractness is the
// BMM flag verbatim (REQ-047), parents are the BMM ancestors filtered to the
// universe in declaration order, and ancestors/descendants are the closures of
// that edge set, computed here rather than read from the table.
func TestProbe094PerClassFactsEqualThePinnedBMM(t *testing.T) {
	r := reducePinnedBMM(t)
	h := hier(t)

	for name := range r.universe {
		abstract, known := h.IsAbstract(name)
		if !known {
			t.Errorf("%s: IsAbstract reports unknown", name)
			continue
		}
		if abstract != r.abstract[name] {
			t.Errorf("%s: IsAbstract = %t, BMM is_abstract = %t", name, abstract, r.abstract[name])
		}

		parents, _ := h.Parents(name)
		if !slices.Equal(parents, r.parents[name]) {
			t.Errorf("%s: Parents = %v, reduction = %v", name, parents, r.parents[name])
		}

		wantAncestors := r.closureUp(name)
		gotAncestors, _ := h.Ancestors(name)
		if !slices.Equal(gotAncestors, wantAncestors) {
			t.Errorf("%s: Ancestors = %v, reduction = %v", name, gotAncestors, wantAncestors)
		}

		wantDescendants := r.concreteDescendants(name)
		gotDescendants, _ := h.ConcreteDescendants(name)
		if !slices.Equal(gotDescendants, wantDescendants) {
			t.Errorf("%s: ConcreteDescendants = %v, reduction = %v", name, gotDescendants, wantDescendants)
		}
	}
}

// closureUp is the reduction's own transitive ancestor closure, sorted, strict.
func (r *bmmReduction) closureUp(name string) []string {
	seen := map[string]bool{name: true}
	var out []string
	queue := slices.Clone(r.parents[name])
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		queue = append(queue, r.parents[n]...)
	}
	slices.Sort(out)
	return out
}

// concreteDescendants is the reduction's own expansion: itself when not
// BMM-abstract, plus every non-abstract class whose closure reaches it.
func (r *bmmReduction) concreteDescendants(name string) []string {
	var out []string
	for candidate := range r.universe {
		if r.abstract[candidate] {
			continue
		}
		if candidate == name || slices.Contains(r.closureUp(candidate), name) {
			out = append(out, candidate)
		}
	}
	slices.Sort(out)
	return out
}

// filterRoots are the classes the parent filter turns into roots: their ONLY
// BMM ancestor is a foundation type the RM target does not emit, so there is no
// in-universe edge left to keep. All four lose a foundation edge, none an RM
// information-model edge:
//
//   - PATHABLE        -> Any            (the universal foundation root)
//   - Point_interval  -> Interval       (a primitive_types generic)
//   - Proper_interval -> Interval
//   - Iso8601_timezone -> Iso8601_type  (a foundation temporal type)
var filterRoots = map[string]bool{
	"PATHABLE":         true,
	"Point_interval":   true,
	"Proper_interval":  true,
	"Iso8601_timezone": true,
}

// TestProbe094FilterCostsNoRMEdge — arm (c), the load-bearing half. Dropping a
// BMM ancestor outside the universe must not drop an RM edge: an ancestor lost
// here silently shrinks every descendant expansion above it.
func TestProbe094FilterCostsNoRMEdge(t *testing.T) {
	r := reducePinnedBMM(t)
	h := hier(t)

	dropped := map[string][]string{}
	for name := range r.universe {
		for _, anc := range r.bmmAncestors[name] {
			if !r.universe[anc] {
				dropped[anc] = append(dropped[anc], name)
			}
		}
	}
	if len(dropped) == 0 {
		t.Fatal("no ancestor was filtered — the closure arm would be vacuous, so the reduction is wrong")
	}
	for anc, children := range dropped {
		// Every dropped edge must be dropped for one of the reasons the
		// universe rule states. Without this, "no RM edge is lost" rests on
		// the pinned root set alone; with it, an RM information-model class
		// that ever fell out of the universe fails HERE, naming itself,
		// rather than surviving as a quietly shrunken expansion.
		reason, accounted := r.exclusionReason(anc)
		if !accounted {
			t.Errorf("ancestor %q (of %v) was dropped but %s — "+
				"if it is an RM class, every expansion above it just shrank", anc, children, reason)
		}
		t.Logf("dropped ancestor %-16s %-58s children=%v", anc, reason, children)
		for _, child := range children {
			// Every class that loses an ancestor must keep at least one,
			// unless it is one of the four whose ONLY BMM ancestor is a
			// foundation type the RM target does not emit. Pinning those
			// four means a fifth is a failure rather than a silent root —
			// and a fifth would mean an RM class had quietly lost its RM
			// parent, which is the loss this arm exists to catch.
			parents, _ := h.Parents(child)
			if len(parents) == 0 && !filterRoots[child] {
				t.Errorf("%s lost its only ancestor (%s) to the filter and is now an unexpected root", child, anc)
			}
			if slices.Contains(parents, anc) {
				t.Errorf("%s kept out-of-universe ancestor %s", child, anc)
			}
		}
	}
	// The pin must stay tight in the other direction too: a class listed
	// here that has gained a parent means the reason for its exemption is
	// gone and the list should shrink.
	for child := range filterRoots {
		if parents, known := h.Parents(child); !known {
			t.Errorf("filterRoots names %q, which is not a known class", child)
		} else if len(parents) > 0 {
			t.Errorf("filterRoots names %q but it now has parents %v — drop it from the pin", child, parents)
		}
	}
	// The class-graph answers never name a class outside the universe.
	for name := range r.universe {
		ancestors, _ := h.Ancestors(name)
		descendants, _ := h.ConcreteDescendants(name)
		for _, n := range slices.Concat(ancestors, descendants) {
			if !r.universe[n] {
				t.Errorf("%s: an answer names %q, which is outside the universe", name, n)
			}
		}
	}
}

// TestProbe094DeclarationSitesAreRealBMMDeclarations — arm (d). The site must
// be a class whose OWN BMM properties map carries the attribute; the class it
// is reported for must be the site or a descendant of it.
func TestProbe094DeclarationSitesAreRealBMMDeclarations(t *testing.T) {
	r := reducePinnedBMM(t)
	h := hier(t)
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement AttributeLister")
	}

	checked := 0
	for name := range r.universe {
		for _, attr := range lister.AttributeNames(name) {
			site, ok := h.DeclaredOn(name, attr)
			if !ok {
				t.Errorf("%s carries %q but reports no declaration site", name, attr)
				continue
			}
			cls := r.classOf[site]
			if cls == nil {
				t.Errorf("DeclaredOn(%s, %s) = %q, which the pinned BMM does not define", name, attr, site)
				continue
			}
			if !bmmDeclares(cls, attr) {
				t.Errorf("DeclaredOn(%s, %s) = %s, but %s's own BMM declaration does not carry %s",
					name, attr, site, site, attr)
				continue
			}
			// The site is the class itself, or a class it descends from —
			// including a filtered foundation class, which is why the
			// BMM ancestor closure is used here, not the shipped one.
			if site != name && !slices.Contains(r.bmmClosureUp(name), site) {
				t.Errorf("DeclaredOn(%s, %s) = %s, which %s does not descend from", name, attr, site, name)
			}
			// Container-ness is the one shape fact readable from the raw
			// BMM property without re-implementing REQ-043's type
			// mapping, so it is the one asserted here.
			gotContainer, _ := rminfo.Default.IsContainer(name, attr)
			if gotContainer != bmmIsContainer(cls, attr) {
				t.Errorf("%s.%s container = %t, but %s's BMM declaration says otherwise",
					name, attr, gotContainer, site)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no declaration site was checked — the arm is vacuous")
	}
	t.Logf("checked %d declaration sites across %d classes", checked, len(r.universe))
}

// TestProbe094AttributeSetsAreComplete — arm (d), the completeness half. Arm
// (d)'s site checks only look at attributes the table SHIPS; nothing held the
// shipped set to being the whole effective property set. That mattered as soon
// as REQ-048 removed the `len(attrs) == 0` class skip, which used to be the
// only signal that the generator had declined to translate every property of a
// class.
//
// The expectation is re-derived here by walking the unfiltered BMM ancestor
// chain and collecting property NAMES only — no type mapping, so this stays
// independent of REQ-043 rather than restating it.
func TestProbe094AttributeSetsAreComplete(t *testing.T) {
	r := reducePinnedBMM(t)
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not implement AttributeLister")
	}

	unshippedHits := map[string]bool{}
	checked := 0
	for name := range r.universe {
		want := r.effectivePropertyNames(name)
		slices.Sort(want)
		shipped := slices.Clone(lister.AttributeNames(name))
		slices.Sort(shipped)
		if !slices.Equal(shipped, want) {
			// Report the asymmetry rather than the two lists: a dropped
			// property and an invented one are different defects.
			for _, w := range want {
				if slices.Contains(shipped, w) {
					continue
				}
				key := name + "." + w
				if unshippedProperties[key] == "" {
					t.Errorf("%s is declared in the BMM chain but absent from the shipped table — "+
						"a silently untranslated property", key)
					continue
				}
				unshippedHits[key] = true
			}
			for _, g := range shipped {
				if !slices.Contains(want, g) {
					t.Errorf("%s.%s is shipped but no class in its BMM chain declares it", name, g)
				}
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no class was checked — the completeness arm is vacuous")
	}
	// A pin that no longer describes reality is worse than no pin: it would
	// keep excusing a drop that had actually been fixed. And the REQ-048 pin
	// rule binds every pin to an open research strand — held mechanically so
	// a future pin cannot excuse a drop with free text.
	strandRef := regexp.MustCompile(`STRAND-\d+`)
	for key, why := range unshippedProperties {
		if !strandRef.MatchString(why) {
			t.Errorf("unshippedProperties pins %s without citing a research strand: %q", key, why)
		}
		if !unshippedHits[key] {
			t.Errorf("unshippedProperties pins %s (%s) but it is shipped now — drop the pin", key, why)
		}
	}
}

// unshippedProperties are the BMM-declared attributes the shipped tables do NOT
// carry, each with why. Pinned rather than tolerated: this arm exists to make a
// silent drop loud, so every drop has to be named here first.
//
// The single entry is a PRE-EXISTING emission gap this arm surfaced, not
// something REQ-048 introduced. `Iso8601_type` is a primitive_types entry
// mapped to Go `string` (REQ-046, § Primitive type mapping), so the generator
// never plans it as a class and never folds the mandatory `value` it declares
// into its class_definitions descendant. openehr/rm agrees with the table
// rather than with the BMM — `rm.ISO8601Timezone` is emitted as an EMPTY
// struct, so the type cannot hold its own value either.
//
// Whether the generator should fold a primitive-mapped ancestor's properties —
// which would make rminfo disagree with the emitted Go struct instead of with
// the BMM — is a REQ-042/REQ-043 emission question, not this surface's, and it
// is open as STRAND-13. It MUST NOT be resolved here.
var unshippedProperties = map[string]string{
	"Iso8601_timezone.value": "inherited from the primitive-mapped Iso8601_type; see STRAND-13",
}

// effectivePropertyNames collects the property names a class carries, own and
// inherited, over the UNFILTERED BMM chain — the same set the generator folds,
// derived independently of it.
func (r *bmmReduction) effectivePropertyNames(name string) []string {
	var out []string
	seen := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string)
	visit = func(n string) {
		if visited[n] {
			return
		}
		visited[n] = true
		cls := r.classOf[n]
		if cls == nil {
			return
		}
		for _, anc := range cls.Ancestors() {
			visit(anc)
		}
		for prop := range bmmProperties(cls) {
			if !seen[prop] {
				seen[prop] = true
				out = append(out, prop)
			}
		}
	}
	visit(name)
	return out
}

// bmmClosureUp is the UNFILTERED BMM ancestor closure — it reaches the
// foundation classes the shipped graph drops, which is what a declaration site
// may legitimately name.
func (r *bmmReduction) bmmClosureUp(name string) []string {
	seen := map[string]bool{name: true}
	var out []string
	queue := slices.Clone(r.bmmAncestors[name])
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
		// classOf covers BOTH schema maps, so this subsumes bmmAncestors
		// (which is populated from the same call, for class_definitions
		// entries only) and additionally reaches a primitive_types site
		// such as Interval.
		if cls := r.classOf[n]; cls != nil {
			queue = append(queue, cls.Ancestors()...)
		}
	}
	return out
}

func bmmProperties(cls bmm.Class) map[string]bmm.Property {
	switch c := cls.(type) {
	case *bmm.SimpleClass:
		return c.Properties
	case *bmm.Interface:
		return c.Properties
	default:
		return nil
	}
}

func bmmDeclares(cls bmm.Class, attr string) bool {
	_, ok := bmmProperties(cls)[attr]
	return ok
}

func bmmIsContainer(cls bmm.Class, attr string) bool {
	_, ok := bmmProperties(cls)[attr].(*bmm.ContainerProperty)
	return ok
}

// TestProbe094NegativeSpace — arm (e). A name outside the universe is
// distinguishable from every in-universe answer, on every question.
func TestProbe094NegativeSpace(t *testing.T) {
	r := reducePinnedBMM(t)
	h := hier(t)

	// Names the pinned BMM defines but the universe excludes are the
	// sharpest probes: they exist upstream and must still be unknown here.
	outside := []string{"EXTRACT", "SYNC_EXTRACT", "Ordered", "TUPLE", "PROPORTION_KIND", "NEVER_AN_RM_CLASS"}
	for _, name := range outside {
		if r.universe[name] {
			t.Errorf("%q is in the reduction's universe — pick a different negative case", name)
			continue
		}
		if _, known := h.IsAbstract(name); known {
			t.Errorf("IsAbstract(%q): known=true for a class outside the universe", name)
		}
		if p, known := h.Parents(name); known || p != nil {
			t.Errorf("Parents(%q) = (%v, %t), want (nil, false)", name, p, known)
		}
		if a, known := h.Ancestors(name); known || a != nil {
			t.Errorf("Ancestors(%q) = (%v, %t), want (nil, false)", name, a, known)
		}
		if d, known := h.ConcreteDescendants(name); known || d != nil {
			t.Errorf("ConcreteDescendants(%q) = (%v, %t), want (nil, false)", name, d, known)
		}
		if _, known := h.ConformsTo(name, "LOCATABLE"); known {
			t.Errorf("ConformsTo(%q, LOCATABLE): known=true", name)
		}
		if _, known := h.ConformsTo("COMPOSITION", name); known {
			t.Errorf("ConformsTo(COMPOSITION, %q): known=true", name)
		}
		if s, ok := h.DeclaredOn(name, "name"); ok {
			t.Errorf("DeclaredOn(%q, name) = (%q, true), want not-found", name, s)
		}
	}

	// A root and a dead-end abstract class report as KNOWN with an empty
	// answer — the distinction the negative space turns on.
	roots := 0
	var deadEnds []string
	for name := range r.universe {
		if p, known := h.Parents(name); known && len(p) == 0 {
			roots++
			if a, known := h.Ancestors(name); !known || len(a) != 0 {
				t.Errorf("root %s: Ancestors = (%v, %t), want (empty, true)", name, a, known)
			}
		}
		abstract, _ := h.IsAbstract(name)
		if d, known := h.ConcreteDescendants(name); known && len(d) == 0 {
			deadEnds = append(deadEnds, name)
			if !abstract {
				t.Errorf("%s has no concrete descendant but is not abstract — a concrete class denotes itself", name)
			}
		}
	}
	if roots == 0 {
		t.Error("no root class found — the root arm is vacuous")
	}
	// Named, not counted: a different single dead-end class would keep the
	// count at three and go unnoticed.
	wantDeadEnds := []string{"ACCESS_CONTROL_SETTINGS", "AUTHORED_RESOURCE", "EXTERNAL_ENVIRONMENT_ACCESS"}
	slices.Sort(deadEnds)
	if !slices.Equal(deadEnds, wantDeadEnds) {
		t.Errorf("dead-end abstract classes = %v, want %v", deadEnds, wantDeadEnds)
	}
	t.Logf("%d roots, dead-end abstract classes %v", roots, deadEnds)
}
