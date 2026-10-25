// Package rmbind holds the rule that binds an RM value to one of the nodes
// an operational template allows under one attribute, and the type tests
// that rule reads.
//
// The composition builder (openehr/composition) binds each value Set is
// given by [Bind]. The validator (openehr/validation) takes the nodes of the
// tier Bind chooses from ([Tier]) and binds a value within it to the first
// node under whose constraints the value validates, so a value built under
// the node Bind returns, which validates there, binds to that node in both.
// The builder accepts less than [Bind] admits: it
// takes only RM values, where the validator also binds a Go primitive at a
// leaf, and it checks on its own that the RM attribute at the path can hold
// the value. The validator's RM type match also reads
// [ConformsToNode], [TypedIntervalBound] and [IntervalConformsTo], and its
// primitive leaves read [PrimitiveValueMatchesShortName] and [PrimitiveValue].
//
// The rule has three tiers, tried in order, and within a tier the first
// node in template order wins:
//
//  1. a node that names the value's own RM type;
//  2. a node that names the AOM 1.4 primitive short name whose canonical
//     DV wrapper the value is, such as DURATION for a DV_DURATION;
//  3. a node the value's type conforms to through the pinned class
//     hierarchy, a node that declares no RM type, or a typed
//     DV_INTERVAL<T> node each present bound of which conforms to T.
//
// The package uses no reflection.
package rmbind

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TypeName returns the RM type name of v: the ITS-JSON parameterised name
// for a typed DV_INTERVAL instantiation, such as DV_INTERVAL<DV_QUANTITY>,
// and otherwise the registered name of v's concrete type. ok is false for
// nil, for an interface that carries a nil pointer, and for a value that is
// no RM value.
func TypeName(v any) (name string, ok bool) {
	if v == nil || rm.IsTypedNil(v) {
		return "", false
	}
	if name, ok := rmnames.TypedIntervalName(v); ok {
		return name, true
	}
	return rm.RMTypeName(v)
}

// describe returns the name the binding compares with a node's RM type:
// v's RM type name ([TypeName]), or, for a value that has none, "<nil>" for
// nil and v's Go type otherwise, names no template node declares.
func describe(v any) string {
	if v == nil {
		return "<nil>"
	}
	if name, ok := TypeName(v); ok {
		return name
	}
	return fmt.Sprintf("%T", v)
}

// Bind returns the node, among the nodes eligible accepts, that binds the
// value v, or nil when none does. It tries the three tiers in order and
// returns the first node, in the order of nodes, of the first tier that
// admits v:
//
//  1. a node that names v's own RM type ([namesRMType]);
//  2. a node that names the AOM 1.4 primitive short name whose canonical
//     DV wrapper v is ([wrapsShortName]);
//  3. a node v's type conforms to through the class hierarchy, an untyped
//     node, or a typed DV_INTERVAL<T> node each present bound of which
//     conforms to T ([conformsToChild]).
//
// A value therefore binds the tightest node that admits it, not a looser
// one listed first: under DV_TEXT then DV_CODED_TEXT, a DV_CODED_TEXT binds
// the DV_CODED_TEXT node, and under DATA_VALUE then DURATION, a DV_DURATION
// binds the DURATION node.
func Bind(nodes []*tcimpl.CompiledNode, v any, eligible func(*tcimpl.CompiledNode) bool) *tcimpl.CompiledNode {
	gotType := describe(v)
	for tier := range tierCount {
		for _, c := range nodes {
			if eligible(c) && admits(tier, c, gotType, v) {
				return c
			}
		}
	}
	return nil
}

// Tier appends to dst the nodes, among those eligible accepts, of the first
// of the three tiers of [Bind] that admits the value v, in the order of
// nodes, and returns the extended slice; it appends none when no node admits
// v. Its first node is the one Bind returns. The builder takes that node; the
// validator binds v within the tier by walking it under each node in turn.
func Tier(dst, nodes []*tcimpl.CompiledNode, v any, eligible func(*tcimpl.CompiledNode) bool) []*tcimpl.CompiledNode {
	gotType := describe(v)
	from := len(dst)
	for tier := range tierCount {
		for _, c := range nodes {
			if eligible(c) && admits(tier, c, gotType, v) {
				dst = append(dst, c)
			}
		}
		if len(dst) > from {
			break
		}
	}
	return dst
}

// tierCount is the number of binding tiers [admits] tests.
const tierCount = 3

// admits reports whether the node c admits the value v of RM type gotType
// ([describe]) in the binding tier numbered tier, from 0, of the three
// [Bind] lists: the one rule Bind and [Tier] both apply.
func admits(tier int, c *tcimpl.CompiledNode, gotType string, v any) bool {
	switch tier {
	case 0:
		return namesRMType(c, gotType, v)
	case 1:
		return wrapsShortName(c, gotType)
	default:
		return conformsToChild(c, gotType, v)
	}
}

// namesRMType reports whether the node c names the value val's own RM type,
// gotType. That is the same name, with one refinement for an interval that
// went through a canonical-JSON round-trip and decoded as the bare
// DV_INTERVAL: its type is the parameterised DV_INTERVAL<T> its bounds give,
// so a typed node whose T every present bound is names it. One with no bound
// present has no parameter, so only a bare DV_INTERVAL node names its type,
// and a typed node admits it in the third tier ([conformsToChild]). An AOM
// 1.4 primitive short name over a primitive val (an integer under INTEGER, a
// string under DURATION) also names val's type. val is read as the RM holds
// it, so DV_COUNT.magnitude arrives as an rm.Integer64, not as the int64 the
// validator later hands a C_INTEGER. An untyped node names no type.
func namesRMType(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" {
		return false
	}
	if gotType == "DV_INTERVAL" {
		bounded := collapsedIntervalBounded(val)
		if bound, typed := TypedIntervalBound(want); typed {
			return bounded && intervalBoundsSatisfy(val, bound)
		}
		if want == "DV_INTERVAL" {
			return !bounded
		}
	}
	if gotType == want {
		return true
	}
	// AOM 1.4 primitive short name (DURATION, DATE, INTEGER, …)
	// pinned under a BMM-typed attribute channel: the RM value
	// may be a Go string, integer, real, or bool rather than an
	// RM wrapper type.
	return tcimpl.IsAOMPrimitiveShortName(want) && c.PrimitiveConstraint() != nil &&
		PrimitiveValueMatchesShortName(want, val)
}

// wrapsShortName reports whether the node c names an AOM 1.4 primitive
// short name whose canonical DV wrapper ([primitiveWrappers]) is the RM type
// gotType.
func wrapsShortName(c *tcimpl.CompiledNode, gotType string) bool {
	wrapper, ok := primitiveWrappers[c.RMTypeName()]
	return ok && wrapper == gotType
}

// conformsToChild reports whether the node c declares no RM type, or one the
// RM type gotType conforms to ([ConformsToNode]). Under a typed
// DV_INTERVAL<T> node each present bound has to be a T, that is, conform to
// T ([IntervalConformsTo]): a DV_COUNT bound is a DV_ORDERED. A round-tripped
// bare DV_INTERVAL val with no bound present carries no T to check, so a
// typed DV_INTERVAL<T> node admits it here, after any bare DV_INTERVAL node
// has named its type ([namesRMType]).
func conformsToChild(c *tcimpl.CompiledNode, gotType string, val any) bool {
	want := c.RMTypeName()
	if want == "" || ConformsToNode(c, gotType) {
		return true
	}
	return IntervalConformsTo(gotType, want, val)
}

// ConformsToNode reports whether the RM type concrete conforms to the RM
// type the compiled node c declares: it is that type or descends from it in
// the pinned class hierarchy, as the compile recorded it for c, or c names an
// AOM 1.4 primitive short name whose canonical DV wrapper concrete is. A
// parameterised concrete, such as DV_INTERVAL<DV_COUNT>, is looked up by its
// class. A parameterised node type is no class the hierarchy knows, so
// [IntervalConformsTo] decides it instead.
func ConformsToNode(c *tcimpl.CompiledNode, concrete string) bool {
	class := bmmtype.Class(concrete)
	if conforms, known := tcimpl.ConformsTo(c, class); known {
		return conforms
	}
	wrapper, ok := primitiveWrappers[c.RMTypeName()]
	return ok && wrapper == class
}

// rmTypeIsSubtypeOf reports whether the RM type concrete conforms to the
// declared RM type declared: equal to it, or a descendant of it in the pinned
// class hierarchy (rminfo), whether declared is abstract or concrete.
//
// A parameterised value type such as "DV_INTERVAL<DV_COUNT>" is looked up by
// its class, "DV_INTERVAL", because the generic parameter does not change
// which classes it descends from. The declared type is looked up as written:
// a parameterised DV_INTERVAL<T> is no class name, so the hierarchy does not
// know it.
//
// When the hierarchy does not know one of the two names, an AOM 1.4
// primitive short name in declared still admits its canonical DV wrapper
// ([primitiveWrappers]).
func rmTypeIsSubtypeOf(concrete, declared string) bool {
	class := bmmtype.Class(concrete)
	if h, ok := rminfo.Default.(rminfo.Hierarchy); ok {
		if conforms, known := h.ConformsTo(class, declared); known {
			return conforms
		}
	}
	wrapper, ok := primitiveWrappers[declared]
	return ok && wrapper == class
}

// IntervalConformsTo reports whether an interval value of RM type got, val,
// conforms to the typed interval want, DV_INTERVAL<T>: each present bound
// has to be a T, which a bound of T's own type or of one that descends from
// T in the pinned class hierarchy is. A typed value, DV_INTERVAL<U>, conforms
// when U does; a round-tripped bare DV_INTERVAL is read by its bounds'
// runtime types, and one with no bound present conforms to every T. An
// abstract T such as DV_ORDERED therefore admits DV_COUNT bounds. Any other
// pair of types does not conform here.
func IntervalConformsTo(got, want string, val any) bool {
	wantBound, wantTyped := TypedIntervalBound(want)
	if !wantTyped {
		return false
	}
	if got == "DV_INTERVAL" {
		return intervalBoundsConform(val, wantBound)
	}
	gotBound, gotTyped := TypedIntervalBound(got)
	return gotTyped && rmTypeIsSubtypeOf(gotBound, wantBound)
}

// TypedIntervalBound returns the bound type of a parameterised interval
// name, "DV_QUANTITY" for "DV_INTERVAL<DV_QUANTITY>". ok is false for any
// other name, the bare "DV_INTERVAL" and a malformed spelling included.
func TypedIntervalBound(name string) (bound string, ok bool) {
	class, params, ok := bmmtype.Split(name)
	if !ok || class != "DV_INTERVAL" || len(params) != 1 {
		return "", false
	}
	return params[0], true
}

// intervalBoundsSatisfy reports whether a round-trip-collapsed
// DVInterval[DVOrdered] names DV_INTERVAL<wantInner> exactly, by the
// runtime types of its bounds (which survive the round-trip via their own
// `_type`). Every *present* bound must be of the wanted element type
// itself; a fully unbounded interval carries no bound to key off and
// trivially satisfies any element type. (Requiring agreement rather than
// accepting on either bound rejects a malformed interval whose lower and
// upper disagree — well-formed DV_INTERVAL<T> bounds are both T.) The
// first binding tier reads it ([namesRMType]); a bound that only conforms
// to wantInner is read by [intervalBoundsConform].
func intervalBoundsSatisfy(val any, wantInner string) bool {
	bounds, ok := collapsedIntervalBounds(val)
	if !ok {
		return false
	}
	for _, b := range bounds {
		if describe(b) != wantInner {
			return false
		}
	}
	// Every present bound matched (or the interval is fully unbounded):
	// an unbounded interval satisfies any DV_INTERVAL<T>.
	return true
}

// intervalBoundsConform reports whether val is a round-trip-collapsed
// DVInterval[DVOrdered] each of whose present bounds conforms to wantInner:
// its type is wantInner or descends from it in the pinned class hierarchy
// ([rmTypeIsSubtypeOf]). An interval with no bound present conforms. The
// first binding tier names a type exactly ([intervalBoundsSatisfy]); this is
// the conformance the third tier and the RM type match read.
func intervalBoundsConform(val any, wantInner string) bool {
	bounds, ok := collapsedIntervalBounds(val)
	if !ok {
		return false
	}
	for _, b := range bounds {
		if !rmTypeIsSubtypeOf(describe(b), wantInner) {
			return false
		}
	}
	return true
}

// collapsedIntervalBounded reports whether val is a round-trip-collapsed
// DVInterval[DVOrdered] with at least one present bound, a bound that
// gives the interval its parameterised type ([namesRMType]).
func collapsedIntervalBounded(val any) bool {
	bounds, ok := collapsedIntervalBounds(val)
	return ok && len(bounds) > 0
}

// collapsedIntervalBounds returns the present bounds of a
// round-trip-collapsed DVInterval[DVOrdered], lower first; a side that
// is unbounded or carries no bound is absent. ok is false when val is
// no such interval.
func collapsedIntervalBounds(val any) (bounds []rm.DVOrdered, ok bool) {
	var iv *rm.DVInterval[rm.DVOrdered]
	switch v := val.(type) {
	case *rm.DVInterval[rm.DVOrdered]:
		iv = v
	case rm.DVInterval[rm.DVOrdered]:
		iv = &v
	default:
		return nil, false
	}
	if !iv.LowerUnbounded && iv.Lower != nil {
		bounds = append(bounds, iv.Lower)
	}
	if !iv.UpperUnbounded && iv.Upper != nil {
		bounds = append(bounds, iv.Upper)
	}
	return bounds, true
}

// PrimitiveValueMatchesShortName reports whether val, a Go value an RM
// attribute holds, satisfies a node that names the AOM 1.4 primitive short
// name shortName (INTEGER, REAL, DURATION, STRING, …) under an attribute of
// a primitive type: a bool under BOOLEAN, an integer under INTEGER (a Go
// integer, an rm.Integer or an rm.Integer64, as [rm.AsInt64] reads them), a
// real under REAL, a string under DATE, TIME, DATE_TIME and DURATION, and a
// string, an rm.Character or a non-nil *string under STRING (AOM 1.4 has no
// C_CHARACTER, so a template constrains a Character, TERM_MAPPING.match, with
// a C_STRING). An optional Boolean, Integer or
// Real the RM holds by pointer matches as the value it points to
// ([PrimitiveValue]); a nil one is an absent attribute and matches nothing.
func PrimitiveValueMatchesShortName(shortName string, val any) bool {
	val = PrimitiveValue(val)
	switch shortName {
	case "BOOLEAN":
		_, ok := val.(bool)
		return ok
	case "INTEGER":
		return rm.IsInt64(val)
	case "REAL":
		return rm.IsReal(val)
	case "DATE", "TIME", "DATE_TIME", "DURATION":
		_, ok := val.(string)
		return ok
	case "STRING":
		// An optional String attribute (DV_TEXT.formatting,
		// DV_IDENTIFIER.issuer, …) is a *string in the RM; the reader
		// returns it only when set, so a nil pointer is absent.
		switch v := val.(type) {
		case *string:
			return v != nil
		case string, rm.Character:
			return true
		}
		return false
	default:
		return false
	}
}

// PrimitiveValue returns the value of val, an optional primitive attribute
// the RM holds by pointer: what a *bool, *rm.Integer or *rm.Real that is not
// nil points to (DV_QUANTITY.precision, DV_AMOUNT.accuracy and
// accuracy_is_percent, INTERVAL_EVENT.sample_count). These are the pointer
// types the RM's primitive fields have, a *string apart; a test keeps the
// two in step. A nil one is an absent attribute and comes back as nil. Any
// other val, a *string included, comes back as it is: an optional String
// matches STRING alone, which [PrimitiveValueMatchesShortName] decides
// itself.
func PrimitiveValue(val any) any {
	switch p := val.(type) {
	case *bool:
		return pointee(p)
	case *rm.Integer:
		return pointee(p)
	case *rm.Real:
		return pointee(p)
	}
	return val
}

// pointee returns what p points to, or nil for a nil p.
func pointee[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// primitiveWrappers maps each AOM 1.4 primitive short name a template may
// declare as an RM type, typically under C_PRIMITIVE_OBJECT, to the
// canonical DV wrapper that carries its value. The short names are no
// classes of the hierarchy, so the hierarchy cannot admit the wrapper; this
// table does. Each wrapper is the type the example generator
// (openehr/instance, concreteFor) builds for the short name, so a generated
// value binds the node it was built for and passes the validator's RM type
// match.
//
// A primitive under a short name (an integer under INTEGER, such as the
// rm.Integer64 of DV_COUNT.magnitude, or a string under DURATION) is
// admitted apart from this table, by [PrimitiveValueMatchesShortName].
var primitiveWrappers = map[string]string{
	"DURATION":  "DV_DURATION",
	"DATE":      "DV_DATE",
	"TIME":      "DV_TIME",
	"DATE_TIME": "DV_DATE_TIME",
	"BOOLEAN":   "DV_BOOLEAN",
	"INTEGER":   "DV_COUNT",
}
