package instance

// interval_order.go: keeps the two bounds of a generated interval in order.

import (
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
// The bounds are replaced by the first of these pairs that is in order and
// that fits each side's OPT constraint: the two bounds swapped; the two
// sides' example values; the lower side's example on both sides; the upper
// side's example on both sides; the lower bound on both sides; the upper
// bound on both sides. When none fits, the bounds are left as they are.
//
// Only DV_COUNT and DV_QUANTITY bounds are ordered, the two bound types the
// RM floor compares, and a DV_QUANTITY pair only when both bounds share
// their units. Other interval types are left alone.
func orderIntervalBounds(optNode *tcimpl.CompiledNode, rmValue any) {
	switch iv := rmValue.(type) {
	case *rm.DVInterval[rm.DVCount]:
		orderBounds(optNode, &iv.Interval, countOrder)
	case *rm.DVInterval[rm.DVQuantity]:
		orderBounds(optNode, &iv.Interval, quantityOrder)
	}
}

// boundOrder is what orderBounds needs to know about one bound type T. It
// works on K, the part of a bound that is compared and constrained, so the
// bound's other fields stay where they are.
type boundOrder[T, K any] struct {
	key    func(T) K
	setKey func(*T, K)
	// compare returns a negative number, zero or a positive number as a is
	// below, equal to or above b, and false when a and b do not compare.
	compare func(a, b K) (int, bool)
	// fits reports whether k satisfies the constraint of a side's bound
	// node. A nil node constrains nothing.
	fits func(node *tcimpl.CompiledNode, k K) bool
	// example returns the example value of a side's bound node, and false
	// when the node has none.
	example func(node *tcimpl.CompiledNode) (K, bool)
}

func orderBounds[T, K any](optNode *tcimpl.CompiledNode, iv *rm.Interval[T], o boundOrder[T, K]) {
	if iv.LowerUnbounded || iv.UpperUnbounded {
		return
	}
	lower, upper := o.key(iv.Lower), o.key(iv.Upper)
	if c, ok := o.compare(lower, upper); !ok || c <= 0 {
		return
	}
	lowerNode, upperNode := boundNode(optNode, "lower"), boundNode(optNode, "upper")
	candidates := [][2]K{{upper, lower}}
	exLower, okLower := o.example(lowerNode)
	exUpper, okUpper := o.example(upperNode)
	if okLower && okUpper {
		candidates = append(candidates, [2]K{exLower, exUpper})
	}
	if okLower {
		candidates = append(candidates, [2]K{exLower, exLower})
	}
	if okUpper {
		candidates = append(candidates, [2]K{exUpper, exUpper})
	}
	candidates = append(candidates, [2]K{lower, lower}, [2]K{upper, upper})
	for _, pair := range candidates {
		c, ok := o.compare(pair[0], pair[1])
		if !ok || c > 0 || !o.fits(lowerNode, pair[0]) || !o.fits(upperNode, pair[1]) {
			continue
		}
		o.setKey(&iv.Lower, pair[0])
		o.setKey(&iv.Upper, pair[1])
		return
	}
}

// boundNode returns the OPT node the generator built an interval's lower or
// upper bound from.
func boundNode(optNode *tcimpl.CompiledNode, side string) *tcimpl.CompiledNode {
	return firstChild(optNode, side)
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

// primitiveOf returns the primitive constraint on node's attrName, taken
// from the attribute's first child, or node's own primitive constraint
// when attrName is empty. It returns nil when there is none.
func primitiveOf(node *tcimpl.CompiledNode, attrName string) constraints.PrimitiveConstraint {
	if attrName != "" {
		node = firstChild(node, attrName)
	}
	if node == nil {
		return nil
	}
	return node.PrimitiveConstraint()
}

// countOrder orders DV_COUNT bounds by magnitude, which the OPT constrains
// through the bound's magnitude attribute.
var countOrder = boundOrder[rm.DVCount, int64]{
	key:    func(c rm.DVCount) int64 { return c.Magnitude },
	setKey: func(c *rm.DVCount, m int64) { c.Magnitude = m },
	compare: func(a, b int64) (int, bool) {
		switch {
		case a < b:
			return -1, true
		case a > b:
			return 1, true
		}
		return 0, true
	},
	fits: func(node *tcimpl.CompiledNode, m int64) bool {
		pc := primitiveOf(node, "magnitude")
		return pc == nil || len(pc.Validate(m)) == 0
	},
	example: func(node *tcimpl.CompiledNode) (int64, bool) {
		pc := primitiveOf(node, "magnitude")
		if pc == nil {
			return 0, false
		}
		return rm.AsInt64(pc.ExampleValue())
	},
}

// quantityKey is the part of a DV_QUANTITY bound that is compared and
// constrained.
type quantityKey struct {
	magnitude float64
	units     string
}

// quantityOrder orders DV_QUANTITY bounds by magnitude when their units
// match. The OPT constrains a DV_QUANTITY bound on the bound node itself.
var quantityOrder = boundOrder[rm.DVQuantity, quantityKey]{
	key: func(q rm.DVQuantity) quantityKey {
		return quantityKey{magnitude: float64(q.Magnitude), units: q.Units}
	},
	setKey: func(q *rm.DVQuantity, k quantityKey) {
		q.Magnitude = rm.Real(k.magnitude)
		q.Units = k.units
	},
	compare: func(a, b quantityKey) (int, bool) {
		if a.units != b.units {
			return 0, false
		}
		switch {
		case a.magnitude < b.magnitude:
			return -1, true
		case a.magnitude > b.magnitude:
			return 1, true
		}
		return 0, true
	},
	fits: func(node *tcimpl.CompiledNode, k quantityKey) bool {
		pc := primitiveOf(node, "")
		v := constraints.QuantityValue{Magnitude: k.magnitude, Units: k.units, Precision: -1}
		return pc == nil || len(pc.Validate(v)) == 0
	},
	example: func(node *tcimpl.CompiledNode) (quantityKey, bool) {
		pc := primitiveOf(node, "")
		if pc == nil {
			return quantityKey{}, false
		}
		ex, ok := pc.ExampleValue().(constraints.QuantityValue)
		if !ok {
			return quantityKey{}, false
		}
		return quantityKey{magnitude: ex.Magnitude, units: ex.Units}, true
	},
}
