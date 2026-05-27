package bmmgen

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// PlannedClass is a planned emission target: a single BMM class
// resolved to its enclosing package + computed Go identifier.
type PlannedClass struct {
	// BMMName is the original class name (e.g. "DV_QUANTITY").
	BMMName string
	// GoName is the Go identifier ("DVQuantity").
	GoName string
	// PackagePath is the BMM package dotted name (e.g.
	// "org.openehr.rm.data_types.quantity"). Empty for primitive types
	// — those are bucketed into a synthetic file (see [PlannedFile]).
	PackagePath string
	// FileBase is the basename (no _gen.go) the class is emitted to.
	FileBase string
	// Class is the loaded BMM class.
	Class bmm.Class
	// IsPrimitive reports whether the class originated in
	// primitive_types (vs class_definitions).
	IsPrimitive bool
	// External is true when this class is NOT owned by the plan's
	// target — it is referenced from another target (e.g. AOM
	// referencing a base/RM class). Renderers consult this flag to
	// decide whether to prefix the Go identifier with the target's
	// [Target.ExternalQualifier]. External classes are NEVER emitted
	// by the current plan; they only exist in [Plan.Classes] so the
	// renderer can resolve cross-target type references.
	External bool
}

// PlannedFile collects all classes assigned to a single generated
// _gen.go file. The order of Classes within a file is alphabetical
// by GoName.
type PlannedFile struct {
	// FileBase is the file stem (e.g. "data_types_quantity"). The
	// emitted path is "<out>/openehr/rm/<FileBase>_gen.go".
	FileBase string
	// PackagePath is the BMM package dotted name. For the synthetic
	// "foundation" bucket the package path is empty.
	PackagePath string
	// Classes is the set of planned classes that belong in this file.
	Classes []*PlannedClass
}

// Plan is the full emission plan for one BMM merge result.
type Plan struct {
	// Target identifies which generation destination this plan emits
	// to. Drives package name, output directory, and cross-package
	// reference qualification.
	Target Target
	// Schema is the merged BMM schema (root + transitive includes).
	Schema *bmm.Schema
	// Files is the ordered list of files to emit, sorted by FileBase.
	Files []*PlannedFile
	// CyclicSingleProps records (ownerBMMName, propertyName) pairs
	// whose SingleProperty must be emitted as a Go pointer to break
	// a class-graph cycle (e.g. ARCHETYPE.ontology <-> ARCHETYPE_ONTOLOGY.parent_archetype
	// in AOM 1.4). Without the pointer the Go compiler reports
	// "invalid recursive type". Computed in [PlanFromSchemaForTarget].
	CyclicSingleProps map[string]map[string]bool
	// Classes is a flat map by BMM class name, useful for
	// cross-package lookup during rendering. Includes BOTH owned and
	// external classes — the renderer consults [PlannedClass.External]
	// to decide whether a reference needs a package qualifier.
	Classes map[string]*PlannedClass
	// ConcreteClasses is the ordered list of concrete (non-abstract,
	// non-enum, non-interface) classes that get registered in
	// typereg. Sorted by BMMName for deterministic output.
	ConcreteClasses []*PlannedClass
	// AbstractDescendants maps each abstract class's BMM name to the
	// sorted list of concrete BMM-class descendants that need an
	// is<X>() marker method. Only OWNED descendants of OWNED abstracts
	// are listed here; cross-target marker emission is the consumer's
	// responsibility (none today).
	AbstractDescendants map[string][]string
	// ConcreteSubtypes maps each NON-abstract BMM class that has at
	// least one descendant to the sorted list of its descendant BMM
	// names. Drives the SDK-GAP-11 narrow-interface emission
	// (`<GoName>Like`): the openEHR RM permits Liskov substitution at
	// every concrete-typed slot, so a property declared as DV_TEXT may
	// admit DV_CODED_TEXT etc. The narrow Go interface lifts those
	// slots so canjson / canxml dispatch via typereg keeps subtype
	// payloads lossless through decode→re-marshal round-trips.
	ConcreteSubtypes map[string][]string
	// Notes collects human-readable warnings/skips encountered during
	// planning. The CLI prints them in verbose mode and the DONE
	// report should mention any non-empty entries.
	Notes []string
	// MethodStubsEmitted counts how many function stubs were emitted
	// across all classes (concrete + abstract-propagated). Diagnostic
	// only — populated by the renderer.
	MethodStubsEmitted int
	// MethodTodoEscapes counts how many method stubs ended up with a
	// fallback `any` return type because the function's BMM result
	// referenced a class that was skipped (FUNCTION, ROUTINE, etc.).
	// Diagnostic only — populated by the renderer.
	MethodTodoEscapes int
}

// BuildPlan loads the BMM root schema (and its transitive includes
// via the supplied resolver) and computes a [Plan]. Uses [TargetRM]
// as the implicit target — kept for backwards compatibility with
// Phase 2 tests. New callers should use [BuildPlanForTarget].
func BuildPlan(ctx context.Context, rootID string, resolver bmm.Resolver) (*Plan, error) {
	t := TargetRM
	if rootID != "" {
		t.RootID = rootID
	}
	return BuildPlanForTarget(ctx, t, resolver)
}

// BuildPlanForTarget loads the BMM root schema for the given target
// (via the supplied resolver) and computes a [Plan].
func BuildPlanForTarget(ctx context.Context, t Target, resolver bmm.Resolver) (*Plan, error) {
	_ = ctx // reserved; bmm.LoadAll does not yet thread context.
	schema, err := bmm.LoadAll(t.RootID, resolver)
	if err != nil {
		return nil, fmt.Errorf("bmmgen: load %q: %w", t.RootID, err)
	}
	return PlanFromSchemaForTarget(schema, t)
}

// PlanFromSchema computes a [Plan] from an already-loaded schema.
// Useful for tests that construct synthetic schemas in memory.
// Defaults to [TargetRM].
func PlanFromSchema(schema *bmm.Schema) (*Plan, error) {
	return PlanFromSchemaForTarget(schema, TargetRM)
}

// PlanFromSchemaForTarget computes a [Plan] from an already-loaded
// schema, using the supplied target.
func PlanFromSchemaForTarget(schema *bmm.Schema, t Target) (*Plan, error) {
	if schema == nil {
		return nil, fmt.Errorf("bmmgen: schema is nil")
	}
	p := &Plan{
		Target:              t,
		Schema:              schema,
		Classes:             make(map[string]*PlannedClass),
		AbstractDescendants: make(map[string][]string),
		ConcreteSubtypes:    make(map[string][]string),
		CyclicSingleProps:   make(map[string]map[string]bool),
	}

	// Walk the package tree to discover each class's enclosing package.
	// Sub-packages in the BMM JSON carry only their *leaf* name in
	// their `name` field, so we compose the dotted path during the
	// walk from the parent's path.
	//
	// A handful of classes (CODE_PHRASE most prominently) appear in
	// BOTH org.openehr.base.* and org.openehr.rm.*. The descendant
	// (rm) definition wins per the BMM loader's merge semantics, so
	// we walk rm packages first to record the rm location; base
	// packages are walked second and only add classes the rm walk
	// did not already place.
	classToPkg := map[string]string{}
	keys := sortedStringKeys(schema.Packages)
	// Sort: rm.* before base.*. Within each group, alphabetical.
	rmFirst := make([]string, 0, len(keys))
	rest := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.HasPrefix(k, "org.openehr.rm.") {
			rmFirst = append(rmFirst, k)
		} else {
			rest = append(rest, k)
		}
	}
	for _, k := range append(rmFirst, rest...) {
		root := schema.Packages[k]
		walkPackage(root, root.Name, classToPkg)
	}

	// Decide which classes survive the skip rules and bucket them by
	// file. Classes whose package is in the skip list are dropped.
	// Primitives go into a synthetic "foundation" bucket so types like
	// `Interval`, `Cardinality`, `Multiplicity_interval`, etc. are
	// emitted in a stable file.
	//
	// Primitives missing from primitiveGoType but present in
	// schema.PrimitiveTypes are emitted as full classes; primitives in
	// primitiveGoType (Boolean, String, ...) are NEVER emitted as
	// classes — they appear as Go primitives at use sites.

	files := map[string]*PlannedFile{}
	addToFile := func(filebase, pkgPath string, pc *PlannedClass) {
		f, ok := files[filebase]
		if !ok {
			f = &PlannedFile{FileBase: filebase, PackagePath: pkgPath}
			files[filebase] = f
		}
		f.Classes = append(f.Classes, pc)
	}

	// First: class_definitions.
	for _, name := range sortedStringKeys(schema.ClassDefinitions) {
		cls := schema.ClassDefinitions[name]
		if isSkippedClass(name) {
			p.Notes = append(p.Notes, fmt.Sprintf("skip class (skipped class set): %s", name))
			continue
		}
		pkgPath := classToPkg[name]
		if isSkippedPackage(pkgPath) {
			p.Notes = append(p.Notes, fmt.Sprintf("skip class %s (in skipped package %s)", name, pkgPath))
			continue
		}
		filebase := FileBase(pkgPath)
		if filebase == "" {
			// Class is not pinned to a package (rare). Bucket into
			// "foundation" so the emit stays deterministic.
			filebase = "foundation_misc"
		}
		external := !p.Target.owns(pkgPath)
		pc := &PlannedClass{
			BMMName:     name,
			GoName:      PascalCase(name),
			PackagePath: pkgPath,
			FileBase:    filebase,
			Class:       cls,
			External:    external,
		}
		p.Classes[name] = pc
		if !external {
			addToFile(filebase, pkgPath, pc)
		}
	}

	// Second: primitive_types. Skip those mapped to a Go primitive or
	// flagged as skipped.
	for _, name := range sortedStringKeys(schema.PrimitiveTypes) {
		cls := schema.PrimitiveTypes[name]
		if isPrimitive(name) {
			continue
		}
		if isSkippedPrimitive(name) {
			continue
		}
		if isSkippedClass(name) {
			continue
		}
		// Primitives without a package path go to a foundation bucket
		// matching where they conceptually live. We sort them into the
		// foundation_types_<sub> bucket by walking the base-schema
		// package tree to find a matching entry. If absent (rare), put
		// them in foundation_misc.
		pkgPath := classToPkg[name]
		filebase := FileBase(pkgPath)
		if filebase == "" {
			filebase = "foundation_misc"
		}
		external := !p.Target.owns(pkgPath)
		pc := &PlannedClass{
			BMMName:     name,
			GoName:      PascalCase(name),
			PackagePath: pkgPath,
			FileBase:    filebase,
			Class:       cls,
			IsPrimitive: true,
			External:    external,
		}
		p.Classes[name] = pc
		if !external {
			addToFile(filebase, pkgPath, pc)
		}
	}

	// Sort each file's classes deterministically.
	for _, f := range files {
		sort.Slice(f.Classes, func(i, j int) bool {
			return f.Classes[i].GoName < f.Classes[j].GoName
		})
	}
	for _, fb := range sortedFileKeys(files) {
		p.Files = append(p.Files, files[fb])
	}

	// Compute concrete class list (for typereg) and abstract
	// descendant closures (for is<X>() markers).
	for _, name := range sortedStringKeys(p.Classes) {
		pc := p.Classes[name]
		if pc.External {
			continue
		}
		if isConcreteForRegistry(pc) {
			p.ConcreteClasses = append(p.ConcreteClasses, pc)
		}
	}

	computeAbstractDescendants(p)
	computeConcreteSubtypes(p)
	computeCyclicSingleProps(p)
	return p, nil
}

// computeCyclicSingleProps detects directed cycles in the
// mandatory-SingleProperty graph among OWNED concrete classes and
// records which property edges need to be emitted as Go pointers to
// break the cycle. Without this, Go reports "invalid recursive type"
// — e.g. AOM's ARCHETYPE.ontology <-> ARCHETYPE_ONTOLOGY.parent_archetype.
//
// Algorithm (sufficient for the small RM/AOM class graphs):
//  1. Build a directed edge `A --(propName)--> B` for each mandatory
//     SingleProperty on owned concrete class A whose value type B
//     resolves to an owned concrete class.
//  2. For each edge (A, propName, B): if B has a directed path back
//     to A in the same graph, mark (A, propName) cyclic.
//
// Pointers are placed on every edge in the cycle. Idempotent under
// reordering since the marking is symmetric.
func computeCyclicSingleProps(p *Plan) {
	type edge struct {
		Owner, Prop, Target string
	}
	var edges []edge
	adj := map[string][]string{}

	addEdge := func(owner, prop, target string) {
		edges = append(edges, edge{owner, prop, target})
		adj[owner] = append(adj[owner], target)
	}

	isOwnedConcreteStruct := func(name string) bool {
		pc, ok := p.Classes[name]
		if !ok || pc.External {
			return false
		}
		sc, isSimple := pc.Class.(*bmm.SimpleClass)
		if !isSimple {
			return false
		}
		// Abstract non-generic classes are interfaces — already nilable.
		if sc.IsAbstract() && !sc.IsGeneric() {
			return false
		}
		return true
	}

	for _, pc := range p.Classes {
		if pc.External {
			continue
		}
		sc, isSimple := pc.Class.(*bmm.SimpleClass)
		if !isSimple {
			continue
		}
		// Include abstract+generic structs in the cycle graph since they
		// are rendered as structs and can carry concrete-class refs.
		if sc.IsAbstract() && !sc.IsGeneric() {
			continue
		}
		for _, propName := range sortedStringKeys(sc.Properties) {
			prop := sc.Properties[propName]
			sp, ok := prop.(*bmm.SingleProperty)
			if !ok {
				continue
			}
			if !sp.IsMandatory {
				// Already a pointer in the rendered output.
				continue
			}
			if !isOwnedConcreteStruct(sp.TypeName) {
				continue
			}
			addEdge(pc.BMMName, propName, sp.TypeName)
		}
	}

	// Reachability: for each edge (A, _, B), is A reachable from B?
	pathExists := func(start, target string) bool {
		visited := map[string]bool{}
		queue := []string{start}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if visited[n] {
				continue
			}
			visited[n] = true
			if n == target {
				return true
			}
			queue = append(queue, adj[n]...)
		}
		return false
	}

	for _, e := range edges {
		if pathExists(e.Target, e.Owner) {
			if _, ok := p.CyclicSingleProps[e.Owner]; !ok {
				p.CyclicSingleProps[e.Owner] = make(map[string]bool)
			}
			p.CyclicSingleProps[e.Owner][e.Prop] = true
		}
	}
}

// walkPackage records the enclosing package for each class in p
// (including sub-packages). The dotted path is composed from the
// parent path because sub-packages in the BMM only carry their leaf
// name in their `name` field.
func walkPackage(p *bmm.Package, path string, into map[string]string) {
	if p == nil {
		return
	}
	for _, c := range p.Classes {
		// First wins — a class can appear in only one package per
		// schema. (If the merge process dropped a class onto two
		// packages, mark the first; downstream code does not rely on
		// the duplicate.)
		if _, exists := into[c]; !exists {
			into[c] = path
		}
	}
	for _, key := range sortedStringKeys(p.Packages) {
		sub := p.Packages[key]
		childPath := path + "." + sub.Name
		walkPackage(sub, childPath, into)
	}
}

// registryGenericConcrete reports whether a generic concrete class
// should be added to the type registry under its bare BMM name
// (e.g. "DV_INTERVAL" → &DVInterval[DVOrdered]{}). Two cases qualify:
//
//  1. Descendants of a codec-polymorphic abstract generic ancestor
//     (e.g. POINT_EVENT / INTERVAL_EVENT under EVENT — ADR 0003).
//
//  2. Top-level concrete generics whose `xsi:type` / `_type`
//     discriminator appears on the wire under a polymorphic parent
//     slot. Real-world canonical-XML fixtures emit
//     `xsi:type="DV_INTERVAL"` at DataValue slots, so the registry
//     must have a constructor under that bare name. The constructor
//     produces the default-instantiated form using each parameter's
//     `Any` / abstract bound.
//
// Case (2) is necessary for canxml.DecodeAs[DataValue] to resolve
// `DV_INTERVAL`. The resulting concrete value satisfies the target
// abstract interface (DVInterval[DVOrdered] implements DataValue).
func registryGenericConcrete(pc *PlannedClass) bool {
	for _, anc := range pc.Class.Ancestors() {
		if codecPolymorphicAbstractGenericNames[anc] {
			return true
		}
	}
	// Top-level concrete generics — DV_INTERVAL, REFERENCE_RANGE, etc.
	// — also need to be registered so polymorphic dispatch via
	// xsi:type / _type resolves them. The default-bound
	// instantiation is what the generator emits for cross-target
	// references (see defaultGenericArgs), and it is what the
	// concrete struct gets registered under in typereg.
	if sc, ok := pc.Class.(*bmm.SimpleClass); ok && sc.IsGeneric() && !sc.IsAbstract() {
		return true
	}
	return false
}

// isConcreteForRegistry reports whether the planned class should get
// a typereg.Register(...) call: must be a SimpleClass (not Interface,
// not Enumeration), not abstract, and not a primitive whose Go form
// is a primitive alias.
func isConcreteForRegistry(pc *PlannedClass) bool {
	if pc == nil {
		return false
	}
	if pc.IsPrimitive {
		// primitive classes (Cardinality, Interval, etc.) are emitted
		// as structs but they do NOT have a stable RM _type
		// discriminator in the wire format — skip them.
		return false
	}
	if sc, ok := pc.Class.(*bmm.SimpleClass); ok {
		if sc.IsAbstract() {
			return false
		}
		if sc.IsGeneric() {
			return registryGenericConcrete(pc)
		}
		return true
	}
	// Interface and Enumeration are never directly registered.
	return false
}

// computeAbstractDescendants populates p.AbstractDescendants. For
// each abstract class A, the value is the sorted list of concrete
// BMM class names that transitively descend from A.
func computeAbstractDescendants(p *Plan) {
	// Build ancestor->children adjacency over the planned classes.
	children := map[string][]string{}
	for _, pc := range p.Classes {
		for _, anc := range pc.Class.Ancestors() {
			children[anc] = append(children[anc], pc.BMMName)
		}
	}
	for _, list := range children {
		sort.Strings(list)
	}

	// For each abstract class, BFS for concrete descendants.
	for _, pc := range p.Classes {
		if pc.External {
			// Marker methods are emitted alongside the abstract class
			// definition. If the abstract class is external (defined in
			// another target), we don't emit anything here.
			continue
		}
		if !pc.Class.IsAbstract() {
			continue
		}
		// Only SimpleClass + Interface get is<X>() markers — and only
		// SimpleClass concrete descendants implement them. (Enums are
		// abstract leaves; nothing descends from them in any practical
		// sense.)
		switch pc.Class.(type) {
		case *bmm.SimpleClass, *bmm.Interface:
			// fall through
		default:
			continue
		}
		seen := map[string]bool{}
		queue := append([]string{}, children[pc.BMMName]...)
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			if seen[next] {
				continue
			}
			seen[next] = true
			child, ok := p.Classes[next]
			if !ok {
				continue
			}
			if child.External {
				// Don't generate marker method on an external descendant —
				// that descendant lives in another package and the
				// abstract is owned by us, but Go doesn't permit defining
				// a method on a type from another package. (Not expected
				// in v1: AOM uses RM ancestors, not vice versa.)
				continue
			}
			if !child.Class.IsAbstract() {
				// Only concrete SimpleClass descendants emit a marker
				// method; enums and interfaces do not.
				if _, isSimple := child.Class.(*bmm.SimpleClass); isSimple {
					p.AbstractDescendants[pc.BMMName] = append(p.AbstractDescendants[pc.BMMName], next)
				}
			}
			queue = append(queue, children[next]...)
		}
		sort.Strings(p.AbstractDescendants[pc.BMMName])
	}
}

// collectReferencedPropertyTypes returns the set of BMM class names
// that appear as the declared type of at least one property anywhere
// in the plan. Includes both single-property type names and the
// element types of container / generic properties (each level of
// nesting unwrapped). Drives the SDK-GAP-11 narrow-interface filter.
func collectReferencedPropertyTypes(p *Plan) map[string]bool {
	out := map[string]bool{}
	addContainer := func(td *bmm.ContainerType) {
		if td == nil || td.TypeDef == nil {
			return
		}
		switch inner := td.TypeDef.(type) {
		case *bmm.SimpleType:
			out[inner.TypeName] = true
		case *bmm.GenericType:
			out[inner.RootType] = true
		}
	}
	for _, pc := range p.Classes {
		sc, isSimple := pc.Class.(*bmm.SimpleClass)
		if !isSimple {
			continue
		}
		for _, prop := range sc.Properties {
			switch pp := prop.(type) {
			case *bmm.SingleProperty:
				out[pp.TypeName] = true
			case *bmm.ContainerProperty:
				addContainer(pp.TypeDef)
			case *bmm.GenericProperty:
				if pp.TypeDef != nil {
					out[pp.TypeDef.RootType] = true
				}
			}
		}
	}
	return out
}

// computeConcreteSubtypes populates p.ConcreteSubtypes. Mirror of
// computeAbstractDescendants but keyed on NON-abstract SimpleClasses
// that (a) have at least one concrete descendant AND (b) are actually
// referenced as a property type somewhere in the schema — the
// SDK-GAP-11 narrow-interface driver. Each entry maps the parent's
// BMM name to the sorted list of all transitive concrete SimpleClass
// descendants.
//
// The "referenced as a property type" filter is what keeps the
// narrow-interface emission focused: openEHR's BMM has a deep
// bookkeeping hierarchy (BASIC_DEFINITIONS, OPENEHR_DEFINITIONS, …)
// whose descendants span the entire data-types package, but those
// roots are never the declared type of any property — they would
// produce huge, useless `*Like` interfaces.
func computeConcreteSubtypes(p *Plan) {
	children := map[string][]string{}
	for _, pc := range p.Classes {
		for _, anc := range pc.Class.Ancestors() {
			children[anc] = append(children[anc], pc.BMMName)
		}
	}
	for _, list := range children {
		sort.Strings(list)
	}
	// Set of class names that appear as the declared type of at least
	// one property somewhere in the plan.
	referencedTypes := collectReferencedPropertyTypes(p)
	for _, pc := range p.Classes {
		if !isConcreteForRegistry(pc) {
			continue
		}
		sc, _ := pc.Class.(*bmm.SimpleClass)
		if pc.External || sc.IsGeneric() {
			continue
		}
		if !referencedTypes[pc.BMMName] {
			// Parent class is never used as a property type — no slot
			// can carry a substituted subtype, so no narrow interface
			// is needed.
			continue
		}
		if _, hasKids := children[pc.BMMName]; !hasKids {
			continue
		}
		seen := map[string]bool{}
		queue := append([]string{}, children[pc.BMMName]...)
		for len(queue) > 0 {
			next := queue[0]
			queue = queue[1:]
			if seen[next] {
				continue
			}
			seen[next] = true
			child, ok := p.Classes[next]
			if !ok {
				continue
			}
			if child.External {
				continue
			}
			// Only concrete SimpleClass descendants take a marker
			// method — abstract intermediates stay interface-only, and
			// enums / P_BMM_INTERFACE leaves cannot receive value
			// receivers.
			cc, isSimple := child.Class.(*bmm.SimpleClass)
			if isSimple && !cc.IsAbstract() {
				p.ConcreteSubtypes[pc.BMMName] = append(p.ConcreteSubtypes[pc.BMMName], next)
			}
			queue = append(queue, children[next]...)
		}
		sort.Strings(p.ConcreteSubtypes[pc.BMMName])
		// If no concrete descendants survived the filter, drop the
		// parent so emitNarrowInterface skips it.
		if len(p.ConcreteSubtypes[pc.BMMName]) == 0 {
			delete(p.ConcreteSubtypes, pc.BMMName)
		}
	}
}

// sortedStringKeys returns the keys of m in lexicographic order.
func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFileKeys(files map[string]*PlannedFile) []string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AncestorChain returns the BMM ancestor chain (immediate + their
// own ancestors) for a class. Used for computing which abstract
// classes a concrete class descends from.
func AncestorChain(p *Plan, name string) []string {
	seen := map[string]bool{}
	var out []string
	var rec func(string)
	rec = func(n string) {
		c, ok := p.Classes[n]
		if !ok {
			return
		}
		for _, anc := range c.Class.Ancestors() {
			if seen[anc] {
				continue
			}
			seen[anc] = true
			out = append(out, anc)
			rec(anc)
		}
	}
	rec(name)
	return out
}

// Diagnostic returns a human-readable summary of the plan, useful
// for `-v` mode in cmd/bmmgen.
func (p *Plan) Diagnostic() string {
	var b strings.Builder
	fmt.Fprintf(&b, "plan: %d files, %d classes, %d concrete\n", len(p.Files), len(p.Classes), len(p.ConcreteClasses))
	for _, f := range p.Files {
		fmt.Fprintf(&b, "  %s_gen.go (%d classes) [%s]\n", f.FileBase, len(f.Classes), f.PackagePath)
	}
	if len(p.Notes) > 0 {
		b.WriteString("notes:\n")
		for _, n := range p.Notes {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	return b.String()
}
