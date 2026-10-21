package instance

// interval_order.go: keeps the two bounds of a generated interval in order.

import (
	"math"
	"strings"
	"time"

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
// share their type (proportionScale). Date-times and times that carry a
// zone are ordered as instants, and other dates, times and date-times as
// text when both share one zoneless layout (temporalCompare). A pair that
// does not compare is left alone.
//
// An inverted pair is replaced by the first of these two pairs that is in
// order and fits each side's OPT constraint:
//
//   - the two bounds swapped, which keeps the values RandomFill drew;
//   - the lowest value the lower side's constraint accepts, with the
//     highest value the upper side's constraint accepts. A side whose
//     constraint has no end in that direction takes the other side's
//     current bound instead, which a range open that way accepts; a
//     proportion side that has no usable end there, whether its range is
//     open or its type, denominator or precision rules the end out, takes
//     that bound's ratio on its own denominator (proportionAdopt). At an exclusive real end the accepted extreme is
//     the nearest float64 inside it, and for a proportion that needs whole
//     numbers the nearest whole number it accepts.
//
// In a swap a proportion bound's precision moves with its numerator and
// denominator, which were settled under it. In the second pair each
// proportion bound keeps its type and denominator and takes a precision
// its own side admits, one that needs no more whole numbers than its type
// does wherever the side admits one (proportionPrecision), so its
// numerator is rounded only where its type, or every precision its side
// admits, needs whole numbers. A pair fits a side only where that side's
// constraint on precision admits the precision of its bound, so every
// bound written keeps the whole numbers its precision needs. An ordinal
// bound the second pair replaces takes the symbol its side lists for the
// new value, with the text the nearest archetype root's term gives a code
// in any terminology other than openehr (useTermText), as the walk writes
// it.
//
// The OPT may constrain a quantity or an ordinal bound as one C_DV_QUANTITY
// or C_DV_ORDINAL on the bound node, or, where it builds the bound as a
// C_COMPLEX_OBJECT, on the nodes of its attributes: the magnitude and the
// units of a quantity, the value and the symbol of an ordinal. Both are
// read (quantityFits, ordinalFits). An ordinal bound built the second way
// holds its symbol apart from its value, so the second pair keeps each
// side's own symbol and changes only its value.
//
// A bound that carries a normal_range and a normal_status was settled by
// its own walk (settleNormalRange), and each pair keeps it so. A value a
// pair gives such a side stays where its bound keeps the rule at that value,
// and else is narrowed as settleNormalRange narrows a bound: to the lowest
// value that keeps the rule for the lower side, or the highest for the upper
// side, among the candidates of the rule that the side's constraint accepts
// (normalAt). A pair in which a side has no such value is not one that fits.
// A bound that does not compare with its normal_range keeps any value, as
// settleNormalRange leaves it as generated.
//
// So for a count, a quantity, an ordinal or a proportion the second pair
// holds each side's extreme, over its precision too for a proportion, and
// on a side with no end that way a value at the other side's current
// bound, which the other side's extreme does not pass; on a side whose bound
// keeps a normal_range rule it is the extreme among the values that keep
// the rule. An in-order pair therefore exists only if the second pair is in
// order: for a proportion, among the pairs that keep each bound's type and
// denominator. When it is not, the two sides' constraints admit no such
// ordered interval. The
// generator has no error for a constraint it cannot satisfy (sampleValue,
// for one, falls back to the example value without a signal), so the
// bounds are then left as they are, and the interval breaks
// Limits_consistent.
func orderIntervalBounds(optNode *tcimpl.CompiledNode, rmValue any, lang string) {
	switch iv := rmValue.(type) {
	case *rm.DVInterval[rm.DVCount]:
		orderConcrete(&iv.Interval, optNode, countScale, countKeyOf, setCountKey)
	case *rm.DVInterval[rm.DVQuantity]:
		orderConcrete(&iv.Interval, optNode, quantityScale, quantityKeyOf, setQuantityKey)
	case *rm.DVInterval[rm.DVDate]:
		orderConcrete(&iv.Interval, optNode, dateScale, dateKeyOf, setDateKey)
	case *rm.DVInterval[rm.DVTime]:
		orderConcrete(&iv.Interval, optNode, timeScale, timeKeyOf, setTimeKey)
	case *rm.DVInterval[rm.DVDateTime]:
		orderConcrete(&iv.Interval, optNode, dateTimeScale, dateTimeKeyOf, setDateTimeKey)
	case *rm.DVInterval[rm.DVProportion]:
		orderConcrete(&iv.Interval, optNode, proportionScale, proportionKeyOf, setProportionKey)
	case *rm.DVInterval[rm.DVOrdinal]:
		orderConcrete(&iv.Interval, optNode, ordinalScaleIn(lang), ordinalKeyOf, setOrdinalKey)
	case *rm.DVInterval[rm.DVOrdered]:
		if bothBounded(&iv.Interval) {
			orderOrderedBounds(optNode, &iv.Interval, lang)
		}
	}
}

// orderConcrete orders one concrete DV_INTERVAL<T> with scale.
func orderConcrete[T any, K any](iv *rm.Interval[T], optNode *tcimpl.CompiledNode, s scale[K], key func(T) K, apply func(*T, K)) {
	if !bothBounded(iv) {
		return
	}
	lowerNode, upperNode := boundNodes(optNode)
	lower := orderSide[K]{node: lowerNode, narrow: normalAt(iv.Lower, lowerNode, key, apply, true)}
	upper := orderSide[K]{node: upperNode, narrow: normalAt(iv.Upper, upperNode, key, apply, false)}
	if lo, hi, ok := orderPair(key(iv.Lower), key(iv.Upper), lower, upper, s); ok {
		apply(&iv.Lower, lo)
		apply(&iv.Upper, hi)
	}
}

// orderOrderedBounds orders a DV_INTERVAL<DV_ORDERED> whose two bounds are
// the same comparable type, in value or pointer form. A pointer bound is
// changed in place and a value bound is replaced, so each keeps its form.
// A mixed or missing pair is left alone. lang is the language of an
// ordinal symbol's text.
func orderOrderedBounds(optNode *tcimpl.CompiledNode, iv *rm.Interval[rm.DVOrdered], lang string) {
	if orderOrderedAs(iv, optNode, countScale, asCount, countKeyOf, setCountKey, setOrderedCount) {
		return
	}
	if orderOrderedAs(iv, optNode, quantityScale, asQuantity, quantityKeyOf, setQuantityKey, setOrderedQuantity) {
		return
	}
	if orderOrderedAs(iv, optNode, dateScale, asDate, dateKeyOf, setDateKey, setOrderedDate) {
		return
	}
	if orderOrderedAs(iv, optNode, timeScale, asTime, timeKeyOf, setTimeKey, setOrderedTime) {
		return
	}
	if orderOrderedAs(iv, optNode, dateTimeScale, asDateTime, dateTimeKeyOf, setDateTimeKey, setOrderedDateTime) {
		return
	}
	if orderOrderedAs(iv, optNode, proportionScale, asProportion, proportionKeyOf, setProportionKey, setOrderedProportion) {
		return
	}
	orderOrderedAs(iv, optNode, ordinalScaleIn(lang), asOrdinal, ordinalKeyOf, setOrdinalKey, setOrderedOrdinal)
}

// orderOrderedAs orders iv when both bounds are T. It reports whether
// they were, including when the pair was already in order or could not
// be repaired. A bound is read as a copy of its T (read, key), which the
// bound's own normal_range rule is tried on (apply); write puts a key on the
// bound itself.
func orderOrderedAs[T any, K any](
	iv *rm.Interval[rm.DVOrdered],
	optNode *tcimpl.CompiledNode,
	s scale[K],
	read func(rm.DVOrdered) (T, bool),
	key func(T) K,
	apply func(*T, K),
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
	lowerNode, upperNode := boundNodes(optNode)
	lower := orderSide[K]{node: lowerNode, narrow: normalAt(lowerValue, lowerNode, key, apply, true)}
	upper := orderSide[K]{node: upperNode, narrow: normalAt(upperValue, upperNode, key, apply, false)}
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
	// adopt returns the value a side takes where lowest (low) or highest
	// gave none: one at other, the other side's current bound, where like
	// is the side's own bound. It chooses only what a value may choose for
	// itself, as proportionAdopt does a precision its side admits; whether
	// the side's bound node accepts the rest is for fits. It returns false
	// when there is none. A nil adopt takes other as it is.
	adopt func(node *tcimpl.CompiledNode, like, other K, low bool) (K, bool)
}

// adopted is s.adopt, or other where s has no adopt.
func (s scale[K]) adopted(node *tcimpl.CompiledNode, like, other K, low bool) (K, bool) {
	if s.adopt == nil {
		return other, true
	}
	return s.adopt(node, like, other, low)
}

// orderSide is one side of an interval that orderPair orders.
type orderSide[K any] struct {
	// node is the OPT node the side's bound is built from, nil for a bound
	// the OPT does not constrain.
	node *tcimpl.CompiledNode
	// narrow returns k as the bound has it once it keeps its own normal_range
	// rule (normalAt), and false where no value does. A nil narrow keeps k.
	narrow func(k K) (K, bool)
}

// narrowed is s.narrow, or k where s has no narrow.
func (s orderSide[K]) narrowed(k K) (K, bool) {
	if s.narrow == nil {
		return k, true
	}
	return s.narrow(k)
}

// orderPair returns an in-order replacement for the inverted pair
// (lower, upper), as orderIntervalBounds describes, and false when the pair
// is in order already, does not compare, or has no in-order replacement.
// Each value a pair gives a side is first narrowed to keep that side's own
// normal_range rule (orderSide.narrow), and a pair in which a side has no
// such value is not one that fits.
func orderPair[K any](lower, upper K, lowerSide, upperSide orderSide[K], s scale[K]) (K, K, bool) {
	if c, ok := s.compare(lower, upper); !ok || c <= 0 {
		return lower, upper, false
	}
	// fitsInOrder returns the pair as each side keeps it, and whether that
	// pair is in order and fits each side's OPT constraint.
	fitsInOrder := func(lo, hi K) (K, K, bool) {
		lo, loOK := lowerSide.narrowed(lo)
		hi, hiOK := upperSide.narrowed(hi)
		if !loOK || !hiOK {
			return lo, hi, false
		}
		c, ok := s.compare(lo, hi)
		return lo, hi, ok && c <= 0 && s.fits(lowerSide.node, lo) && s.fits(upperSide.node, hi)
	}
	if lo, hi, ok := fitsInOrder(upper, lower); ok {
		return lo, hi, true
	}
	lo, loOK := s.lowest(lowerSide.node, lower)
	if !loOK {
		lo, loOK = s.adopted(lowerSide.node, lower, upper, true)
	}
	hi, hiOK := s.highest(upperSide.node, upper)
	if !hiOK {
		hi, hiOK = s.adopted(upperSide.node, upper, lower, false)
	}
	if loOK && hiOK {
		if lo, hi, ok := fitsInOrder(lo, hi); ok {
			return lo, hi, true
		}
	}
	return lower, upper, false
}

// boundNodes returns the OPT nodes the generator built an interval's lower
// and upper bounds from, nil for a bound the OPT does not constrain.
func boundNodes(optNode *tcimpl.CompiledNode) (lower, upper *tcimpl.CompiledNode) {
	return bindingChild(optNode, "lower"), bindingChild(optNode, "upper")
}

// bindingChild returns the alternative of node's attrName whose
// constraints materialiseSingle builds under ([bindingAlternative]),
// among those the OPT does not prohibit, or nil when node or the
// attribute has none. A pass after the walk reads the constraints the
// walk built under, so a default it writes passes the child the
// validator binds.
func bindingChild(node *tcimpl.CompiledNode, attrName string) *tcimpl.CompiledNode {
	if node == nil {
		return nil
	}
	attr := node.Attribute(attrName)
	if attr == nil {
		return nil
	}
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return nil
	}
	return bindingAlternative(children, builtRMType(children[0]))
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
		pc := primitiveOf(bindingChild(node, "magnitude"))
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
// DV_COUNT bound node's magnitude accepts (integerEndOf).
func integerEnd(node *tcimpl.CompiledNode, low bool) (int64, bool) {
	return integerEndOf(bindingChild(node, "magnitude"), low)
}

// integerEndOf returns the lowest (low) or highest value the C_INTEGER of
// node accepts: the extreme list entry the range also admits, or the
// range's own end. It returns false when there is no such constraint, or
// when the constraint has no end in that direction.
func integerEndOf(node *tcimpl.CompiledNode, low bool) (int64, bool) {
	c, ok := primitiveOf(node).(constraints.CInteger)
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
// match. The OPT constrains a DV_QUANTITY bound as one C_DV_QUANTITY on the
// bound node itself, or on the nodes of its magnitude and units attributes
// where it builds the bound as a C_COMPLEX_OBJECT. Neither writes the
// bound's precision, so each bound keeps the precision its own side
// admitted.
var quantityScale = scale[quantityKey]{
	compare: func(a, b quantityKey) (int, bool) {
		if a.units != b.units {
			return 0, false
		}
		return compareOrdered(a.magnitude, b.magnitude), true
	},
	fits: quantityFits,
	lowest: func(node *tcimpl.CompiledNode, like quantityKey) (quantityKey, bool) {
		return magnitudeEnd(node, like.units, true)
	},
	highest: func(node *tcimpl.CompiledNode, like quantityKey) (quantityKey, bool) {
		return magnitudeEnd(node, like.units, false)
	},
}

// quantityFits reports whether k suits a side's bound node: the C_DV_QUANTITY
// on the node accepts it, or, where the node is a C_COMPLEX_OBJECT, the
// constraint on each of its magnitude and units attributes does. A nil node
// constrains nothing.
func quantityFits(node *tcimpl.CompiledNode, k quantityKey) bool {
	if pc := primitiveOf(node); pc != nil {
		v := constraints.QuantityValue{Magnitude: k.magnitude, Units: k.units, Precision: -1}
		return len(pc.Validate(v)) == 0
	}
	return componentFits(bindingChild(node, "magnitude"), k.magnitude) &&
		componentFits(bindingChild(node, "units"), k.units)
}

// magnitudeEnd returns the lowest (low) or highest magnitude a DV_QUANTITY
// bound node accepts in units: the end of the magnitude range of the first
// entry for those units, which is the entry DvQuantity.Validate checks. At
// an exclusive end it is the nearest float64 inside the range. It returns
// false when there is no such entry, or when its range has no end in that
// direction. Where the node is a C_COMPLEX_OBJECT, the end is that of the
// C_REAL on its magnitude (realEnd), and there is none unless the C_STRING
// on its units accepts units.
func magnitudeEnd(node *tcimpl.CompiledNode, units string, low bool) (quantityKey, bool) {
	c, ok := primitiveOf(node).(constraints.DvQuantity)
	if !ok {
		if !componentFits(bindingChild(node, "units"), units) {
			return quantityKey{}, false
		}
		m, ok := realEnd(bindingChild(node, "magnitude"), low)
		if !ok {
			return quantityKey{}, false
		}
		return quantityKey{magnitude: m, units: units}, true
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

func countKeyOf(c rm.DVCount) int64 { return int64(c.Magnitude) }

func setCountKey(c *rm.DVCount, m int64) { c.Magnitude = rm.Integer64(m) }

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
		x.Magnitude = rm.Integer64(m)
	case rm.DVCount:
		x.Magnitude = rm.Integer64(m)
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

// dateScale, timeScale and dateTimeScale order DV_DATE, DV_TIME and
// DV_DATE_TIME bounds (temporalCompare says how). A C_DATE, C_TIME or
// C_DATE_TIME has no numeric end, so only a swap repairs an inversion.
var (
	dateScale     = temporalScale(temporalCompare("", noZone))
	timeScale     = temporalScale(temporalCompare("15:04:05Z07:00", timeZoned))
	dateTimeScale = temporalScale(temporalCompare(time.RFC3339, dateTimeZoned))
)

func dateKeyOf(d rm.DVDate) string { return d.Value }

func setDateKey(d *rm.DVDate, v string) { d.Value = v }

func timeKeyOf(d rm.DVTime) string { return d.Value }

func setTimeKey(d *rm.DVTime, v string) { d.Value = v }

func dateTimeKeyOf(d rm.DVDateTime) string { return d.Value }

func setDateTimeKey(d *rm.DVDateTime, v string) { d.Value = v }

func temporalScale(compare func(a, b string) (int, bool)) scale[string] {
	return scale[string]{
		compare: compare,
		fits: func(node *tcimpl.CompiledNode, v string) bool {
			pc := primitiveOf(bindingChild(node, "value"))
			return pc == nil || len(pc.Validate(v)) == 0
		},
		lowest:  func(*tcimpl.CompiledNode, string) (string, bool) { return "", false },
		highest: func(*tcimpl.CompiledNode, string) (string, bool) { return "", false },
	}
}

// temporalCompare compares two ISO 8601 values. When both parse under
// layout, which carries a zone, they compare as instants. Otherwise they
// compare as text, which is their chronological order only when both have
// the same layout and no zone: then they are the same fixed-width shape.
// Any other pair, such as a zoned value and a zoneless one, or a full date
// and a partial one, does not compare. An empty layout skips the instant
// step.
func temporalCompare(layout string, zoned func(string) bool) func(a, b string) (int, bool) {
	return func(a, b string) (int, bool) {
		if layout != "" {
			ta, errA := time.Parse(layout, a)
			tb, errB := time.Parse(layout, b)
			if errA == nil && errB == nil {
				return ta.Compare(tb), true
			}
		}
		if zoned(a) || zoned(b) || isoShape(a) != isoShape(b) {
			return 0, false
		}
		return strings.Compare(a, b), true
	}
}

func noZone(string) bool { return false }

// timeZoned reports whether an ISO 8601 time carries a zone: Z or an
// offset.
func timeZoned(s string) bool { return strings.ContainsAny(s, "Z+-") }

// dateTimeZoned reports whether an ISO 8601 date-time carries a zone after
// its time part. A value with no time part has none.
func dateTimeZoned(s string) bool {
	_, clock, ok := strings.Cut(s, "T")
	return ok && timeZoned(clock)
}

// isoShape is s with every digit replaced by 0, so two values have the same
// shape exactly when they use the same layout.
func isoShape(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '0'
		}
		return r
	}, s)
}

// proportionKey is the part of a DV_PROPORTION bound that is compared and
// constrained. DV_PROPORTION.magnitude and less_than are not implemented
// on the generated type. The RM states the magnitude as the ratio
// numerator/denominator, and is_strictly_comparable_to as "the same
// type", so two bounds compare only when their types match and neither
// denominator is zero. precision is the bound's precision, nil when it has
// none: it moves with the numerator and the denominator, which were
// settled under it, so a swap cannot put a number that is not whole on a
// bound of precision 0.
type proportionKey struct {
	numerator   float64
	denominator float64
	kind        int64
	precision   *rm.Integer
}

func proportionKeyOf(p rm.DVProportion) proportionKey {
	k := proportionKey{
		numerator:   float64(p.Numerator),
		denominator: float64(p.Denominator),
		kind:        int64(p.Type),
	}
	if p.Precision != nil {
		k.precision = new(*p.Precision)
	}
	return k
}

// setProportionKey writes k onto p, with a precision of its own, so two
// bounds never share one.
func setProportionKey(p *rm.DVProportion, k proportionKey) {
	p.Numerator = rm.Real(k.numerator)
	p.Denominator = rm.Real(k.denominator)
	p.Type = rm.Integer(k.kind)
	p.Precision = nil
	if k.precision != nil {
		p.Precision = new(*k.precision)
	}
}

// proportionScale orders DV_PROPORTION bounds by numerator/denominator
// when their types match. The extreme replacement keeps the bound's type
// and denominator, takes a precision its side admits (proportionPrecision)
// and moves the numerator to the end its C_REAL accepts, which is the low
// or high ratio while that denominator stays positive. A negative
// denominator reverses which numerator end is the low ratio. A bound that
// needs whole numbers, as a fraction or a bound of precision 0 does, takes
// the whole number nearest that end its C_REAL accepts, so the
// replacement keeps the rules of its kind (settleProportion). Where
// proportionEnd gives a side no bound, for whatever reason, the side takes
// the other bound's ratio on its own type and denominator instead
// (proportionAdopt), kept only where its constraints accept the result.
var proportionScale = scale[proportionKey]{
	compare: compareProportions,
	fits:    proportionFits,
	lowest: func(node *tcimpl.CompiledNode, like proportionKey) (proportionKey, bool) {
		return proportionEnd(node, like, true)
	},
	highest: func(node *tcimpl.CompiledNode, like proportionKey) (proportionKey, bool) {
		return proportionEnd(node, like, false)
	},
	adopt: proportionAdopt,
}

func compareProportions(a, b proportionKey) (int, bool) {
	if a.kind != b.kind {
		return 0, false
	}
	ra, aOK := proportionRatio(a)
	rb, bOK := proportionRatio(b)
	if !aOK || !bOK {
		return 0, false
	}
	return compareOrdered(ra, rb), true
}

func proportionRatio(k proportionKey) (float64, bool) {
	if k.denominator == 0 || math.IsNaN(k.numerator) || math.IsNaN(k.denominator) {
		return 0, false
	}
	return k.numerator / k.denominator, true
}

// proportionFits reports whether k suits a side's bound node: its
// numerator, denominator, type and precision each fit the node's own
// constraint on that attribute.
func proportionFits(node *tcimpl.CompiledNode, k proportionKey) bool {
	return componentFits(bindingChild(node, "numerator"), k.numerator) &&
		componentFits(bindingChild(node, "denominator"), k.denominator) &&
		componentFits(bindingChild(node, "type"), k.kind) &&
		precisionFits(node, k.precision)
}

// precisionFits reports whether a side's bound node admits precision, nil
// for none: no precision where the node does not require one, and a
// precision where the node does not prohibit it and its constraint on
// precision accepts it. A nil node constrains nothing.
func precisionFits(node *tcimpl.CompiledNode, precision *rm.Integer) bool {
	if node == nil {
		return true
	}
	if precision == nil {
		attr := node.Attribute("precision")
		return attr == nil || !isRequired(attr)
	}
	return !prohibited(node, "precision") && componentFits(bindingChild(node, "precision"), int64(*precision))
}

func componentFits(node *tcimpl.CompiledNode, v any) bool {
	pc := primitiveOf(node)
	return pc == nil || len(pc.Validate(v)) == 0
}

// proportionEnd returns the lowest (low) or highest ratio a side accepts
// while keeping like's type and denominator, with the precision
// proportionPrecision gives and a whole numerator where that precision or
// the type needs whole numbers. It returns false when the numerator
// constraint has no end in the direction that moves the ratio that way,
// or no whole number toward that end where one is needed, or when the
// side does not admit like's type or denominator, or admits no precision.
func proportionEnd(node *tcimpl.CompiledNode, like proportionKey, low bool) (proportionKey, bool) {
	if like.denominator == 0 || !componentFits(bindingChild(node, "type"), like.kind) || !componentFits(bindingChild(node, "denominator"), like.denominator) {
		return proportionKey{}, false
	}
	precision, ok := proportionPrecision(node, like)
	if !ok {
		return proportionKey{}, false
	}
	numLow := low
	if like.denominator < 0 {
		numLow = !low
	}
	n, ok := realEndOf(bindingChild(node, "numerator"), numLow, wholeOperands(like.kind, precision))
	if !ok {
		return proportionKey{}, false
	}
	return proportionKey{numerator: n, denominator: like.denominator, kind: like.kind, precision: precision}, true
}

// proportionPrecision returns the precision a bound of like's type takes
// where the second pair of orderIntervalBounds rewrites it on node's
// side, like being the bound that side drew, and false when node admits
// none. It tries like's own precision, none, the members of the C_INTEGER
// on precision, 1 and -1, in that order, and takes the first node admits
// that needs no more whole numbers than the type does, so the numerator
// need not be rounded. Only where node admits no such precision does it
// take the first node admits, which is then 0. It never takes a precision
// below -1, which the RM floor rejects (-1 is no limit).
func proportionPrecision(node *tcimpl.CompiledNode, like proportionKey) (*rm.Integer, bool) {
	candidates := []*rm.Integer{like.precision, nil}
	if c, ok := primitiveOf(bindingChild(node, "precision")).(constraints.CInteger); ok {
		for _, p := range c.List {
			candidates = append(candidates, new(rm.Integer(p)))
		}
	}
	candidates = append(candidates, new(rm.Integer(1)), new(rm.Integer(-1)))
	typeWhole := wholeOperands(like.kind, nil)
	for _, rounded := range []bool{false, true} {
		for _, p := range candidates {
			if p != nil && *p < -1 {
				continue
			}
			if (rounded || wholeOperands(like.kind, p) == typeWhole) && precisionFits(node, p) {
				return p, true
			}
		}
	}
	return nil, false
}

// proportionAdopt returns the bound a side takes at other, the other
// side's current bound, where proportionEnd gave it none, low naming the
// lower side: like's type and denominator, the precision
// proportionPrecision gives, and the numerator that puts its ratio at
// other's. Where that precision or the type needs whole numbers, the
// numerator is the whole number next to that one on the side's own side of
// other, down for the lower side over a positive denominator. float64 can
// leave the ratio just past other's where the two denominators differ, so
// the numerator then steps back toward the side's own side, at most four
// times, until it is not: each time by one float64 step, which is then
// rounded to a whole number where whole numbers are needed. It returns
// false when node admits no precision or the numerator is not a finite
// number. It does not check that the rest suits node, which the caller
// does.
func proportionAdopt(node *tcimpl.CompiledNode, like, other proportionKey, low bool) (proportionKey, bool) {
	precision, ok := proportionPrecision(node, like)
	if !ok {
		return proportionKey{}, false
	}
	k := proportionKey{numerator: other.numerator, denominator: like.denominator, kind: like.kind, precision: precision}
	if like.denominator != other.denominator {
		k.numerator = other.numerator * like.denominator / other.denominator
	}
	// A lower numerator lowers the ratio over a positive denominator.
	round, away := math.Ceil, math.Inf(1)
	if low == (like.denominator > 0) {
		round, away = math.Floor, math.Inf(-1)
	}
	whole := wholeOperands(like.kind, precision)
	if whole {
		k.numerator = round(k.numerator)
	}
	for range 4 {
		if c, ok := compareProportions(k, other); !ok || (low && c <= 0) || (!low && c >= 0) {
			break
		}
		k.numerator = math.Nextafter(k.numerator, away)
		if whole {
			k.numerator = round(k.numerator)
		}
	}
	// A numerator that overflowed float64 is no bound: canonical JSON has no
	// infinity, so the pair stays as generated.
	if math.IsInf(k.numerator, 0) || math.IsNaN(k.numerator) {
		return proportionKey{}, false
	}
	return k, true
}

// realEnd returns the lowest (low) or highest value a C_REAL accepts:
// the extreme list entry the range also admits, or the range's own end.
// At an exclusive end it is the nearest float64 inside the range. It
// returns false when there is no such constraint, or when the constraint
// has no end in that direction.
func realEnd(node *tcimpl.CompiledNode, low bool) (float64, bool) {
	return realEndOf(node, low, false)
}

// realEndOf is realEnd, among whole numbers only where whole says so: the
// extreme whole list entry, or the whole number nearest the range's end
// inside it, up from its low end and down from its high end. It returns
// false where there is no such number.
func realEndOf(node *tcimpl.CompiledNode, low, whole bool) (float64, bool) {
	c, ok := primitiveOf(node).(constraints.CReal)
	if !ok {
		return 0, false
	}
	if len(c.List) > 0 {
		var end float64
		found := false
		for _, n := range c.List {
			if len(c.Validate(n)) != 0 || (whole && math.Trunc(n) != n) {
				continue
			}
			if !found || (low && n < end) || (!low && n > end) {
				end, found = n, true
			}
		}
		return end, found
	}
	r := c.Range
	var m float64
	switch {
	case !r.IsBounded():
		return 0, false
	case low && !r.LowerUnbounded:
		m = r.Lower
		if !r.LowerInclusive {
			m = math.Nextafter(m, math.Inf(1))
		}
	case !low && !r.UpperUnbounded:
		m = r.Upper
		if !r.UpperInclusive {
			m = math.Nextafter(m, math.Inf(-1))
		}
	default:
		return 0, false
	}
	if !whole {
		return m, true
	}
	if low {
		m = math.Ceil(m)
	} else {
		m = math.Floor(m)
	}
	return m, len(c.Validate(m)) == 0
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

// ordinalScaleIn orders DV_ORDINAL bounds by Value. The OPT constrains a
// DV_ORDINAL bound as one C_DV_ORDINAL on the bound node itself, or on the
// nodes of its value and symbol attributes where it builds the bound as a
// C_COMPLEX_OBJECT. lang is the language of the text of a symbol the
// extreme replacement writes (ordinalEnd).
func ordinalScaleIn(lang string) scale[ordinalKey] {
	return scale[ordinalKey]{
		compare: func(a, b ordinalKey) (int, bool) { return compareOrdered(a.value, b.value), true },
		fits:    ordinalFits,
		lowest: func(node *tcimpl.CompiledNode, like ordinalKey) (ordinalKey, bool) {
			return ordinalEnd(node, like, true, lang)
		},
		highest: func(node *tcimpl.CompiledNode, like ordinalKey) (ordinalKey, bool) {
			return ordinalEnd(node, like, false, lang)
		},
		adopt: ordinalAdopt,
	}
}

// ordinalFits reports whether k suits a side's bound node. When the node
// lists (value, symbol) pairs, k's symbol must be the one listed for k's
// value, so a swap cannot hand one side the other side's symbol. A side
// with no list constrains the value only. Where the node is a
// C_COMPLEX_OBJECT, the value must fit the C_INTEGER on its value and the
// RM Integer, and the symbol the constraint on its symbol, a code and a
// text the side admits (codedTextAdmits).
func ordinalFits(node *tcimpl.CompiledNode, k ordinalKey) bool {
	pc := primitiveOf(node)
	if pc == nil {
		return fitsInteger(k.value) &&
			componentFits(bindingChild(node, "value"), k.value) &&
			codedTextAdmits(bindingChild(node, "symbol"), k.symbol)
	}
	if c, ok := pc.(constraints.CDvOrdinal); ok && len(c.Values) > 0 {
		pair, found := ordinalPair(c, k.value)
		want := ordinalSymbolText(pair.Symbol).DefiningCode
		got := k.symbol.DefiningCode
		return found && got.CodeString == want.CodeString && got.TerminologyID == want.TerminologyID
	}
	return len(pc.Validate(int(k.value))) == 0
}

// ordinalEnd returns the lowest (low) or highest pair node's C_DV_ORDINAL
// lists, with the symbol the walk writes for it: for a code in any
// terminology other than openehr, the text of the term the nearest
// archetype root defines for it (useTermText). It returns false when node
// lists no pair its constraint admits. Where node is a C_COMPLEX_OBJECT,
// the symbol and the value are constrained apart, so the end is the lowest
// or highest value of the C_INTEGER on its value (integerEndOf) with like's
// symbol, the side's own, which its constraint on the symbol admitted; there
// is none where that C_INTEGER has no end.
func ordinalEnd(node *tcimpl.CompiledNode, like ordinalKey, low bool, lang string) (ordinalKey, bool) {
	c, ok := primitiveOf(node).(constraints.CDvOrdinal)
	if !ok {
		n, ok := integerEndOf(bindingChild(node, "value"), low)
		if !ok {
			return ordinalKey{}, false
		}
		return ordinalKey{value: n, symbol: like.symbol}, true
	}
	if len(c.Values) == 0 {
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
	symbol := ordinalSymbolText(best.Symbol)
	useTermText(node, &symbol, lang)
	return ordinalKey{value: int64(best.Value), symbol: symbol}, true
}

// ordinalAdopt returns the bound a side takes at other, the other side's
// current bound, where ordinalEnd gave it none. A side built as a
// C_COMPLEX_OBJECT constrains its symbol apart from its value, so it takes
// other's value with its own symbol, which its constraint admitted. Any
// other side takes other as it is.
func ordinalAdopt(node *tcimpl.CompiledNode, like, other ordinalKey, _ bool) (ordinalKey, bool) {
	if node == nil || primitiveOf(node) != nil {
		return other, true
	}
	return ordinalKey{value: other.value, symbol: like.symbol}, true
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
