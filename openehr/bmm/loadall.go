package bmm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
)

// LoadAll resolves a BMM schema and its transitive includes via the
// supplied [Resolver], then returns a single merged [Schema].
//
// Merge contract:
//
//   - The schema identified by rootID is the *descendant* schema.
//     Any included schemas are walked depth-first; each include is
//     itself fully merged with its own ancestors before contributing
//     to the result.
//   - ClassDefinitions and PrimitiveTypes: the descendant's entries
//     take precedence over identically-named entries contributed by
//     the ancestor. (This reflects observed overlap in the real
//     openEHR BMM corpus, e.g. TRANSLATION_DETAILS appears in both
//     openehr_base_1.3.0 and openehr_rm_1.2.0, with the RM definition
//     being the authoritative refinement.) The same precedence holds
//     when two includes of one schema bring *differing* definitions of
//     a name: the definition declared by a schema that includes,
//     directly or transitively, the schema declaring the other one
//     wins. A schema that reaches openehr_base_1.3.0 both through
//     openehr_am_1.4.0 and through openehr_rm_1.2.0 therefore takes
//     RM's TRANSLATION_DETAILS. A definition is declared by the schema
//     whose own class_definitions or primitive_types carry it, not by a
//     schema that passes it on from an include. When neither declaring
//     schema includes the other, that's an ErrSchemaConflict; there is
//     no winner. An *identical* definition reached via two include
//     paths (a diamond, e.g. am24 includes both base and lang, and lang
//     also includes base) is benign and merged once.
//   - Packages are merged by fully-qualified package name (the map key
//     plus the recursive Name field). When two schemas both define the
//     same package node, their class-name lists are concatenated
//     (deduped) and sub-packages are merged recursively.
//   - The result's top-level metadata fields (BMMVersion, RMPublisher,
//     SchemaName, RMRelease, SchemaRevision, SchemaLifecycleState,
//     SchemaDescription, SchemaAuthor, Includes) are copied from the
//     root schema verbatim.
//
// Cycle detection: LoadAll keeps a visited set keyed by schema id and
// returns ErrCircularIncludes (wrapping the offending id and chain)
// on detection. Re-entering the same schema via two different paths
// (a DAG, not a cycle) is allowed; the second visit short-circuits and
// reuses the prior result.
func LoadAll(rootID string, resolver Resolver) (*Schema, error) {
	if resolver == nil {
		return nil, errors.New("bmm.LoadAll: resolver is nil")
	}
	if rootID == "" {
		return nil, errors.New("bmm.LoadAll: rootID is empty")
	}
	state := &loadAllState{
		resolver: resolver,
		visited:  map[string]*mergedSchema{},
		stack:    nil,
	}
	merged, err := state.load(context.Background(), rootID)
	if err != nil {
		return nil, err
	}
	return merged.schema, nil
}

type loadAllState struct {
	resolver Resolver
	visited  map[string]*mergedSchema
	stack    []string // current path for cycle diagnostics
}

// mergedSchema is one schema merged with all of its includes, plus what
// a later merge needs to settle two differing forms of one name.
type mergedSchema struct {
	schema *Schema
	// ancestors holds the id of every schema this one includes, directly
	// or transitively.
	ancestors map[string]struct{}
	// classSources and primSources map each class and primitive type name
	// to the schemas whose own declaration supplied the merged form: one
	// schema, or several that declare the same form. A schema that
	// redeclares a name it also reaches through an include is that name's
	// only source.
	classSources map[string][]string
	primSources  map[string][]string
}

func (st *loadAllState) load(ctx context.Context, id string) (*mergedSchema, error) {
	// Cycle detection: id already in stack?
	if slices.Contains(st.stack, id) {
		chain := append([]string{}, st.stack...)
		chain = append(chain, id)
		return nil, &circularIncludesError{SchemaID: id, Chain: chain}
	}
	// Already merged via another path?
	if cached, ok := st.visited[id]; ok {
		return cached, nil
	}
	rc, err := st.resolver.Resolve(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("bmm.LoadAll: resolve %q: %w", id, err)
	}
	root, err := Load(rc)
	if closeErr := rc.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, fmt.Errorf("bmm.LoadAll: load %q: %w", id, err)
	}

	st.stack = append(st.stack, id)
	defer func() { st.stack = st.stack[:len(st.stack)-1] }()

	// Snapshot the descendant's own class names before any ancestor
	// merge — needed by mergeAncestor to distinguish "descendant
	// shadows ancestor" (allowed) from "ancestor vs ancestor"
	// (settled by mergeForms).
	descendantClasses := make(map[string]struct{}, len(root.ClassDefinitions))
	for k := range root.ClassDefinitions {
		descendantClasses[k] = struct{}{}
	}
	descendantPrims := make(map[string]struct{}, len(root.PrimitiveTypes))
	for k := range root.PrimitiveTypes {
		descendantPrims[k] = struct{}{}
	}
	merged := &mergedSchema{
		schema:       root,
		ancestors:    map[string]struct{}{},
		classSources: ownSources(root.ClassDefinitions, id),
		primSources:  ownSources(root.PrimitiveTypes, id),
	}

	// Merge ancestors first (depth-first), in deterministic key order
	// so error messages are reproducible.
	for _, inclID := range sortedKeys(root.Includes) {
		ancestor, err := st.load(ctx, inclID)
		if err != nil {
			return nil, err
		}
		merged.ancestors[inclID] = struct{}{}
		maps.Copy(merged.ancestors, ancestor.ancestors)
		if err := st.mergeAncestor(merged, ancestor, descendantClasses, descendantPrims); err != nil {
			return nil, err
		}
	}
	st.visited[id] = merged
	return merged, nil
}

// ownSources records id as the source of every name the schema declares
// itself. The entries share one slice, which is never appended to in
// place.
func ownSources(own map[string]Class, id string) map[string][]string {
	sources := make(map[string][]string, len(own))
	self := []string{id}
	for k := range own {
		sources[k] = self
	}
	return sources
}

// mergeAncestor folds the ancestor schema's primitive types, class
// definitions, and packages into dst (the descendant). Mutates dst.
// Names the descendant declares itself (descendantClasses,
// descendantPrims) shadow any ancestor contribution — that's the
// documented merge precedence, not a conflict. A name two includes both
// contribute is settled by mergeForms.
func (st *loadAllState) mergeAncestor(dst, ancestor *mergedSchema, descendantClasses, descendantPrims map[string]struct{}) error {
	d, a := dst.schema, ancestor.schema
	if len(a.PrimitiveTypes) > 0 {
		if d.PrimitiveTypes == nil {
			d.PrimitiveTypes = make(map[string]Class, len(a.PrimitiveTypes))
		}
		if err := st.mergeForms(d.PrimitiveTypes, dst.primSources, a.PrimitiveTypes, ancestor.primSources, descendantPrims); err != nil {
			return err
		}
	}
	if len(a.ClassDefinitions) > 0 {
		if d.ClassDefinitions == nil {
			d.ClassDefinitions = make(map[string]Class, len(a.ClassDefinitions))
		}
		if err := st.mergeForms(d.ClassDefinitions, dst.classSources, a.ClassDefinitions, ancestor.classSources, descendantClasses); err != nil {
			return err
		}
	}
	// Packages: merge by name, recursively.
	if len(a.Packages) > 0 {
		if d.Packages == nil {
			d.Packages = make(map[string]*Package, len(a.Packages))
		}
		for k, v := range a.Packages {
			existing, ok := d.Packages[k]
			if !ok {
				d.Packages[k] = clonePackage(v)
				continue
			}
			mergePackage(existing, v)
		}
	}
	return nil
}

// mergeForms folds one kind of named entry (classes or primitive types)
// from an ancestor into dst, keeping dstSources in step. A name in own
// is the descendant's and is kept. A name dst does not hold yet is
// copied. A name dst already holds through an earlier include keeps one
// form:
//
//   - an identical form is merged once and gains the ancestor's sources;
//   - a differing form loses to the other when every one of its sources
//     is included, directly or transitively, by one of the other form's
//     sources: that schema refines the class it reached;
//   - otherwise there is no winner, and the result is a
//     schemaConflictError naming two sources neither of which includes
//     the other.
func (st *loadAllState) mergeForms(dst map[string]Class, dstSources map[string][]string, src map[string]Class, srcSources map[string][]string, own map[string]struct{}) error {
	// Sorted, so that of several conflicts the same one is reported.
	for _, name := range sortedKeys(src) {
		form := src[name]
		if _, inDescendant := own[name]; inDescendant {
			continue // descendant shadow — allowed
		}
		existing, exists := dst[name]
		if !exists {
			dst[name] = form
			dstSources[name] = srcSources[name]
			continue
		}
		have, incoming := dstSources[name], srcSources[name]
		if reflect.DeepEqual(existing, form) {
			// Identical definition reached via two include paths (diamond) — benign.
			dstSources[name] = addSources(have, incoming)
			continue
		}
		incomingOnly, incomingStands := st.unshadowed(have, incoming)
		if !incomingStands {
			continue // the form already merged refines the incoming one
		}
		haveOnly, haveStands := st.unshadowed(incoming, have)
		if !haveStands {
			dst[name] = form // the incoming form refines the one already merged
			dstSources[name] = incoming
			continue
		}
		return &schemaConflictError{
			ClassName: name,
			SchemaA:   haveOnly,
			SchemaB:   incomingOnly,
		}
	}
	return nil
}

// unshadowed returns the first schema in sources that no schema in by
// includes, directly or transitively, and false when every one is.
func (st *loadAllState) unshadowed(by, sources []string) (string, bool) {
	for _, s := range sources {
		if !slices.ContainsFunc(by, func(b string) bool { return st.includes(b, s) }) {
			return s, true
		}
	}
	return "", false
}

// includes reports whether schema a includes schema b, directly or
// transitively. Every source a merge compares is an include of the
// schema being merged, or one of its ancestors, so it is in visited.
func (st *loadAllState) includes(a, b string) bool {
	_, ok := st.visited[a].ancestors[b]
	return ok
}

// addSources returns have with every source of incoming it lacks
// appended, without writing into have's backing array, which other
// entries may share.
func addSources(have, incoming []string) []string {
	for _, s := range incoming {
		if !slices.Contains(have, s) {
			have = append(slices.Clip(have), s)
		}
	}
	return have
}

func mergePackage(dst, src *Package) {
	// Concatenate class lists, dedup.
	if len(src.Classes) > 0 {
		seen := make(map[string]struct{}, len(dst.Classes)+len(src.Classes))
		for _, c := range dst.Classes {
			seen[c] = struct{}{}
		}
		for _, c := range src.Classes {
			if _, exists := seen[c]; !exists {
				dst.Classes = append(dst.Classes, c)
				seen[c] = struct{}{}
			}
		}
	}
	if len(src.Packages) > 0 {
		if dst.Packages == nil {
			dst.Packages = make(map[string]*Package, len(src.Packages))
		}
		for k, v := range src.Packages {
			existing, ok := dst.Packages[k]
			if !ok {
				dst.Packages[k] = clonePackage(v)
				continue
			}
			mergePackage(existing, v)
		}
	}
}

func clonePackage(p *Package) *Package {
	if p == nil {
		return nil
	}
	cp := &Package{
		Name:    p.Name,
		Classes: append([]string(nil), p.Classes...),
	}
	if len(p.Packages) > 0 {
		cp.Packages = make(map[string]*Package, len(p.Packages))
		for k, v := range p.Packages {
			cp.Packages[k] = clonePackage(v)
		}
	}
	return cp
}
