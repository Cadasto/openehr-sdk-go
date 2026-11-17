package instance

// interval_order.go: keeps the two bounds of a generated interval in order.

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
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
// First it makes the two bounds strictly comparable where both sides admit
// such a pair (shareComparable): two DV_QUANTITY bounds take one unit, and
// two DV_PROPORTION bounds one type, that both sides admit, one in which an
// ordered pair fits both sides where there is such a one, so that RM
// DV_INTERVAL.Limits_consistent, which needs
// lower.is_strictly_comparable_to(upper), can hold.
//
// It orders DV_COUNT, DV_QUANTITY, DV_DATE, DV_TIME, DV_DATE_TIME,
// DV_DURATION, DV_PROPORTION and DV_ORDINAL bounds, in a DV_INTERVAL of that
// type and in a DV_INTERVAL<DV_ORDERED> whose two bounds are the same one of
// those types. Quantities are ordered only when both bounds share their
// units, since magnitudes in different units do not compare. Proportions
// are ordered by the ratio numerator/denominator, and only when both bounds
// share their type (proportionScale). Dates, times and date-times are
// ordered as the RM floor orders them: at the precision the two share, and
// two times or date-times written with different zone designators as
// instants (temporalCompare). Durations are ordered by their length in
// seconds, as the RM floor orders them (compareDurations). A pair that does
// not compare is left alone.
//
// An inverted pair is replaced by the first of these pairs that is in order
// and fits each side's OPT constraint:
//
//   - the two bounds swapped, which keeps the values RandomFill drew;
//   - the lowest value the lower side's constraint accepts, with the
//     highest value the upper side's constraint accepts. A side whose
//     constraint has no end in that direction takes the other side's
//     current bound instead, which a range open that way accepts; a
//     proportion side that has no usable end there, whether its range is
//     open or its type, denominator or precision rules the end out, takes
//     that bound's ratio on its own denominator (proportionAdopt). At an
//     exclusive real end the accepted extreme is the nearest float64 inside
//     it, and for a proportion that needs whole numbers the nearest whole
//     number it accepts. For a date, a time, a date-time or a duration it
//     is the value nearest the end of the range on the bound's value,
//     written to the side's pattern and moved inside an excluded end, among
//     the candidates temporalEnd reads;
//   - for a proportion, the second pair with a side's denominator moved to
//     the one the C_REAL on it gives that puts the side's ratio furthest
//     toward its end (proportionWider): the lower side's alone, then the
//     upper side's alone, then both;
//   - for a proportion, a side's bound moved to meet the other side's
//     bound in the second or third pair, over the denominator that puts its
//     ratio there (proportionReach): the lower side's, then the upper
//     side's.
//
// In a swap a proportion bound's precision moves with its numerator and
// denominator, which were settled under it. In the second pair each
// proportion bound keeps its type and denominator, and in the third and
// fourth its type; in each it takes a precision its own side admits, one
// that needs no more whole numbers than its type does wherever the side
// admits one (proportionPrecision), so its numerator is rounded only where
// its type, an is_integral its side pins to true, or every precision its
// side admits, needs whole numbers. A pair fits a side only where that
// side's constraint on precision admits the precision of its bound, so
// every bound written keeps the whole numbers its precision needs. A
// denominator the third or fourth pair moves is a whole number where the
// bound needs whole numbers, and stays where its kind fixes it. An ordinal bound the second pair replaces takes
// the symbol its side lists for the new value, with the text the nearest
// archetype root's term gives a code in any terminology other than openehr
// (useTermText), as the walk writes it.
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
// the rule. An in-order pair of two counts, quantities or ordinals
// therefore exists only if the second pair is in order. For a proportion
// the third pair holds each side's extreme over the denominators the C_REAL
// on its side lists or bounds on both sides away from 0 too, and the fourth
// a ratio at the other side's bound over a denominator no such C_REAL need
// give, from the side's own numerator or an end of the C_REAL on it. For a
// date, a time, a date-time or a duration the second pair holds the value
// nearest each side's end among temporalEnd's candidates, which leave out a
// value with a fraction of a second, one in another zone than the end, and
// a duration in more than one designator other than the end itself. So for
// a proportion and for those types an in-order pair can exist that no pair
// here finds. When no pair is in order, the bounds are left as they are,
// and the interval breaks Limits_consistent: the generator has no error for
// a constraint it cannot satisfy (sampleValue, for one, falls back to the
// example value without a signal).
func orderIntervalBounds(optNode *tcimpl.CompiledNode, rmValue any) {
	shareComparable(optNode, rmValue)
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
	case *rm.DVInterval[rm.DVDuration]:
		orderConcrete(&iv.Interval, optNode, durationScale, durationKeyOf, setDurationKey)
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
	if !inverted(key(iv.Lower), key(iv.Upper), s) {
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
// A mixed or missing pair is left alone.
func orderOrderedBounds(optNode *tcimpl.CompiledNode, iv *rm.Interval[rm.DVOrdered]) {
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
	if orderOrderedAs(iv, optNode, durationScale, asDuration, durationKeyOf, setDurationKey, setOrderedDuration) {
		return
	}
	if orderOrderedAs(iv, optNode, proportionScale, asProportion, proportionKeyOf, setProportionKey, setOrderedProportion) {
		return
	}
	orderOrderedAs(iv, optNode, ordinalScale, asOrdinal, ordinalKeyOf, setOrdinalKey, setOrderedOrdinal)
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
	if !inverted(key(lowerValue), key(upperValue), s) {
		return true
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
	// wider returns the lowest (low) or highest value a side's bound node
	// accepts where the value may change more of like than lowest and
	// highest change, as proportionWider changes a proportion's
	// denominator, and false when there is none. orderPair tries it only
	// where the pairs lowest and highest give are not in order. A nil wider
	// gives none.
	wider func(node *tcimpl.CompiledNode, like K, low bool) (K, bool)
	// reach returns a value a side's bound node accepts at other, the other
	// side's bound in a pair orderPair tries, or past it on the side's own
	// side (below it for the lower side, low), where the value may change
	// more of like than adopt changes, as proportionReach changes a
	// proportion's denominator, and false when there is none. orderPair
	// tries it only where no pair lowest, highest and wider give is in order.
	// A nil reach gives none.
	reach func(node *tcimpl.CompiledNode, like, other K, low bool) (K, bool)
}

// adopted is s.adopt, or other where s has no adopt.
func (s scale[K]) adopted(node *tcimpl.CompiledNode, like, other K, low bool) (K, bool) {
	if s.adopt == nil {
		return other, true
	}
	return s.adopt(node, like, other, low)
}

// inverted reports whether lower lies above upper, as s compares them, and
// false for a pair that does not compare.
func inverted[K any](lower, upper K, s scale[K]) bool {
	c, ok := s.compare(lower, upper)
	return ok && c > 0
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
	if !inverted(lower, upper, s) {
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
	type bound struct {
		k  K
		ok bool
	}
	var wideLo, wideHi bound
	if s.wider != nil {
		// A side changes more of its bound only where that puts the pair in
		// order: the lower side alone, then the upper side alone, then both.
		wideLo.k, wideLo.ok = s.wider(lowerSide.node, lower, true)
		wideHi.k, wideHi.ok = s.wider(upperSide.node, upper, false)
		for _, pair := range []struct {
			lo, hi K
			ok     bool
		}{
			{wideLo.k, hi, wideLo.ok && hiOK},
			{lo, wideHi.k, loOK && wideHi.ok},
			{wideLo.k, wideHi.k, wideLo.ok && wideHi.ok},
		} {
			if !pair.ok {
				continue
			}
			if lo, hi, ok := fitsInOrder(pair.lo, pair.hi); ok {
				return lo, hi, true
			}
		}
	}
	if s.reach == nil {
		return lower, upper, false
	}
	// A side moves its bound to meet the other side's bound in the pairs
	// above: the lower side to the upper's, then the upper side to the
	// lower's.
	for _, other := range []bound{{hi, hiOK}, wideHi} {
		if !other.ok {
			continue
		}
		if met, ok := s.reach(lowerSide.node, lower, other.k, true); ok {
			if lo, hi, ok := fitsInOrder(met, other.k); ok {
				return lo, hi, true
			}
		}
	}
	for _, other := range []bound{{lo, loOK}, wideLo} {
		if !other.ok {
			continue
		}
		if met, ok := s.reach(upperSide.node, upper, other.k, false); ok {
			if lo, hi, ok := fitsInOrder(other.k, met); ok {
				return lo, hi, true
			}
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
// entry for those units. DvQuantity.Validate accepts a value inside any
// entry for its units, so a value inside the first one validates. At
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
// DV_DATE_TIME bounds (temporalCompare says how), and durationScale
// DV_DURATION bounds (compareDurations). Each reads a side's lowest and
// highest value from the range of the C_DATE, C_TIME, C_DATE_TIME or
// C_DURATION on the bound's value (temporalEnd), among the candidates
// clockEnds or durationEnds gives.
var (
	dateScale     = temporalScale(temporalCompare(dateOrderRead), clockEnds(dateParts))
	timeScale     = temporalScale(temporalCompare(timeOrderRead), clockEnds(timeParts))
	dateTimeScale = temporalScale(temporalCompare(dateTimeOrderRead), clockEnds(dateTimeParts))
	durationScale = temporalScale(compareDurations, durationEnds)
)

// compareDurations compares two ISO 8601 durations by their length in
// seconds, BASE Iso8601_duration.to_seconds, which is the magnitude the
// REQ-123 parse gives and the order the RM floor puts two DV_DURATION
// bounds in (REQ-112 DV_INTERVAL). A value the parse refuses does not
// compare.
func compareDurations(a, b string) (int, bool) {
	da, errA := iso8601.ParseDuration(a)
	db, errB := iso8601.ParseDuration(b)
	if errA != nil || errB != nil {
		return 0, false
	}
	return compareOrdered(da.ToSeconds(), db.ToSeconds()), true
}

// temporalRead reads an ISO 8601 value of one type as the order of its
// type reads it: its fields as written, the zone not applied
// ([iso8601.Fields]); its magnitude read on its clock, the zone not applied,
// as the RM's Compare reads a date or a time; the zone designator it is
// written with, "" for none; and whether the REQ-123 parse accepts it.
type temporalRead func(s string) (fields iso8601.Fields, magnitude float64, zone string, ok bool)

func dateOrderRead(s string) (iso8601.Fields, float64, string, bool) {
	d, err := iso8601.ParseDate(s)
	return iso8601.DateFields(d), float64(d.Magnitude()), "", err == nil
}

func timeOrderRead(s string) (iso8601.Fields, float64, string, bool) {
	t, err := iso8601.ParseTime(s)
	return iso8601.TimeFields(t), t.Magnitude(), t.Zone, err == nil
}

// dateTimeOrderRead reads a date-time's magnitude on its clock, which
// temporalCompare expects: the RM's Compare reads a zoned date-time in UTC,
// and temporalCompare applies the zone itself. A date carries no "T", so a
// date-time the parse accepts has a time exactly when its text holds one.
func dateTimeOrderRead(s string) (iso8601.Fields, float64, string, bool) {
	d, t, err := iso8601.ParseDateTime(s)
	return iso8601.DateTimeFields(d, t, strings.Contains(s, "T")), iso8601.DateTimeMagnitude(d, t), t.Zone, err == nil
}

func dateKeyOf(d rm.DVDate) string { return d.Value }

func setDateKey(d *rm.DVDate, v string) { d.Value = v }

func timeKeyOf(d rm.DVTime) string { return d.Value }

func setTimeKey(d *rm.DVTime, v string) { d.Value = v }

func dateTimeKeyOf(d rm.DVDateTime) string { return d.Value }

func setDateTimeKey(d *rm.DVDateTime, v string) { d.Value = v }

func durationKeyOf(d rm.DVDuration) string { return d.Value }

func setDurationKey(d *rm.DVDuration, v string) { d.Value = v }

// temporalScale is the scale of one ISO 8601 type that compare orders, whose
// OPT constraint is the one on the bound's value. A side's lowest and
// highest value is the one temporalEnd reads from that constraint's range
// among the candidates ends gives.
func temporalScale(compare func(a, b string) (int, bool), ends func(r constraints.TemporalRange, low bool) []string) scale[string] {
	end := func(node *tcimpl.CompiledNode, low bool) (string, bool) {
		return temporalEnd(primitiveOf(bindingChild(node, "value")), compare, ends, low)
	}
	return scale[string]{
		compare: compare,
		fits: func(node *tcimpl.CompiledNode, v string) bool {
			pc := primitiveOf(bindingChild(node, "value"))
			return pc == nil || len(pc.Validate(v)) == 0
		},
		lowest:  func(node *tcimpl.CompiledNode, _ string) (string, bool) { return end(node, true) },
		highest: func(node *tcimpl.CompiledNode, _ string) (string, bool) { return end(node, false) },
	}
}

// temporalEnd returns the lowest (low) or highest value pc, the constraint
// on a side's bound value, accepts among the candidates ends gives for its
// range: the first it accepts, replaced by each later one that compare puts
// further that way. It returns false where pc is no C_DATE, C_TIME,
// C_DATE_TIME or C_DURATION, where its range has no end that way, and where
// pc accepts no candidate.
func temporalEnd(pc constraints.PrimitiveConstraint, compare func(a, b string) (int, bool), ends func(r constraints.TemporalRange, low bool) []string, low bool) (string, bool) {
	r, ok := temporalRangeOf(pc)
	if !ok || !r.IsBounded() || (low && r.LowerUnbounded) || (!low && r.UpperUnbounded) {
		return "", false
	}
	var best string
	found := false
	for _, v := range ends(r, low) {
		if len(pc.Validate(v)) != 0 {
			continue
		}
		if !found {
			best, found = v, true
			continue
		}
		if c, ok := compare(v, best); ok && ((low && c < 0) || (!low && c > 0)) {
			best = v
		}
	}
	return best, found
}

// temporalRangeOf returns the range of a C_DATE, C_TIME, C_DATE_TIME or
// C_DURATION, and false for any other constraint.
func temporalRangeOf(pc constraints.PrimitiveConstraint) (constraints.TemporalRange, bool) {
	switch c := pc.(type) {
	case constraints.CDate:
		return c.Range, true
	case constraints.CTime:
		return c.Range, true
	case constraints.CDateTime:
		return c.Range, true
	case constraints.CDuration:
		return c.Range, true
	}
	return constraints.TemporalRange{}, false
}

// durationEnds returns the candidates for the lowest (low) or highest
// duration the range r admits: its end that way as r writes it, then, in
// each designator in written order, the length 0 and the count of that
// designator nearest the end inside r (durationCounts), as the sampler
// writes a count, so a pattern that refuses the end's own designators
// admits one of them.
func durationEnds(r constraints.TemporalRange, low bool) []string {
	out := []string{r.Upper}
	if low {
		out[0] = r.Lower
	}
	for _, d := range durationDesignators {
		out = append(out, d.prefix+"0"+d.letter)
		if lo, hi, ok := durationCounts(r, d.prefix, d.letter); ok {
			n := hi
			if low {
				n = lo
			}
			out = append(out, fmt.Sprintf("%s%d%s", d.prefix, n, d.letter))
		}
	}
	return out
}

// The fields of a clockParts, from the largest down.
const (
	clockYear = iota
	clockMonth
	clockDay
	clockHour
	clockMinute
	clockSecond
	clockFieldCount
)

// clockParts is a date, a time or a date-time read field by field, its
// fraction of a second left out. f holds the year, the month, the day, the
// hour, the minute and the second; the type has the fields from lo up to
// hi, and the value carries those from lo up to n. zone is the designator
// the value is written with, "" for none, which a value with no time does
// not write.
type clockParts struct {
	f         [clockFieldCount]int
	lo, hi, n int
	zone      string
}

func dateParts(s string) (clockParts, bool) {
	d, err := iso8601.ParseDate(s)
	p := clockParts{lo: clockYear, hi: clockHour}
	p.setDate(d)
	return p, err == nil
}

func timeParts(s string) (clockParts, bool) {
	t, err := iso8601.ParseTime(s)
	p := clockParts{lo: clockHour, hi: clockFieldCount}
	p.setTime(t)
	return p, err == nil
}

// dateTimeParts reads a date-time, which carries a time exactly when its
// text holds a "T", as dateTimeOrderRead says.
func dateTimeParts(s string) (clockParts, bool) {
	d, t, err := iso8601.ParseDateTime(s)
	p := clockParts{lo: clockYear, hi: clockFieldCount}
	p.setDate(d)
	if strings.Contains(s, "T") {
		p.setTime(t)
	}
	return p, err == nil
}

func (p *clockParts) setDate(d iso8601.Date) {
	p.f[clockYear], p.n = d.Year, clockYear+1
	if d.MonthKnown {
		p.f[clockMonth], p.n = d.Month, clockMonth+1
	}
	if d.DayKnown {
		p.f[clockDay], p.n = d.Day, clockDay+1
	}
}

func (p *clockParts) setTime(t iso8601.Time) {
	p.f[clockHour], p.n = t.Hour, clockHour+1
	if t.MinuteKnown {
		p.f[clockMinute], p.n = t.Minute, clockMinute+1
	}
	if t.SecondKnown {
		p.f[clockSecond], p.n = t.Second, clockSecond+1
	}
	p.zone = t.Zone
}

// String writes p in the extended layout, with its zone after its time.
func (p clockParts) String() string {
	var b strings.Builder
	for i := p.lo; i < p.n; i++ {
		switch i {
		case clockYear:
			fmt.Fprintf(&b, "%04d", p.f[i])
		case clockMonth, clockDay:
			fmt.Fprintf(&b, "-%02d", p.f[i])
		case clockHour:
			if p.lo < clockHour {
				b.WriteByte('T')
			}
			fmt.Fprintf(&b, "%02d", p.f[i])
		default:
			fmt.Fprintf(&b, ":%02d", p.f[i])
		}
	}
	if p.n > clockHour {
		b.WriteString(p.zone)
	}
	return b.String()
}

// withFields returns p written with m fields: cut after its m-th where it
// carries more, and else filled out with the start of the period it names
// (start) or the end of it, such as 2021-01-01 or 2021-12-31 for 2021.
func (p clockParts) withFields(m int, start bool) clockParts {
	for i := p.n; i < m; i++ {
		switch {
		case start && (i == clockMonth || i == clockDay):
			p.f[i] = 1
		case start:
			p.f[i] = 0
		case i == clockMonth:
			p.f[i] = 12
		case i == clockDay:
			// Day 0 of the next month is the last day of this one.
			p.f[i] = time.Date(p.f[clockYear], time.Month(p.f[clockMonth]+1), 0, 0, 0, 0, 0, time.UTC).Day()
		case i == clockHour:
			p.f[i] = 23
		default:
			p.f[i] = 59
		}
	}
	p.n = m
	return p
}

// moved returns p moved by step, 1 or -1, in its field at, and cut after
// that field, so that it names the period next to the one p names at that
// precision: 2021-06 and 2021-04 for 2021-05-03 at the month. It returns
// false where the move takes a time out of its day.
func (p clockParts) moved(at, step int) (clockParts, bool) {
	f := p.withFields(at+1, true).withFields(clockFieldCount, true).f
	if p.lo == clockHour {
		f[clockYear], f[clockMonth], f[clockDay] = 2000, 1, 1
	}
	f[at] += step
	t := time.Date(f[clockYear], time.Month(f[clockMonth]), f[clockDay], f[clockHour], f[clockMinute], f[clockSecond], 0, time.UTC)
	if p.lo == clockHour && t.YearDay() != 1 {
		return clockParts{}, false
	}
	q := p
	q.f = [clockFieldCount]int{t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second()}
	q.n = at + 1
	return q, true
}

// clockEnds returns the candidates for the lowest (low) or highest value a
// range of dates, times or date-times, read by read, admits. They are its
// end that way as the range writes it, and then the end written with each
// number of fields its type has, from all down to one: cut after the last,
// or filled out with the start of the period it names for the lowest and
// the end of that period for the highest (withFields). Where the range
// excludes the end, they are instead the end moved by one in each field it
// carries, up for the lowest and down for the highest (moved), each so
// written. Each is in the extended layout, with the end's zone and without
// a fraction of a second.
func clockEnds(read func(s string) (clockParts, bool)) func(r constraints.TemporalRange, low bool) []string {
	return func(r constraints.TemporalRange, low bool) []string {
		end, excluded, step := r.Upper, !r.UpperInclusive, -1
		if low {
			end, excluded, step = r.Lower, !r.LowerInclusive, 1
		}
		out := []string{end}
		p, ok := read(end)
		if !ok {
			return out
		}
		seeds := []clockParts{p}
		if excluded {
			seeds = seeds[:0]
			for at := p.lo; at < p.n; at++ {
				if q, ok := p.moved(at, step); ok {
					seeds = append(seeds, q)
				}
			}
		}
		for _, s := range seeds {
			for m := s.hi; m > s.lo; m-- {
				out = append(out, s.withFields(m, low).String())
			}
		}
		return out
	}
}

// temporalCompare compares two ISO 8601 values of one type, read by read,
// in the order the RM floor puts two bounds of that type in (REQ-112
// DV_INTERVAL): at the precision the two share ([iso8601.Fields.Compare])
// when they are written with one zone designator, or both with none, so a
// partial value covers the whole period it names and 2021-05 is equal to
// 2021; and as instants, each by its magnitude less its zone's offset from
// UTC ([iso8601.UTCMagnitude]), a partial value standing for the start of
// the period it names, when both carry a zone and the designators differ.
// A value with a zone does not compare with one without, nor does a value
// the REQ-123 parse refuses with any.
func temporalCompare(read temporalRead) func(a, b string) (int, bool) {
	return func(a, b string) (int, bool) {
		aFields, aMag, aZone, aOK := read(a)
		bFields, bMag, bZone, bOK := read(b)
		if !aOK || !bOK {
			return 0, false
		}
		if aZone == bZone {
			return aFields.Compare(bFields), true
		}
		aUTC, aZoned := iso8601.UTCMagnitude(aMag, aZone)
		bUTC, bZoned := iso8601.UTCMagnitude(bMag, bZone)
		if !aZoned || !bZoned {
			return 0, false
		}
		return compareOrdered(aUTC, bUTC), true
	}
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
// and denominator, and the wider one its type alone (proportionWider); each
// takes a precision its side admits (proportionPrecision) and moves the
// numerator to the end its C_REAL accepts, which is the low or high ratio
// while that denominator stays positive. A negative
// denominator reverses which numerator end is the low ratio. A bound that
// needs whole numbers, as a fraction or a bound of precision 0 does, takes
// the whole number nearest that end its C_REAL accepts, so the
// replacement keeps the rules of its kind (settleProportion); so does a
// bound whose side pins is_integral to true (sideWhole). Where
// proportionEnd gives a side no bound, for whatever reason, the side takes
// the other bound's ratio on its own type and denominator instead
// (proportionAdopt), kept only where its constraints accept the result.
// Where no such pair is in order, a side keeps its type and takes a
// denominator that puts its ratio at the other bound's (proportionReach),
// which no C_REAL on its denominator need list or bound.
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
	wider: proportionWider,
	reach: proportionReach,
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
// constraint on that attribute, and its numerator and denominator are
// whole numbers where the node pins is_integral to true (integralPinned).
func proportionFits(node *tcimpl.CompiledNode, k proportionKey) bool {
	return componentFits(bindingChild(node, "numerator"), k.numerator) &&
		componentFits(bindingChild(node, "denominator"), k.denominator) &&
		componentFits(bindingChild(node, "type"), k.kind) &&
		precisionFits(node, k.precision) &&
		(!integralPinned(node) || (math.Trunc(k.numerator) == k.numerator && math.Trunc(k.denominator) == k.denominator))
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
		return attr == nil || !isRequired(node, attr)
	}
	return !prohibited(node, "precision") && componentFits(bindingChild(node, "precision"), int64(*precision))
}

func componentFits(node *tcimpl.CompiledNode, v any) bool {
	pc := primitiveOf(node)
	return pc == nil || len(pc.Validate(v)) == 0
}

// proportionEnd returns the lowest (low) or highest ratio a side accepts
// while keeping like's type and denominator, with the precision
// proportionPrecision gives and a whole numerator where that precision,
// the type or the side's is_integral pin needs whole numbers (sideWhole).
// It returns false when the numerator constraint has no end in the
// direction that moves the ratio that way, or no whole number toward that
// end where one is needed, or when the side does not admit like's type or
// denominator, or admits no precision.
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
	n, ok := realEndOf(bindingChild(node, "numerator"), numLow, sideWhole(node, like.kind, precision))
	if !ok {
		return proportionKey{}, false
	}
	return proportionKey{numerator: n, denominator: like.denominator, kind: like.kind, precision: precision}, true
}

// proportionWider returns the lowest (low) or highest ratio a side accepts
// while keeping like's type, over like's denominator and over each
// denominator the C_REAL on the side's denominator gives: the members of
// its list, or the ends of its range (realEndOf). Each denominator takes
// the numerator proportionEnd gives over it, and the ratio furthest toward
// that end wins, like's own denominator where two tie. A denominator is a
// whole number where the precision proportionPrecision gives, the type or
// the side's is_integral pin needs whole numbers (sideWhole), and stays as
// like has it for a unitary proportion or a percent, whose kind fixes it
// at 1 or 100. A numerator over a denominator of fixed sign is linear in
// both, so the ends of a range that does not hold 0 bound the ratios over
// it; one that holds 0 is read at its ends alone. It returns false where
// proportionEnd gives no bound over any of them.
func proportionWider(node *tcimpl.CompiledNode, like proportionKey, low bool) (proportionKey, bool) {
	precision, ok := proportionPrecision(node, like)
	if !ok {
		return proportionKey{}, false
	}
	whole := sideWhole(node, like.kind, precision)
	denominators := []float64{like.denominator}
	if like.kind != pkUnitary && like.kind != pkPercent {
		denominatorNode := bindingChild(node, "denominator")
		if c, ok := primitiveOf(denominatorNode).(constraints.CReal); ok && len(c.List) > 0 {
			denominators = append(denominators, c.List...)
		} else {
			for _, end := range []bool{true, false} {
				if d, ok := realEndOf(denominatorNode, end, whole); ok {
					denominators = append(denominators, d)
				}
			}
		}
	}
	var best proportionKey
	found := false
	for _, d := range denominators {
		if whole && math.Trunc(d) != d {
			continue
		}
		over := like
		over.denominator = d
		k, ok := proportionEnd(node, over, low)
		if !ok {
			continue
		}
		if c, _ := compareProportions(k, best); !found || (low && c < 0) || (!low && c > 0) {
			best, found = k, true
		}
	}
	return best, found
}

// proportionReach returns the bound a side takes to meet other, the other
// side's bound in a pair orderPair tries, where no denominator
// proportionWider reads puts the pair in order: like's type, the precision
// proportionPrecision gives, and a numerator over the denominator that puts
// its ratio at other's. Where float64 or a whole denominator leaves that
// ratio off, the denominator moves to put the ratio past other's on the
// side's own side, below it for the lower side (low): away from 0 or toward
// it, whichever moves the ratio that way, by rounding to a whole number where
// the precision, the type or the side's is_integral pin needs whole numbers
// (sideWhole), and then by one step, at most four times, of one float64 or
// one whole number. The numerator is like's own, else the low end, else the
// high end the C_REAL on the side's numerator accepts (realEndOf), the first
// over which the bound fits the side (proportionFits). It returns false for
// a unitary proportion or a percent, whose kind fixes the denominator, where
// other's ratio is 0 or does not compare, where node admits no precision,
// and where no numerator gives a bound that fits.
func proportionReach(node *tcimpl.CompiledNode, like, other proportionKey, low bool) (proportionKey, bool) {
	target, ok := proportionRatio(other)
	if like.kind == pkUnitary || like.kind == pkPercent || !ok || target == 0 {
		return proportionKey{}, false
	}
	precision, ok := proportionPrecision(node, like)
	if !ok {
		return proportionKey{}, false
	}
	whole := sideWhole(node, like.kind, precision)
	numerators := []float64{like.numerator}
	for _, end := range []bool{true, false} {
		if n, ok := realEndOf(bindingChild(node, "numerator"), end, whole); ok {
			numerators = append(numerators, n)
		}
	}
	// A denominator further from 0 puts the ratio nearer 0, which lowers a
	// positive ratio and raises a negative one.
	away := low == (target > 0)
	round := func(d float64) float64 {
		switch {
		case !whole:
			return d
		case away:
			return math.Copysign(math.Ceil(math.Abs(d)), d)
		}
		return math.Trunc(d)
	}
	for _, n := range numerators {
		if n == 0 || (whole && math.Trunc(n) != n) {
			continue
		}
		k := proportionKey{numerator: n, denominator: round(n * other.denominator / other.numerator), kind: like.kind, precision: precision}
		for range 4 {
			if c, ok := compareProportions(k, other); !ok || (low && c <= 0) || (!low && c >= 0) {
				break
			}
			toward := 0.0
			if away {
				toward = math.Copysign(math.Inf(1), k.denominator)
			}
			k.denominator = round(math.Nextafter(k.denominator, toward))
		}
		c, ok := compareProportions(k, other)
		if !ok || math.IsInf(k.denominator, 0) || (low && c > 0) || (!low && c < 0) || !proportionFits(node, k) {
			continue
		}
		return k, true
	}
	return proportionKey{}, false
}

// proportionPrecision returns the precision a bound of like's type takes
// where the second pair of orderIntervalBounds rewrites it on node's
// side, like being the bound that side drew, and false when node admits
// none. It tries like's own precision, none, the members of the C_INTEGER
// on precision, 1 and -1, in that order, and takes the first node admits
// that needs no more whole numbers than the type, or an is_integral node
// pins to true, does (sideWhole), so the numerator need not be rounded.
// Only where node admits no such precision does it take the first node
// admits, which is then 0. It never takes a precision below -1, which the
// RM floor rejects (-1 is no limit).
func proportionPrecision(node *tcimpl.CompiledNode, like proportionKey) (*rm.Integer, bool) {
	candidates := []*rm.Integer{like.precision, nil}
	if c, ok := primitiveOf(bindingChild(node, "precision")).(constraints.CInteger); ok {
		for _, p := range c.List {
			candidates = append(candidates, new(rm.Integer(p)))
		}
	}
	candidates = append(candidates, new(rm.Integer(1)), new(rm.Integer(-1)))
	typeWhole := sideWhole(node, like.kind, nil)
	for _, rounded := range []bool{false, true} {
		for _, p := range candidates {
			if p != nil && *p < -1 {
				continue
			}
			if (rounded || sideWhole(node, like.kind, p) == typeWhole) && precisionFits(node, p) {
				return p, true
			}
		}
	}
	return nil, false
}

// sideWhole reports whether a bound of kind with precision on a side's
// bound node needs a whole-number numerator and denominator: where its kind
// or its precision does (wholeOperands), and wherever the node pins
// is_integral to true (integralPinned).
func sideWhole(node *tcimpl.CompiledNode, kind int64, precision *rm.Integer) bool {
	return wholeOperands(kind, precision) || integralPinned(node)
}

// proportionAdopt returns the bound a side takes at other, the other
// side's current bound, where proportionEnd gave it none, low naming the
// lower side: like's type and denominator, the precision
// proportionPrecision gives, and the numerator that puts its ratio at
// other's. Where that precision, the type or the side's is_integral pin
// needs whole numbers (sideWhole), the
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
	whole := sideWhole(node, like.kind, precision)
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

// ordinalScale orders DV_ORDINAL bounds by Value. The OPT constrains a
// DV_ORDINAL bound as one C_DV_ORDINAL on the bound node itself, or on the
// nodes of its value and symbol attributes where it builds the bound as a
// C_COMPLEX_OBJECT.
var ordinalScale = scale[ordinalKey]{
	compare: func(a, b ordinalKey) (int, bool) { return compareOrdered(a.value, b.value), true },
	fits:    ordinalFits,
	lowest: func(node *tcimpl.CompiledNode, like ordinalKey) (ordinalKey, bool) {
		return ordinalEnd(node, like, true)
	},
	highest: func(node *tcimpl.CompiledNode, like ordinalKey) (ordinalKey, bool) {
		return ordinalEnd(node, like, false)
	},
	adopt: ordinalAdopt,
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
func ordinalEnd(node *tcimpl.CompiledNode, like ordinalKey, low bool) (ordinalKey, bool) {
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
	useTermText(node, &symbol)
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

func asDuration(v rm.DVOrdered) (rm.DVDuration, bool) {
	switch x := v.(type) {
	case rm.DVDuration:
		return x, true
	case *rm.DVDuration:
		if x != nil {
			return *x, true
		}
	}
	return rm.DVDuration{}, false
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

func setOrderedDuration(slot *rm.DVOrdered, value string) {
	switch x := (*slot).(type) {
	case *rm.DVDuration:
		x.Value = value
	case rm.DVDuration:
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

// shareComparable gives the two bounds of a generated interval, both sides
// bounded, one unit where they are two DV_QUANTITY (shareUnits), and one type
// where they are two DV_PROPORTION (shareTypes), that both sides admit, in a
// DV_INTERVAL of that type and in a DV_INTERVAL<DV_ORDERED> whose two bounds
// are both of it, in value or pointer form. The unit or type is one in which
// an ordered pair fits both sides where there is such a one, so two bounds
// that share theirs can take another. A pair the two sides admit no such unit
// or type for is left as it is, and so is a pair of two classes: the RM floor
// reports either as not strictly comparable (REQ-112 DV_INTERVAL).
func shareComparable(optNode *tcimpl.CompiledNode, rmValue any) {
	switch iv := rmValue.(type) {
	case *rm.DVInterval[rm.DVQuantity]:
		if bothBounded(&iv.Interval) {
			lowerNode, upperNode := boundNodes(optNode)
			shareUnits(&iv.Lower, &iv.Upper, lowerNode, upperNode)
		}
	case *rm.DVInterval[rm.DVProportion]:
		if bothBounded(&iv.Interval) {
			lowerNode, upperNode := boundNodes(optNode)
			shareTypes(&iv.Lower, &iv.Upper, lowerNode, upperNode)
		}
	case *rm.DVInterval[rm.DVOrdered]:
		if !bothBounded(&iv.Interval) {
			return
		}
		lowerNode, upperNode := boundNodes(optNode)
		if lo, hi, ok := orderedPair(iv, asQuantity); ok && shareUnits(&lo, &hi, lowerNode, upperNode) {
			setOrderedQuantity(&iv.Lower, quantityKeyOf(lo))
			setOrderedQuantity(&iv.Upper, quantityKeyOf(hi))
		}
		if lo, hi, ok := orderedPair(iv, asProportion); ok && shareTypes(&lo, &hi, lowerNode, upperNode) {
			setOrderedProportion(&iv.Lower, proportionKeyOf(lo))
			setOrderedProportion(&iv.Upper, proportionKeyOf(hi))
		}
	}
}

// orderedPair reads both bounds of a DV_INTERVAL<DV_ORDERED> as copies of
// one T, and reports false where either is not a T.
func orderedPair[T any](iv *rm.DVInterval[rm.DVOrdered], read func(rm.DVOrdered) (T, bool)) (lower, upper T, ok bool) {
	lower, lowerOK := read(iv.Lower)
	upper, upperOK := read(iv.Upper)
	return lower, upper, lowerOK && upperOK
}

// shareUnits gives two DV_QUANTITY bounds one unit both sides admit, and
// reports whether it changed them. It tries, in this order, the lower
// bound's units, the upper bound's, the units the lower side lists and then
// those the upper side lists (listedUnits). Of those that both sides admit
// (inUnits), it takes the first in which the two bounds can be put in order
// (orderable), and the first of them where none can. A pair with none is left
// as it is, and so is a pair in one unit that is the one it takes.
func shareUnits(lower, upper *rm.DVQuantity, lowerNode, upperNode *tcimpl.CompiledNode) bool {
	candidates := append([]string{lower.Units, upper.Units}, listedUnits(lowerNode)...)
	candidates = append(candidates, listedUnits(upperNode)...)
	lo, hi, ok := firstOrderable(candidates, func(units string) (rm.DVQuantity, rm.DVQuantity, bool) {
		lo, loOK := inUnits(lowerNode, *lower, units)
		hi, hiOK := inUnits(upperNode, *upper, units)
		return lo, hi, loOK && hiOK
	}, func(lo, hi rm.DVQuantity) bool {
		return orderable(lo, hi, lowerNode, upperNode, quantityScale, quantityKeyOf, setQuantityKey)
	})
	if !ok || (lo.Units == lower.Units && hi.Units == upper.Units) {
		return false
	}
	*lower, *upper = lo, hi
	return true
}

// firstOrderable returns the pair as of the first candidate that both sides
// admit (as) and in which the pair can be put in order (inOrder), else the
// pair as of the first candidate that both sides admit, and false where both
// sides admit none.
func firstOrderable[C, T any](candidates []C, as func(C) (T, T, bool), inOrder func(lo, hi T) bool) (lower, upper T, ok bool) {
	for _, c := range candidates {
		lo, hi, admitted := as(c)
		if !admitted {
			continue
		}
		if inOrder(lo, hi) {
			return lo, hi, true
		}
		if !ok {
			lower, upper, ok = lo, hi, true
		}
	}
	return lower, upper, ok
}

// orderable reports whether two bounds of T are in order, or can be put in
// order inside their sides' constraints as orderConcrete puts them
// (orderPair), each keeping its own normal_range rule (normalAt). A pair that
// does not compare is not.
func orderable[T, K any](lower, upper T, lowerNode, upperNode *tcimpl.CompiledNode, s scale[K], key func(T) K, apply func(*T, K)) bool {
	c, ok := s.compare(key(lower), key(upper))
	if !ok {
		return false
	}
	if c <= 0 {
		return true
	}
	lowerSide := orderSide[K]{node: lowerNode, narrow: normalAt(lower, lowerNode, key, apply, true)}
	upperSide := orderSide[K]{node: upperNode, narrow: normalAt(upper, upperNode, key, apply, false)}
	_, _, ok = orderPair(key(lower), key(upper), lowerSide, upperSide, s)
	return ok
}

// listedUnits returns the units a side lists: those of the entries of the
// C_DV_QUANTITY on its bound node, or, where it lists none, the units of the
// property it names (PropertyUnits), which are the units it admits, or the
// list of the C_STRING on the units of a bound the OPT builds attribute by
// attribute. A side that lists none returns nil.
func listedUnits(node *tcimpl.CompiledNode) []string {
	if c, ok := primitiveOf(node).(constraints.DvQuantity); ok {
		if len(c.Units) == 0 {
			return c.PropertyUnits
		}
		units := make([]string, 0, len(c.Units))
		for _, u := range c.Units {
			units = append(units, u.Units)
		}
		return units
	}
	if c, ok := primitiveOf(bindingChild(node, "units")).(constraints.CString); ok {
		return c.List
	}
	return nil
}

// inUnits returns q in units, and whether its side admits that: whether
// node, the side's bound node, accepts the result (quantityFits). A
// C_DV_QUANTITY that lists entries ties a magnitude range to each unit, so
// there q takes the magnitude ExampleValue gives for the first entry in
// units; on any other side q keeps its magnitude, which the side's
// constraint on the magnitude accepted already. A bound that carries a
// normal_range of its own admits only its own units: its range was settled
// in them (settleNormalRange). A nil node constrains nothing.
func inUnits(node *tcimpl.CompiledNode, q rm.DVQuantity, units string) (rm.DVQuantity, bool) {
	if q.Units == units {
		return q, true
	}
	if q.NormalRange != nil {
		return q, false
	}
	if c, ok := primitiveOf(node).(constraints.DvQuantity); ok && len(c.Units) > 0 {
		var entries []constraints.QuantityUnit
		for _, u := range c.Units {
			if u.Units == units {
				entries = append(entries, u)
			}
		}
		if len(entries) == 0 {
			return q, false
		}
		example, ok := constraints.DvQuantity{Units: entries}.ExampleValue().(constraints.QuantityValue)
		if !ok {
			return q, false
		}
		q.Magnitude = rm.Real(example.Magnitude)
	}
	q.Units = units
	return q, quantityFits(node, quantityKeyOf(q))
}

// shareTypes gives two DV_PROPORTION bounds one type both sides admit, and
// reports whether it changed them. It tries, in this order, the lower
// bound's type, the upper bound's, and every proportion kind in code order.
// Of those that both sides admit (asKind), it takes the first in which the
// two bounds can be put in order (orderable), and the first of them where
// none can. A pair with none is left as it is, and so is a pair of one type
// that is the one it takes.
func shareTypes(lower, upper *rm.DVProportion, lowerNode, upperNode *tcimpl.CompiledNode) bool {
	candidates := []int64{int64(lower.Type), int64(upper.Type)}
	for kind := range int64(pkIntegerFraction + 1) {
		candidates = append(candidates, kind)
	}
	lo, hi, ok := firstOrderable(candidates, func(kind int64) (rm.DVProportion, rm.DVProportion, bool) {
		lo, loOK := asKind(lowerNode, *lower, kind)
		hi, hiOK := asKind(upperNode, *upper, kind)
		return lo, hi, loOK && hiOK
	}, func(lo, hi rm.DVProportion) bool {
		return orderable(lo, hi, lowerNode, upperNode, proportionScale, proportionKeyOf, setProportionKey)
	})
	if !ok || (lo.Type == lower.Type && hi.Type == upper.Type) {
		return false
	}
	*lower, *upper = lo, hi
	return true
}

// asKind returns p as a proportion of kind, and whether its side admits
// that: p as it is where it is of kind already, which the walk settled
// (settleProportion), and else p settled as that kind inside the OPT's
// constraints on its parts under node (settleProportionAs). A bound that
// carries a normal_range of its own admits only its own type: its range was
// settled for it (settleNormalRange).
func asKind(node *tcimpl.CompiledNode, p rm.DVProportion, kind int64) (rm.DVProportion, bool) {
	if int64(p.Type) == kind {
		return p, true
	}
	if p.NormalRange != nil || !settleProportionAs(node, &p, kind, integralPinned(node)) {
		return p, false
	}
	return p, true
}

// sharedQuantityUnit returns one unit both DV_QUANTITY bounds of an
// interval can use, drawn from the intersection of their unit lists, for
// RandomFill to draw both bounds in. An empty intersection, or a side that
// is not a C_DV_QUANTITY, returns false so each side draws its own unit;
// shareComparable then gives the two bounds one unit where both sides admit
// one, a side the OPT builds attribute by attribute included, and another
// shared unit where an ordered pair fits both sides only in that one.
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
