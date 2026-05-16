package bmm

import (
	"context"
	"fmt"
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
//     openEHR BMM corpus — e.g. TRANSLATION_DETAILS appears in both
//     openehr_base_1.3.0 and openehr_rm_1.2.0, with the RM definition
//     being the authoritative refinement.) When two *sibling* ancestor
//     schemas (i.e. two distinct includes of the same descendant) both
//     declare the same class, that's an ErrSchemaConflict — there is
//     no winner.
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
		return nil, fmt.Errorf("bmm.LoadAll: resolver is nil")
	}
	if rootID == "" {
		return nil, fmt.Errorf("bmm.LoadAll: rootID is empty")
	}
	state := &loadAllState{
		resolver: resolver,
		visited:  map[string]*Schema{},
		stack:    nil,
	}
	return state.load(context.Background(), rootID)
}

type loadAllState struct {
	resolver Resolver
	visited  map[string]*Schema
	stack    []string // current path for cycle diagnostics
}

func (st *loadAllState) load(ctx context.Context, id string) (*Schema, error) {
	// Cycle detection: id already in stack?
	for _, s := range st.stack {
		if s == id {
			chain := append([]string{}, st.stack...)
			chain = append(chain, id)
			return nil, &circularIncludesError{SchemaID: id, Chain: chain}
		}
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
	// (conflict).
	descendantClasses := make(map[string]struct{}, len(root.ClassDefinitions))
	for k := range root.ClassDefinitions {
		descendantClasses[k] = struct{}{}
	}
	descendantPrims := make(map[string]struct{}, len(root.PrimitiveTypes))
	for k := range root.PrimitiveTypes {
		descendantPrims[k] = struct{}{}
	}

	// Merge ancestors first (depth-first), in deterministic key order
	// so error messages are reproducible.
	for _, inclID := range sortedKeys(root.Includes) {
		ancestor, err := st.load(ctx, inclID)
		if err != nil {
			return nil, err
		}
		if err := mergeAncestor(root, ancestor, descendantClasses, descendantPrims); err != nil {
			return nil, err
		}
	}
	st.visited[id] = root
	return root, nil
}

// mergeAncestor folds the ancestor schema's primitive types, class
// definitions, and packages into dst (the descendant). Mutates dst.
// Returns ErrSchemaConflict when two sibling ancestors both declare
// the same class (after which there is no clear winner). Classes that
// already exist in the descendant's own snapshot (descendantClasses)
// shadow any ancestor contribution — that's the documented merge
// precedence, not a conflict.
func mergeAncestor(dst, ancestor *Schema, descendantClasses, descendantPrims map[string]struct{}) error {
	// Primitives: descendant precedence; only add ancestor entries
	// that dst does not already declare. Collision between two
	// ancestors on a primitive name is conflated with the same
	// "ancestor vs ancestor" rule as classes.
	if len(ancestor.PrimitiveTypes) > 0 {
		if dst.PrimitiveTypes == nil {
			dst.PrimitiveTypes = make(map[string]Class, len(ancestor.PrimitiveTypes))
		}
		for k, v := range ancestor.PrimitiveTypes {
			if _, inDescendant := descendantPrims[k]; inDescendant {
				continue // descendant shadow — allowed
			}
			if _, exists := dst.PrimitiveTypes[k]; exists {
				return &schemaConflictError{
					ClassName: k,
					SchemaA:   dst.SchemaID(),
					SchemaB:   ancestor.SchemaID(),
				}
			}
			dst.PrimitiveTypes[k] = v
		}
	}
	// ClassDefinitions: descendant-shadows-ancestor allowed;
	// ancestor-vs-ancestor is a hard conflict.
	if len(ancestor.ClassDefinitions) > 0 {
		if dst.ClassDefinitions == nil {
			dst.ClassDefinitions = make(map[string]Class, len(ancestor.ClassDefinitions))
		}
		for k, v := range ancestor.ClassDefinitions {
			if _, inDescendant := descendantClasses[k]; inDescendant {
				continue // descendant shadow — allowed
			}
			if _, exists := dst.ClassDefinitions[k]; exists {
				return &schemaConflictError{
					ClassName: k,
					SchemaA:   dst.SchemaID(),
					SchemaB:   ancestor.SchemaID(),
				}
			}
			dst.ClassDefinitions[k] = v
		}
	}
	// Packages: merge by name, recursively.
	if len(ancestor.Packages) > 0 {
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
	return nil
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
