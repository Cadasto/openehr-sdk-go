// Package bmmdiff computes a structured, human-readable diff between
// two loaded BMM schemas. Distinct from a textual `git diff` because
// the comparison understands the BMM shape — added/removed classes,
// per-class property additions/removals/changes, cardinality changes,
// function changes, primitive additions/removals.
//
// The package is intentionally small. The output Report is a plain
// data structure; rendering (Format) lives in render.go and a
// CHANGELOG-entry helper (SuggestChangelogEntry) is in changelog.go.
//
// Internal package: consumed by cmd/bmmdiff and by the simulated
// version-bump test in internal/bmmgen.
package bmmdiff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// Report is the structured semantic diff between two BMM schemas
// (old → new). All slices are sorted for deterministic rendering.
type Report struct {
	// OldSchemaID / NewSchemaID are the canonical ids of the
	// compared schemas (e.g. "openehr_rm_1.2.0").
	OldSchemaID string
	NewSchemaID string

	// AddedClasses are classes present in new but not in old.
	AddedClasses []ClassRef
	// RemovedClasses are classes present in old but not in new.
	RemovedClasses []ClassRef
	// ChangedClasses are classes present in both with structural
	// differences. Sorted by ClassName.
	ChangedClasses []ClassChange

	// AddedPrimitives are primitive type names new but not in old.
	AddedPrimitives []string
	// RemovedPrimitives are primitive type names in old but not in new.
	RemovedPrimitives []string
}

// HasChanges returns true when at least one of the diff fields is
// non-empty.
func (r *Report) HasChanges() bool {
	if r == nil {
		return false
	}
	return len(r.AddedClasses) > 0 ||
		len(r.RemovedClasses) > 0 ||
		len(r.ChangedClasses) > 0 ||
		len(r.AddedPrimitives) > 0 ||
		len(r.RemovedPrimitives) > 0
}

// ClassRef identifies a class by its BMM name and its enclosing
// dotted package path (or "" if it could not be located in the
// schema's package tree). Used in the added/removed lists.
type ClassRef struct {
	ClassName string
	Package   string
}

// ClassChange enumerates the kinds of intra-class differences. All
// fields are optional; one ClassChange may carry just an ancestor
// change, just an added property, etc.
type ClassChange struct {
	ClassName       string
	OldAncestors    []string
	NewAncestors    []string
	AncestorsDiffer bool

	AddedProperties   []PropertyRef
	RemovedProperties []string
	ChangedProperties []PropertyChange

	AddedFunctions   []string
	RemovedFunctions []string

	// CardinalityChanges captures cardinality alterations on
	// ContainerProperty values that were present in both schemas.
	// Reported separately from ChangedProperties for emphasis since
	// cardinality is a common review concern at version bumps.
	CardinalityChanges []CardinalityChange
}

// PropertyRef names a property and renders its short type.
type PropertyRef struct {
	Name     string
	TypeName string
}

// PropertyChange records a property-level alteration. The TypeName
// strings use the same compact rendering as PropertyRef (the BMM
// type spelled with container brackets / generics).
type PropertyChange struct {
	Name           string
	OldType        string
	NewType        string
	OldMandatory   bool
	NewMandatory   bool
	MandatoryDiff  bool
	TypeDiff       bool
	OldPropertyKey string // discriminator kind (e.g. P_BMM_SINGLE_PROPERTY)
	NewPropertyKey string
	KindDiff       bool
}

// CardinalityChange records a container-cardinality alteration.
type CardinalityChange struct {
	Name     string
	OldLower int
	OldUpper string // "*" for unbounded
	NewLower int
	NewUpper string
}

// Diff computes a Report comparing oldS (the older schema) with newS
// (the newer schema). Both arguments MUST be non-nil. The function
// is total — it never returns an error; malformed values render as
// best-effort strings.
func Diff(oldS, newS *bmm.Schema) *Report {
	if oldS == nil || newS == nil {
		return &Report{}
	}
	r := &Report{
		OldSchemaID: oldS.SchemaID(),
		NewSchemaID: newS.SchemaID(),
	}

	oldClasses := oldS.ClassDefinitions
	newClasses := newS.ClassDefinitions

	oldPkgMap := classPackageMap(oldS)
	newPkgMap := classPackageMap(newS)

	// Added / removed classes.
	for _, name := range sortedKeys(newClasses) {
		if _, ok := oldClasses[name]; !ok {
			r.AddedClasses = append(r.AddedClasses, ClassRef{
				ClassName: name,
				Package:   newPkgMap[name],
			})
		}
	}
	for _, name := range sortedKeys(oldClasses) {
		if _, ok := newClasses[name]; !ok {
			r.RemovedClasses = append(r.RemovedClasses, ClassRef{
				ClassName: name,
				Package:   oldPkgMap[name],
			})
		}
	}

	// Changed classes — present in both, structurally different.
	for _, name := range sortedKeys(oldClasses) {
		newCls, ok := newClasses[name]
		if !ok {
			continue
		}
		if ch, changed := diffClass(name, oldClasses[name], newCls); changed {
			r.ChangedClasses = append(r.ChangedClasses, ch)
		}
	}

	// Primitive types — name-set only (we do not compare primitive
	// internals because they are stable Go-side aliases).
	for _, name := range sortedKeys(newS.PrimitiveTypes) {
		if _, ok := oldS.PrimitiveTypes[name]; !ok {
			r.AddedPrimitives = append(r.AddedPrimitives, name)
		}
	}
	for _, name := range sortedKeys(oldS.PrimitiveTypes) {
		if _, ok := newS.PrimitiveTypes[name]; !ok {
			r.RemovedPrimitives = append(r.RemovedPrimitives, name)
		}
	}
	return r
}

// diffClass compares two classes and returns the populated change
// plus a flag indicating whether anything differs.
func diffClass(name string, oldC, newC bmm.Class) (ClassChange, bool) {
	ch := ClassChange{ClassName: name}
	differs := false

	// Ancestors (a sorted-set comparison; positional order is not
	// semantically meaningful in BMM).
	oldAnc := append([]string(nil), oldC.Ancestors()...)
	newAnc := append([]string(nil), newC.Ancestors()...)
	sort.Strings(oldAnc)
	sort.Strings(newAnc)
	if !stringSliceEqual(oldAnc, newAnc) {
		ch.OldAncestors = oldAnc
		ch.NewAncestors = newAnc
		ch.AncestorsDiffer = true
		differs = true
	}

	// Properties — pull from the embedded common via Class
	// interfaces. SimpleClass / Interface / Enumeration all embed
	// classCommon; we reach properties via a tiny type switch.
	oldProps, oldFns := classMembers(oldC)
	newProps, newFns := classMembers(newC)

	// Property add/remove/change.
	for _, pname := range sortedKeys(newProps) {
		if _, ok := oldProps[pname]; !ok {
			ch.AddedProperties = append(ch.AddedProperties, PropertyRef{
				Name:     pname,
				TypeName: renderPropertyType(newProps[pname]),
			})
			differs = true
		}
	}
	for _, pname := range sortedKeys(oldProps) {
		if _, ok := newProps[pname]; !ok {
			ch.RemovedProperties = append(ch.RemovedProperties, pname)
			differs = true
		}
	}
	for _, pname := range sortedKeys(oldProps) {
		newP, ok := newProps[pname]
		if !ok {
			continue
		}
		oldP := oldProps[pname]
		if pc, cChange := diffProperty(pname, oldP, newP); cChange {
			ch.ChangedProperties = append(ch.ChangedProperties, pc)
			differs = true
		}
		if cc, hasCC := cardinalityChange(pname, oldP, newP); hasCC {
			ch.CardinalityChanges = append(ch.CardinalityChanges, cc)
			differs = true
		}
	}

	// Functions — name set only (signatures are not normally part of
	// a semantic version-bump review; the regenerated code rebuilds
	// the signatures from BMM regardless).
	for _, fname := range sortedKeys(newFns) {
		if _, ok := oldFns[fname]; !ok {
			ch.AddedFunctions = append(ch.AddedFunctions, fname)
			differs = true
		}
	}
	for _, fname := range sortedKeys(oldFns) {
		if _, ok := newFns[fname]; !ok {
			ch.RemovedFunctions = append(ch.RemovedFunctions, fname)
			differs = true
		}
	}

	return ch, differs
}

// diffProperty returns the population PropertyChange and a flag.
// Same-kind properties compare type + mandatory; cross-kind
// transitions (Single → Container, etc.) populate KindDiff.
func diffProperty(name string, oldP, newP bmm.Property) (PropertyChange, bool) {
	pc := PropertyChange{Name: name}
	oldKind := propertyKind(oldP)
	newKind := propertyKind(newP)
	pc.OldPropertyKey = oldKind
	pc.NewPropertyKey = newKind
	differs := false
	if oldKind != newKind {
		pc.KindDiff = true
		differs = true
	}
	oldType := renderPropertyType(oldP)
	newType := renderPropertyType(newP)
	if oldType != newType {
		pc.OldType = oldType
		pc.NewType = newType
		pc.TypeDiff = true
		differs = true
	}
	oldMand := propertyMandatory(oldP)
	newMand := propertyMandatory(newP)
	if oldMand != newMand {
		pc.OldMandatory = oldMand
		pc.NewMandatory = newMand
		pc.MandatoryDiff = true
		differs = true
	}
	return pc, differs
}

// cardinalityChange extracts a CardinalityChange when both properties
// are ContainerProperty with non-equal cardinality bounds.
func cardinalityChange(name string, oldP, newP bmm.Property) (CardinalityChange, bool) {
	oldC, okOld := oldP.(*bmm.ContainerProperty)
	newC, okNew := newP.(*bmm.ContainerProperty)
	if !okOld || !okNew {
		return CardinalityChange{}, false
	}
	oldL, oldU := cardBounds(oldC.Cardinality)
	newL, newU := cardBounds(newC.Cardinality)
	if oldL == newL && oldU == newU {
		return CardinalityChange{}, false
	}
	return CardinalityChange{
		Name:     name,
		OldLower: oldL, OldUpper: oldU,
		NewLower: newL, NewUpper: newU,
	}, true
}

func cardBounds(c *bmm.Cardinality) (int, string) {
	if c == nil {
		return 0, "*"
	}
	upper := "*"
	if !c.UpperUnbounded && c.Upper != nil {
		upper = fmt.Sprintf("%d", *c.Upper)
	}
	return c.Lower, upper
}

// classMembers returns the Properties and Functions maps for any
// concrete bmm.Class implementation. Returns empty maps on unknown
// kinds — Enumeration carries neither and we treat it as a leaf
// for diff purposes.
//
// bmm.SimpleClass and bmm.Interface both embed the unexported
// classCommon which carries Properties and Functions. Both struct
// types expose the embedded fields by name (Properties, Functions),
// so we recover them via a type switch.
func classMembers(c bmm.Class) (map[string]bmm.Property, map[string]*bmm.Function) {
	var props map[string]bmm.Property
	var fns map[string]*bmm.Function
	switch v := c.(type) {
	case *bmm.SimpleClass:
		props, fns = v.Properties, v.Functions
	case *bmm.Interface:
		props, fns = v.Properties, v.Functions
	}
	if props == nil {
		props = map[string]bmm.Property{}
	}
	if fns == nil {
		fns = map[string]*bmm.Function{}
	}
	return props, fns
}

// propertyKind maps a Property to its P_BMM_* discriminator-equivalent.
func propertyKind(p bmm.Property) string {
	switch p.(type) {
	case *bmm.SingleProperty:
		return bmm.TypeP_BMM_SINGLE_PROPERTY
	case *bmm.SinglePropertyOpen:
		return bmm.TypeP_BMM_SINGLE_PROPERTY_OPEN
	case *bmm.ContainerProperty:
		return bmm.TypeP_BMM_CONTAINER_PROPERTY
	case *bmm.GenericProperty:
		return bmm.TypeP_BMM_GENERIC_PROPERTY
	}
	return ""
}

// propertyMandatory returns the is_mandatory flag for any property
// kind.
func propertyMandatory(p bmm.Property) bool {
	switch v := p.(type) {
	case *bmm.SingleProperty:
		return v.IsMandatory
	case *bmm.SinglePropertyOpen:
		return v.IsMandatory
	case *bmm.ContainerProperty:
		return v.IsMandatory
	case *bmm.GenericProperty:
		return v.IsMandatory
	}
	return false
}

// renderPropertyType produces a compact human-readable type spelling
// for any property kind. Container types render with "List<T>" /
// "Set<T>" / "Hash<K,V>" syntax; generic types render with "Root<a,b>"
// — these mirror how the openEHR spec text talks about them.
func renderPropertyType(p bmm.Property) string {
	switch v := p.(type) {
	case *bmm.SingleProperty:
		return v.TypeName
	case *bmm.SinglePropertyOpen:
		return v.TypeName
	case *bmm.ContainerProperty:
		if v.TypeDef == nil {
			return "<container>"
		}
		return renderContainerType(v.TypeDef)
	case *bmm.GenericProperty:
		if v.TypeDef == nil {
			return "<generic>"
		}
		return renderGenericType(v.TypeDef)
	}
	return ""
}

func renderContainerType(c *bmm.ContainerType) string {
	inner := renderType(c.TypeDef)
	return fmt.Sprintf("%s<%s>", c.ContainerType, inner)
}

func renderGenericType(g *bmm.GenericType) string {
	var args []string
	for _, p := range g.GenericParameters {
		args = append(args, renderType(p))
	}
	// generic_parameter_defs (keyed form): render as "K=<type>".
	if len(args) == 0 && len(g.GenericParameterDefs) > 0 {
		for _, k := range sortedKeys(g.GenericParameterDefs) {
			args = append(args, fmt.Sprintf("%s=%s", k, renderType(g.GenericParameterDefs[k])))
		}
	}
	if len(args) == 0 {
		return g.RootType
	}
	return fmt.Sprintf("%s<%s>", g.RootType, strings.Join(args, ","))
}

func renderType(t bmm.Type) string {
	switch v := t.(type) {
	case *bmm.SimpleType:
		return v.TypeName
	case *bmm.GenericType:
		return renderGenericType(v)
	case *bmm.ContainerType:
		return renderContainerType(v)
	}
	return "?"
}

// classPackageMap walks a schema's package tree and returns a flat
// map: class name → dotted package path. Mirrors internal/bmmgen's
// walkPackage so the diff output uses the same labels.
func classPackageMap(s *bmm.Schema) map[string]string {
	m := map[string]string{}
	if s == nil {
		return m
	}
	for _, k := range sortedKeys(s.Packages) {
		root := s.Packages[k]
		walkPackage(root, root.Name, m)
	}
	return m
}

func walkPackage(p *bmm.Package, path string, into map[string]string) {
	if p == nil {
		return
	}
	for _, c := range p.Classes {
		if _, exists := into[c]; !exists {
			into[c] = path
		}
	}
	for _, key := range sortedKeys(p.Packages) {
		sub := p.Packages[key]
		childPath := path + "." + sub.Name
		walkPackage(sub, childPath, into)
	}
}

// sortedKeys returns the keys of m in lexicographic order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
