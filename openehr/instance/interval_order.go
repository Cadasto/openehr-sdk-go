package instance

// interval_order.go: keeps the two bounds of a generated interval in order.

import (
	"math"
	"strings"

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
// It orders DV_COUNT, DV_QUANTITY, DV_DATE, DV_TIME, DV_DATE_TIME,
// DV_PROPORTION and DV_ORDINAL bounds, in a DV_INTERVAL of that type and
// in a DV_INTERVAL<DV_ORDERED> whose two bounds are the same one of those
// types. Quantities are ordered only when both bounds share their units,
// since magnitudes in different units do not compare. Proportions are
// ordered by the ratio numerator/denominator, and only when both bounds
// share their type (proportionScale). Dates, times and date-times are
// ordered as the ISO-8601 strings the generator writes. A pair that does
// not compare is left alone.
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
		orderConcrete(&iv.Interval, optNode, countScale,
			func(b rm.DVCount) int64 { return b.Magnitude },
			func(b *rm.DVCount, m int64) { b.Magnitude = m })
	case *rm.DVInterval[rm.DVQuantity]:
		orderConcrete(&iv.Interval, optNode, quantityScale, quantityKeyOf, setQuantityKey)
	case *rm.DVInterval[rm.DVDate]:
		orderConcrete(&iv.Interval, optNode, isoScale,
			func(b rm.DVDate) string { return b.Value },
			func(b *rm.DVDate, v string) { b.Value = v })
	case *rm.DVInterval[rm.DVTime]:
		orderConcrete(&iv.Interval, optNode, isoScale,
			func(b rm.DVTime) string { return b.Value },
			func(b *rm.DVTime, v string) { b.Value = v })
	case *rm.DVInterval[rm.DVDateTime]:
		orderConcrete(&iv.Interval, optNode, isoScale,
			func(b rm.DVDateTime) string { return b.Value },
			func(b *rm.DVDateTime, v string) { b.Value = v })
	case *rm.DVInterval[rm.DVProportion]:
		orderConcrete(&iv.Interval, optNode, proportionScale, proportionKeyOf, setProportionKey)
	case *rm.DVInterval[rm.DVOrdinal]:
		orderConcrete(&iv.Interval, optNode, ordinalScale, ordinalKeyOf, setOrdinalKey)
	case *rm.DVInterval[rm.DVOrdered]:
		if bothBounded(&iv.Interval) {
			orderOrderedBounds(optNode, &iv.Interval)
		}
	}
}

// orderConcrete orders one concrete DV_INTERVAL<T> with scale.
func orderConcrete[T any, K any](iv *rm.Interval[T], optNode *tcimpl.CompiledNode, s scale[K], key func(T) K, apply func(*T, K)) {
	if !bothBounded(iv) {
		return
	}
	lower, upper := boundNodes(optNode)
	if lo, hi, ok := orderPair(key(iv.Lower), key(iv.Upper), lower, upper, s); ok {
		apply(&iv.Lower, lo)
		apply(&iv.Upper, hi)
	}
}

// orderOrderedBounds orders a DV_INTERVAL<DV_ORDERED> whose two bounds are
// the same comparable type, in value or pointer form. A pointer bound is
// changed in place and a value bound is replaced, so each keeps its form.
// A mixed or missing pair is left alone.
func orderOrderedBounds(optNode *tcimpl.CompiledNode, iv *rm.Interval[rm.DVOrdered]) {
	if orderOrderedAs(iv, optNode, countScale, asCount, func(c rm.DVCount) int64 { return c.Magnitude }, setOrderedCount) {
		return
	}
	if orderOrderedAs(iv, optNode, quantityScale, asQuantity, quantityKeyOf, setOrderedQuantity) {
		return
	}
	if orderOrderedAs(iv, optNode, isoScale, asDate, func(d rm.DVDate) string { return d.Value }, setOrderedDate) {
		return
	}
	if orderOrderedAs(iv, optNode, isoScale, asTime, func(d rm.DVTime) string { return d.Value }, setOrderedTime) {
		return
	}
	if orderOrderedAs(iv, optNode, isoScale, asDateTime, func(d rm.DVDateTime) string { return d.Value }, setOrderedDateTime) {
		return
	}
	if orderOrderedAs(iv, optNode, proportionScale, asProportion, proportionKeyOf, setOrderedProportion) {
		return
	}
	orderOrderedAs(iv, optNode, ordinalScale, asOrdinal, ordinalKeyOf, setOrderedOrdinal)
}

// orderOrderedAs orders iv when both bounds are T. It reports whether
// they were, including when the pair was already in order or could not
// be repaired.
func orderOrderedAs[T any, K any](
	iv *rm.Interval[rm.DVOrdered],
	optNode *tcimpl.CompiledNode,
	s scale[K],
	read func(rm.DVOrdered) (T, bool),
	key func(T) K,
	write func(*rm.DVOrdered, K),
) bool {
	lowerValue, ok := read(iv.Lower)
	if !ok {
		return false
	}
	upperValue, ok := read(iv.Upper)
	if !ok {
		return false
	}
	lower, upper := boundNodes(optNode)
	if lo, hi, ok := orderPair(key(lowerValue), key(upperValue), lower, upper, s); ok {
		write(&iv.Lower, lo)
		write(&iv.Upper, hi)
	}
	return true
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

// isoScale orders DV_DATE, DV_TIME and DV_DATE_TIME bounds by the
// ISO-8601 strings the generator writes. Those strings are fixed-width,
// so their lexicographic order is their chronological order. A C_DATE,
// C_TIME or C_DATE_TIME has no numeric end, so only a swap repairs an
// inversion.
var isoScale = scale[string]{
	compare: func(a, b string) (int, bool) { return strings.Compare(a, b), true },
	fits: func(node *tcimpl.CompiledNode, v string) bool {
		pc := primitiveOf(firstChild(node, "value"))
		return pc == nil || len(pc.Validate(v)) == 0
	},
	lowest:  func(*tcimpl.CompiledNode, string) (string, bool) { return "", false },
	highest: func(*tcimpl.CompiledNode, string) (string, bool) { return "", false },
}

// proportionKey is the part of a DV_PROPORTION bound that is compared.
// DV_PROPORTION.magnitude and less_than are not implemented on the
// generated type. The RM states the magnitude as the ratio
// numerator/denominator, and is_strictly_comparable_to as "the same
// type", so two bounds compare only when their types match and neither
// denominator is zero.
type proportionKey struct {
	numerator   float64
	denominator float64
	kind        int64
}

func proportionKeyOf(p rm.DVProportion) proportionKey {
	return proportionKey{numerator: float64(p.Numerator), denominator: float64(p.Denominator), kind: int64(p.Type)}
}

func setProportionKey(p *rm.DVProportion, k proportionKey) {
	p.Numerator = rm.Real(k.numerator)
	p.Denominator = rm.Real(k.denominator)
	p.Type = rm.Integer(k.kind)
}

// proportionScale orders DV_PROPORTION bounds by numerator/denominator
// when their types match. The extreme replacement keeps the bound's type
// and denominator and moves the numerator to the end its C_REAL accepts,
// which is the low or high ratio while that denominator stays positive.
// A negative denominator reverses which numerator end is the low ratio.
var proportionScale = scale[proportionKey]{
	compare: func(a, b proportionKey) (int, bool) {
		if a.kind != b.kind {
			return 0, false
		}
		ra, aOK := proportionRatio(a)
		rb, bOK := proportionRatio(b)
		if !aOK || !bOK {
			return 0, false
		}
		return compareOrdered(ra, rb), true
	},
	fits: proportionFits,
	lowest: func(node *tcimpl.CompiledNode, like proportionKey) (proportionKey, bool) {
		return proportionEnd(node, like, true)
	},
	highest: func(node *tcimpl.CompiledNode, like proportionKey) (proportionKey, bool) {
		return proportionEnd(node, like, false)
	},
}

func proportionRatio(k proportionKey) (float64, bool) {
	if k.denominator == 0 || math.IsNaN(k.numerator) || math.IsNaN(k.denominator) {
		return 0, false
	}
	return k.numerator / k.denominator, true
}

func proportionFits(node *tcimpl.CompiledNode, k proportionKey) bool {
	return componentFits(firstChild(node, "numerator"), k.numerator) &&
		componentFits(firstChild(node, "denominator"), k.denominator) &&
		componentFits(firstChild(node, "type"), k.kind)
}

func componentFits(node *tcimpl.CompiledNode, v any) bool {
	pc := primitiveOf(node)
	return pc == nil || len(pc.Validate(v)) == 0
}

// proportionEnd returns the lowest (low) or highest ratio a side accepts
// while keeping like's type and denominator. It returns false when the
// numerator constraint has no end in the direction that moves the ratio
// that way, or when like itself does not fit.
func proportionEnd(node *tcimpl.CompiledNode, like proportionKey, low bool) (proportionKey, bool) {
	if like.denominator == 0 || !componentFits(firstChild(node, "type"), like.kind) || !componentFits(firstChild(node, "denominator"), like.denominator) {
		return proportionKey{}, false
	}
	numLow := low
	if like.denominator < 0 {
		numLow = !low
	}
	n, ok := realEnd(firstChild(node, "numerator"), numLow)
	if !ok {
		return proportionKey{}, false
	}
	return proportionKey{numerator: n, denominator: like.denominator, kind: like.kind}, true
}

// realEnd returns the lowest (low) or highest value a C_REAL accepts:
// the extreme list entry the range also admits, or the range's own end.
// At an exclusive end it is the nearest float64 inside the range. It
// returns false when there is no such constraint, or when the constraint
// has no end in that direction.
func realEnd(node *tcimpl.CompiledNode, low bool) (float64, bool) {
	c, ok := primitiveOf(node).(constraints.CReal)
	if !ok {
		return 0, false
	}
	if len(c.List) > 0 {
		var end float64
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
		m := r.Lower
		if !r.LowerInclusive {
			m = math.Nextafter(m, math.Inf(1))
		}
		return m, true
	case !low && !r.UpperUnbounded:
		m := r.Upper
		if !r.UpperInclusive {
			m = math.Nextafter(m, math.Inf(-1))
		}
		return m, true
	}
	return 0, false
}

// ordinalKey is the part of a DV_ORDINAL bound that is compared and
// constrained. Value is the order; the symbol travels with it.
type ordinalKey struct {
	value  int64
	symbol rm.DVCodedText
}

func ordinalKeyOf(o rm.DVOrdinal) ordinalKey {
	return ordinalKey{value: int64(o.Value), symbol: o.Symbol}
}

func setOrdinalKey(o *rm.DVOrdinal, k ordinalKey) {
	o.Value = rm.Integer(k.value)
	o.Symbol = k.symbol
}

// ordinalScale orders DV_ORDINAL bounds by Value. The OPT constrains a
// DV_ORDINAL bound on the bound node itself.
var ordinalScale = scale[ordinalKey]{
	compare: func(a, b ordinalKey) (int, bool) { return compareOrdered(a.value, b.value), true },
	fits: func(node *tcimpl.CompiledNode, k ordinalKey) bool {
		pc := primitiveOf(node)
		return pc == nil || len(pc.Validate(int(k.value))) == 0
	},
	lowest: func(node *tcimpl.CompiledNode, _ ordinalKey) (ordinalKey, bool) {
		return ordinalEnd(node, true)
	},
	highest: func(node *tcimpl.CompiledNode, _ ordinalKey) (ordinalKey, bool) {
		return ordinalEnd(node, false)
	},
}

func ordinalEnd(node *tcimpl.CompiledNode, low bool) (ordinalKey, bool) {
	c, ok := primitiveOf(node).(constraints.CDvOrdinal)
	if !ok || len(c.Values) == 0 {
		return ordinalKey{}, false
	}
	var best constraints.OrdinalSymbol
	found := false
	for _, v := range c.Values {
		if len(c.Validate(v.Value)) != 0 {
			continue
		}
		if !found || (low && v.Value < best.Value) || (!low && v.Value > best.Value) {
			best, found = v, true
		}
	}
	if !found {
		return ordinalKey{}, false
	}
	return ordinalKey{value: int64(best.Value), symbol: ordinalSymbolText(best.Symbol)}, true
}

func asDate(v rm.DVOrdered) (rm.DVDate, bool) {
	switch x := v.(type) {
	case rm.DVDate:
		return x, true
	case *rm.DVDate:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVDate{}, false
}

func asTime(v rm.DVOrdered) (rm.DVTime, bool) {
	switch x := v.(type) {
	case rm.DVTime:
		return x, true
	case *rm.DVTime:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVTime{}, false
}

func asDateTime(v rm.DVOrdered) (rm.DVDateTime, bool) {
	switch x := v.(type) {
	case rm.DVDateTime:
		return x, true
	case *rm.DVDateTime:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVDateTime{}, false
}

func asProportion(v rm.DVOrdered) (rm.DVProportion, bool) {
	switch x := v.(type) {
	case rm.DVProportion:
		return x, true
	case *rm.DVProportion:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVProportion{}, false
}

func asOrdinal(v rm.DVOrdered) (rm.DVOrdinal, bool) {
	switch x := v.(type) {
	case rm.DVOrdinal:
		return x, true
	case *rm.DVOrdinal:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVOrdinal{}, false
}

func setOrderedDate(slot *rm.DVOrdered, value string) {
	switch x := (*slot).(type) {
	case *rm.DVDate:
		x.Value = value
	case rm.DVDate:
		x.Value = value
		*slot = x
	}
}

func setOrderedTime(slot *rm.DVOrdered, value string) {
	switch x := (*slot).(type) {
	case *rm.DVTime:
		x.Value = value
	case rm.DVTime:
		x.Value = value
		*slot = x
	}
}

func setOrderedDateTime(slot *rm.DVOrdered, value string) {
	switch x := (*slot).(type) {
	case *rm.DVDateTime:
		x.Value = value
	case rm.DVDateTime:
		x.Value = value
		*slot = x
	}
}

func setOrderedProportion(slot *rm.DVOrdered, k proportionKey) {
	switch x := (*slot).(type) {
	case *rm.DVProportion:
		setProportionKey(x, k)
	case rm.DVProportion:
		setProportionKey(&x, k)
		*slot = x
	}
}

func setOrderedOrdinal(slot *rm.DVOrdered, k ordinalKey) {
	switch x := (*slot).(type) {
	case *rm.DVOrdinal:
		setOrdinalKey(x, k)
	case rm.DVOrdinal:
		setOrdinalKey(&x, k)
		*slot = x
	}
}

// sharedQuantityUnit returns one unit both DV_QUANTITY bounds of an
// interval can use, drawn from the intersection of their unit lists.
// An empty intersection, or a side that is not a DV_QUANTITY constraint,
// returns false so each side draws its own unit.
func sharedQuantityUnit(node *tcimpl.CompiledNode, s sampler) (string, bool) {
	if node == nil || !strings.HasPrefix(node.RMTypeName(), "DV_INTERVAL") {
		return "", false
	}
	lowerNode, upperNode := boundNodes(node)
	lower, lok := primitiveOf(lowerNode).(constraints.DvQuantity)
	upper, uok := primitiveOf(upperNode).(constraints.DvQuantity)
	if !lok || !uok {
		return "", false
	}
	allowed := make(map[string]struct{}, len(upper.Units))
	for _, u := range upper.Units {
		allowed[u.Units] = struct{}{}
	}
	var shared []string
	seen := make(map[string]struct{}, len(lower.Units))
	for _, u := range lower.Units {
		if _, ok := allowed[u.Units]; !ok {
			continue
		}
		if _, dup := seen[u.Units]; dup {
			continue
		}
		seen[u.Units] = struct{}{}
		shared = append(shared, u.Units)
	}
	if len(shared) == 0 {
		return "", false
	}
	return shared[s.intN(len(shared))], true
}
