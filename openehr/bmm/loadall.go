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
//     when the includes of one schema bring *differing* definitions of
//     a name: the definition whose declaring schemas include, directly
//     or transitively, every schema that declares another one wins,
//     whatever the order of the includes. A schema that reaches
//     openehr_base_1.3.0 both through openehr_am_1.4.0 and through
//     openehr_rm_1.2.0 therefore takes RM's TRANSLATION_DETAILS; two
//     definitions from schemas neither of which includes the other give
//     way to that of a third schema that includes both and declares the
//     name again. A definition is declared by the schema whose own
//     class_definitions or primitive_types carry it, not by a schema
//     that passes it on from an include. When no definition's declaring
//     schemas include every schema that declares another one, that's an
//     ErrSchemaConflict; there is no winner. An *identical* definition
//     reached via two include paths (a diamond, e.g. am24 includes both
//     base and lang, and lang also includes base) is benign and merged
//     once.
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

	merged := &mergedSchema{
		schema:       root,
		ancestors:    map[string]struct{}{},
		classSources: ownSources(root.ClassDefinitions, id),
		primSources:  ownSources(root.PrimitiveTypes, id),
	}

	// Merge ancestors first (depth-first), in deterministic key order
	// so error messages are reproducible.
	includes := make([]*mergedSchema, 0, len(root.Includes))
	for _, inclID := range sortedKeys(root.Includes) {
		ancestor, err := st.load(ctx, inclID)
		if err != nil {
			return nil, err
		}
		merged.ancestors[inclID] = struct{}{}
		maps.Copy(merged.ancestors, ancestor.ancestors)
		includes = append(includes, ancestor)
	}
	// Each name is settled over every form the includes bring at once, so
	// the include order cannot change which form wins.
	root.PrimitiveTypes, err = st.mergeForms(root.PrimitiveTypes, merged.primSources, includes, primitiveForms)
	if err != nil {
		return nil, err
	}
	root.ClassDefinitions, err = st.mergeForms(root.ClassDefinitions, merged.classSources, includes, classForms)
	if err != nil {
		return nil, err
	}
	for _, ancestor := range includes {
		mergePackages(root, ancestor.schema)
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

// formsOf picks one kind of named entry (classes or primitive types) of a
// merged schema, with the sources of each.
type formsOf func(*mergedSchema) (map[string]Class, map[string][]string)

func primitiveForms(m *mergedSchema) (map[string]Class, map[string][]string) {
	return m.schema.PrimitiveTypes, m.primSources
}

func classForms(m *mergedSchema) (map[string]Class, map[string][]string) {
	return m.schema.ClassDefinitions, m.classSources
}

// candidate is one form of a name the includes bring, with every schema
// whose own declaration supplied it.
type candidate struct {
	form    Class
	sources []string
}

// mergeForms folds one kind of named entry from the includes into dst,
// which holds the descendant's own entries, keeping dstSources in step,
// and returns dst, made when it was nil and gains an entry. A name in dst
// is the descendant's and is kept. Every other name is settled over all
// the forms the includes bring for it:
//
//   - identical forms are one candidate, whose sources are those of each;
//   - a candidate refines another when every one of the other's sources is
//     included, directly or transitively, by one of its own;
//   - the one candidate no other refines refines every other and wins;
//   - when two or more stand, there is no winner, and the result is a
//     schemaConflictError naming a source of each of the first two
//     neither of which includes the other.
func (st *loadAllState) mergeForms(dst map[string]Class, dstSources map[string][]string, includes []*mergedSchema, kind formsOf) (map[string]Class, error) {
	names := map[string]struct{}{}
	for _, inc := range includes {
		forms, _ := kind(inc)
		for name := range forms {
			if _, own := dst[name]; own {
				continue // descendant shadow — allowed
			}
			names[name] = struct{}{}
		}
	}
	// Sorted, so that of several conflicts the same one is reported.
	for _, name := range sortedKeys(names) {
		var candidates []candidate
		for _, inc := range includes {
			forms, sources := kind(inc)
			form, ok := forms[name]
			if !ok {
				continue
			}
			i := slices.IndexFunc(candidates, func(c candidate) bool { return reflect.DeepEqual(c.form, form) })
			if i < 0 {
				candidates = append(candidates, candidate{form: form, sources: sources[name]})
				continue
			}
			// Identical definition reached via two include paths (diamond) — benign.
			candidates[i].sources = addSources(candidates[i].sources, sources[name])
		}
		standing := st.standing(candidates)
		if len(standing) > 1 {
			a, b := candidates[standing[0]], candidates[standing[1]]
			onlyA, _ := st.unshadowed(b.sources, a.sources)
			onlyB, _ := st.unshadowed(a.sources, b.sources)
			return nil, &schemaConflictError{ClassName: name, SchemaA: onlyA, SchemaB: onlyB}
		}
		if dst == nil {
			dst = make(map[string]Class, len(names))
		}
		winner := candidates[standing[0]]
		dst[name] = winner.form
		dstSources[name] = winner.sources
	}
	return dst, nil
}

// standing returns the indexes of the candidates no other candidate
// refines, in order. Since a schema never includes itself, directly or
// transitively, refining is a strict order between candidates, so of a
// non-empty list at least one stands, and when only one does it refines
// every other.
func (st *loadAllState) standing(candidates []candidate) []int {
	var out []int
	for i, c := range candidates {
		refined := false
		for j, other := range candidates {
			if j == i {
				continue
			}
			if _, stands := st.unshadowed(other.sources, c.sources); !stands {
				refined = true
				break
			}
		}
		if !refined {
			out = append(out, i)
		}
	}
	return out
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

// mergePackages folds the ancestor schema's packages into dst (the
// descendant) by name, recursively. Mutates dst.
func mergePackages(dst, ancestor *Schema) {
	if len(ancestor.Packages) == 0 {
		return
	}
	if dst.Packages == nil {
		dst.Packages = make(map[string]*Package, len(ancestor.Packages))
	}
	for k, v := range ancestor.Packages {
		existing, ok := dst.Packages[k]
		if !ok {
			dst.Packages[k] = clonePackage(v)
			continue
		}
		mergePackage(existing, v)
	}
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
