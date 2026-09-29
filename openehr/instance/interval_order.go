package instance

// interval_order.go: keeps the two bounds of a generated interval in order.

import (
	"math"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// orderIntervalBounds puts the bounds of a generated interval in order when
// both sides are bounded and the lower bound lies above the upper one,
// which BASE Interval forbids (Limits_consistent). Each side takes its
// bound from its own OPT constraint, so RandomFill can draw the two either
// way round, and two sides with different constraints can give inverted
// example values too.
//
// It orders DV_COUNT and DV_QUANTITY bounds, the two bound types the RM
// floor compares, in a DV_INTERVAL<DV_COUNT> or DV_INTERVAL<DV_QUANTITY>
// and in a DV_INTERVAL<DV_ORDERED> whose two bounds are both counts or
// both quantities. Quantities are ordered only when both bounds share
// their units, since magnitudes in different units do not compare. Other
// intervals are left alone.
//
// An inverted pair is replaced by the first of these two pairs that is in
// order and fits each side's OPT constraint:
//
//   - the two bounds swapped, which keeps the values RandomFill drew;
//   - the lowest value the lower side's constraint accepts, with the
//     highest value the upper side's constraint accepts. A side whose
//     constraint has no end in that direction uses the other side's
//     current bound instead, which a range open that way accepts. At an
//     exclusive real end the accepted extreme is the nearest float64
//     inside it.
//
// These constraints are one-dimensional, so an in-order pair exists only if
// the second pair is in order. When it is not, the two sides' constraints
// admit no ordered interval at all. The generator has no error for a
// constraint it cannot satisfy (sampleValue, for one, falls back to the
// example value without a signal), so the bounds are then left as they
// are, and the interval breaks Limits_consistent.
func orderIntervalBounds(optNode *tcimpl.CompiledNode, rmValue any) {
	switch iv := rmValue.(type) {
	case *rm.DVInterval[rm.DVCount]:
		if !bothBounded(&iv.Interval) {
			return
		}
		lower, upper := boundNodes(optNode)
		if lo, hi, ok := orderPair(iv.Lower.Magnitude, iv.Upper.Magnitude, lower, upper, countScale); ok {
			iv.Lower.Magnitude, iv.Upper.Magnitude = lo, hi
		}
	case *rm.DVInterval[rm.DVQuantity]:
		if !bothBounded(&iv.Interval) {
			return
		}
		lower, upper := boundNodes(optNode)
		if lo, hi, ok := orderPair(quantityKeyOf(iv.Lower), quantityKeyOf(iv.Upper), lower, upper, quantityScale); ok {
			setQuantityKey(&iv.Lower, lo)
			setQuantityKey(&iv.Upper, hi)
		}
	case *rm.DVInterval[rm.DVOrdered]:
		if bothBounded(&iv.Interval) {
			orderOrderedBounds(optNode, &iv.Interval)
		}
	}
}

// orderOrderedBounds orders a DV_INTERVAL<DV_ORDERED> whose two bounds are
// both counts or both quantities, in value or pointer form. A pointer bound
// is changed in place and a value bound is replaced, so each keeps its form.
func orderOrderedBounds(optNode *tcimpl.CompiledNode, iv *rm.Interval[rm.DVOrdered]) {
	if lowerCount, ok := asCount(iv.Lower); ok {
		upperCount, ok := asCount(iv.Upper)
		if !ok {
			return
		}
		lower, upper := boundNodes(optNode)
		if lo, hi, ok := orderPair(lowerCount.Magnitude, upperCount.Magnitude, lower, upper, countScale); ok {
			setOrderedCount(&iv.Lower, lo)
			setOrderedCount(&iv.Upper, hi)
		}
		return
	}
	if lowerQuantity, ok := asQuantity(iv.Lower); ok {
		upperQuantity, ok := asQuantity(iv.Upper)
		if !ok {
			return
		}
		lower, upper := boundNodes(optNode)
		if lo, hi, ok := orderPair(quantityKeyOf(lowerQuantity), quantityKeyOf(upperQuantity), lower, upper, quantityScale); ok {
			setOrderedQuantity(&iv.Lower, lo)
			setOrderedQuantity(&iv.Upper, hi)
		}
	}
}

func bothBounded[T any](iv *rm.Interval[T]) bool {
	return !iv.LowerUnbounded && !iv.UpperUnbounded
}

// scale is what orderPair needs to know about K, the part of one bound type
// that is compared and constrained.
type scale[K any] struct {
	// compare returns a negative number, zero or a positive number as a is
	// below, equal to or above b, and false when a and b do not compare.
	compare func(a, b K) (int, bool)
	// fits reports whether k satisfies the constraint of a side's bound
	// node. A nil node constrains nothing.
	fits func(node *tcimpl.CompiledNode, k K) bool
	// lowest and highest return the lowest and the highest value a side's
	// bound node accepts, in the units of like where units apply, and false
	// when its constraint has no end in that direction.
	lowest, highest func(node *tcimpl.CompiledNode, like K) (K, bool)
}

// orderPair returns an in-order replacement for the inverted pair
// (lower, upper), as orderIntervalBounds describes, and false when the pair
// is in order already, does not compare, or has no in-order replacement.
func orderPair[K any](lower, upper K, lowerNode, upperNode *tcimpl.CompiledNode, s scale[K]) (K, K, bool) {
	if c, ok := s.compare(lower, upper); !ok || c <= 0 {
		return lower, upper, false
	}
	fitsInOrder := func(lo, hi K) bool {
		c, ok := s.compare(lo, hi)
		return ok && c <= 0 && s.fits(lowerNode, lo) && s.fits(upperNode, hi)
	}
	if fitsInOrder(upper, lower) {
		return upper, lower, true
	}
	lo, ok := s.lowest(lowerNode, lower)
	if !ok {
		lo = upper
	}
	hi, ok := s.highest(upperNode, upper)
	if !ok {
		hi = lower
	}
	if fitsInOrder(lo, hi) {
		return lo, hi, true
	}
	return lower, upper, false
}

// boundNodes returns the OPT nodes the generator built an interval's lower
// and upper bounds from, nil for a bound the OPT does not constrain.
func boundNodes(optNode *tcimpl.CompiledNode) (lower, upper *tcimpl.CompiledNode) {
	return firstChild(optNode, "lower"), firstChild(optNode, "upper")
}

// firstChild returns the first child of node's attrName, the alternative
// materialiseSingle builds, or nil when node or the attribute has none.
func firstChild(node *tcimpl.CompiledNode, attrName string) *tcimpl.CompiledNode {
	if node == nil {
		return nil
	}
	attr := node.Attribute(attrName)
	if attr == nil {
		return nil
	}
	children := attr.Children()
	if len(children) == 0 {
		return nil
	}
	return children[0]
}

// primitiveOf returns node's primitive constraint, or nil for a nil node.
func primitiveOf(node *tcimpl.CompiledNode) constraints.PrimitiveConstraint {
	if node == nil {
		return nil
	}
	return node.PrimitiveConstraint()
}

func compareOrdered[N int64 | float64](a, b N) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// countScale orders DV_COUNT bounds by magnitude. The OPT constrains a
// DV_COUNT bound through the C_INTEGER on its magnitude attribute.
var countScale = scale[int64]{
	compare: func(a, b int64) (int, bool) { return compareOrdered(a, b), true },
	fits: func(node *tcimpl.CompiledNode, m int64) bool {
		pc := primitiveOf(firstChild(node, "magnitude"))
		return pc == nil || len(pc.Validate(m)) == 0
	},
	lowest: func(node *tcimpl.CompiledNode, _ int64) (int64, bool) {
		return integerEnd(node, true)
	},
	highest: func(node *tcimpl.CompiledNode, _ int64) (int64, bool) {
		return integerEnd(node, false)
	},
}

// integerEnd returns the lowest (low) or highest value the C_INTEGER on a
// DV_COUNT bound node's magnitude accepts: the extreme list entry the range
// also admits, or the range's own end. It returns false when there is no
// such constraint, or when the constraint has no end in that direction.
func integerEnd(node *tcimpl.CompiledNode, low bool) (int64, bool) {
	c, ok := primitiveOf(firstChild(node, "magnitude")).(constraints.CInteger)
	if !ok {
		return 0, false
	}
	if len(c.List) > 0 {
		var end int64
		found := false
		for _, n := range c.List {
			if len(c.Validate(n)) != 0 {
				continue
			}
			if !found || (low && n < end) || (!low && n > end) {
				end, found = n, true
			}
		}
		return end, found
	}
	r := c.Range
	switch {
	case !r.IsBounded():
		return 0, false
	case low && !r.LowerUnbounded:
		n := math.Ceil(r.Lower)
		if !r.LowerInclusive && n == r.Lower {
			n++
		}
		return int64(n), true
	case !low && !r.UpperUnbounded:
		n := math.Floor(r.Upper)
		if !r.UpperInclusive && n == r.Upper {
			n--
		}
		return int64(n), true
	}
	return 0, false
}

// quantityKey is the part of a DV_QUANTITY bound that is compared and
// constrained.
type quantityKey struct {
	magnitude float64
	units     string
}

func quantityKeyOf(q rm.DVQuantity) quantityKey {
	return quantityKey{magnitude: float64(q.Magnitude), units: q.Units}
}

func setQuantityKey(q *rm.DVQuantity, k quantityKey) {
	q.Magnitude = rm.Real(k.magnitude)
	q.Units = k.units
}

// quantityScale orders DV_QUANTITY bounds by magnitude when their units
// match. The OPT constrains a DV_QUANTITY bound on the bound node itself.
var quantityScale = scale[quantityKey]{
	compare: func(a, b quantityKey) (int, bool) {
		if a.units != b.units {
			return 0, false
		}
		return compareOrdered(a.magnitude, b.magnitude), true
	},
	fits: func(node *tcimpl.CompiledNode, k quantityKey) bool {
		pc := primitiveOf(node)
		v := constraints.QuantityValue{Magnitude: k.magnitude, Units: k.units, Precision: -1}
		return pc == nil || len(pc.Validate(v)) == 0
	},
	lowest: func(node *tcimpl.CompiledNode, like quantityKey) (quantityKey, bool) {
		return magnitudeEnd(node, like.units, true)
	},
	highest: func(node *tcimpl.CompiledNode, like quantityKey) (quantityKey, bool) {
		return magnitudeEnd(node, like.units, false)
	},
}

// magnitudeEnd returns the lowest (low) or highest magnitude a DV_QUANTITY
// bound node accepts in units: the end of the magnitude range of the first
// entry for those units, which is the entry DvQuantity.Validate checks. At
// an exclusive end it is the nearest float64 inside the range. It returns
// false when there is no such entry, or when its range has no end in that
// direction.
func magnitudeEnd(node *tcimpl.CompiledNode, units string, low bool) (quantityKey, bool) {
	c, ok := primitiveOf(node).(constraints.DvQuantity)
	if !ok {
		return quantityKey{}, false
	}
	for _, u := range c.Units {
		if u.Units != units {
			continue
		}
		r := u.Magnitude
		switch {
		case !r.IsBounded():
			return quantityKey{}, false
		case low && !r.LowerUnbounded:
			m := r.Lower
			if !r.LowerInclusive {
				m = math.Nextafter(m, math.Inf(1))
			}
			return quantityKey{magnitude: m, units: units}, true
		case !low && !r.UpperUnbounded:
			m := r.Upper
			if !r.UpperInclusive {
				m = math.Nextafter(m, math.Inf(-1))
			}
			return quantityKey{magnitude: m, units: units}, true
		}
		return quantityKey{}, false
	}
	return quantityKey{}, false
}

// asCount and asQuantity read a DV_INTERVAL<DV_ORDERED> bound held in value
// or pointer form; a nil or typed-nil bound is neither.
func asCount(v rm.DVOrdered) (rm.DVCount, bool) {
	switch x := v.(type) {
	case rm.DVCount:
		return x, true
	case *rm.DVCount:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVCount{}, false
}

func asQuantity(v rm.DVOrdered) (rm.DVQuantity, bool) {
	switch x := v.(type) {
	case rm.DVQuantity:
		return x, true
	case *rm.DVQuantity:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVQuantity{}, false
}

func setOrderedCount(slot *rm.DVOrdered, m int64) {
	switch x := (*slot).(type) {
	case *rm.DVCount:
		x.Magnitude = m
	case rm.DVCount:
		x.Magnitude = m
		*slot = x
	}
}

func setOrderedQuantity(slot *rm.DVOrdered, k quantityKey) {
	switch x := (*slot).(type) {
	case *rm.DVQuantity:
		setQuantityKey(x, k)
	case rm.DVQuantity:
		setQuantityKey(&x, k)
		*slot = x
	}
}
