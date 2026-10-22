package instance

// normal_range.go: keeps an ordered value's normal_status consistent with
// its normal_range.

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// normalCode is the code string of the normal status of a value inside its
// normal range, the one DV_ORDERED Normal_range_and_status_consistency
// reads.
const normalCode = "N"

// highPrefix and lowPrefix start the code string of a normal status above
// the normal range (H, HH, HHH of the openEHR normal statuses code set) and
// of one below it (L, LL, LLL).
const (
	highPrefix = "H"
	lowPrefix  = "L"
)

// errNoNormalValue reports an ordered value whose OPT admits no value the
// generator can derive on the side of its normal_range that its
// normal_status needs: inside for the status N, outside for any other.
var errNoNormalValue = errors.New("the OPT admits no value the generator can derive inside the normal_range for the normal_status N, or outside it for another normal_status")

// settleNormalRange makes an ordered value the walk wrote keep DV_ORDERED
// Normal_range_and_status_consistency where it has both a normal_range and
// a normal_status: a status whose code string is N keeps the value inside
// the range, and any other status keeps it outside, as BASE Interval.has
// reads the range's bounds and its lower_included and upper_included. A
// value that already does stays, a RandomFill draw included. Else the
// value is narrowed to the first candidate (normalRule.candidates) that
// does and that the OPT's own constraint on the value accepts. A status
// whose code string starts with H (the high statuses) tries the step above
// the range's upper bound first, and one that starts with L (the low
// statuses) the step below its lower bound, so a high value lies above the
// range and a low value below it where the OPT lets it. The rest of the
// value is kept: a DV_QUANTITY's units, a DV_PROPORTION's type,
// denominator and precision, with a whole numerator where its kind or its
// precision needs one, and a DV_ORDINAL's symbol, which the OPT constrains
// on a node of its own. Where no candidate does, it returns an error
// wrapping ErrConstraintUnsatisfiable that names the RM type, the attribute
// and the OPT path, and no value.
//
// It reads the pairs orderIntervalBounds orders, with the comparisons it
// orders them by: a DV_COUNT by magnitude; a DV_QUANTITY by magnitude where
// the bounds of its range share its units; a DV_PROPORTION by ratio where
// they share its type; a DV_ORDINAL by value; and a DV_DATE, DV_TIME or
// DV_DATE_TIME where its bounds compare with it (temporalCompare). Any
// other value, such as a DV_SCALE or a DV_DURATION, one that a bound of its
// range does not compare with, and one whose range is open on both sides,
// which has no bound to pair it with, is left as generated. It runs once
// the value and its range are final: the walk has written the range, with
// its bounds in order
// (orderIntervalBounds) and its flags settled (settleIntervalEndpoints), and
// finishNode has settled a DV_PROPORTION's kind (settleProportion). The OPT
// constrains a DV_ORDINAL here through a C_INTEGER on its value: a
// C_DV_ORDINAL makes the ordinal a primitive leaf, which carries no
// normal_range. opt is the OPT node of v.
//
// A value that is a bound of an interval is settled by its own walk, before
// the interval's walk puts its bounds in order, which keeps each bound's
// settle (normalAt).
func settleNormalRange(opt *tcimpl.CompiledNode, v any) error {
	attr, kept := keepNormalRange(opt, v, firstCandidate)
	if kept {
		return nil
	}
	path := leafPath(opt) + "/" + attr
	if node := bindingChild(opt, attr); node != nil {
		path = leafPath(node)
	}
	return fmt.Errorf("%w: %s.%s at %s: %w", ErrConstraintUnsatisfiable, rmTypeOf(v), attr, path, errNoNormalValue)
}

// keepNormalRange is settleNormalRange without the error, and with the end
// of the candidates it takes: it narrows v as settleNormalRange describes,
// and reports the attribute it narrows and whether v keeps the rule or is
// left as generated. It reports false when no candidate keeps the rule, and
// v is then as it was.
func keepNormalRange(opt *tcimpl.CompiledNode, v any, end normalEnd) (attr string, kept bool) {
	switch o := v.(type) {
	case *rm.DVCount:
		return "magnitude", settleCountNormal(opt, o, end)
	case *rm.DVQuantity:
		return "magnitude", settleQuantityNormal(opt, o, end)
	case *rm.DVProportion:
		return "numerator", settleProportionNormal(opt, o, end)
	case *rm.DVOrdinal:
		return "value", settleOrdinalNormal(opt, o, end)
	case *rm.DVDate:
		return "value", settleTemporalNormal(opt, o.NormalStatus, o.NormalRange, &o.Value, dateScale, dateSteps, end,
			func(b rm.DVOrdered) (string, bool) { d, ok := asDate(b); return d.Value, ok })
	case *rm.DVTime:
		return "value", settleTemporalNormal(opt, o.NormalStatus, o.NormalRange, &o.Value, timeScale, timeSteps, end,
			func(b rm.DVOrdered) (string, bool) { d, ok := asTime(b); return d.Value, ok })
	case *rm.DVDateTime:
		return "value", settleTemporalNormal(opt, o.NormalStatus, o.NormalRange, &o.Value, dateTimeScale, dateTimeSteps, end,
			func(b rm.DVOrdered) (string, bool) { d, ok := asDateTime(b); return d.Value, ok })
	}
	return "", true
}

// normalAt returns how a bound of an interval, of Go type T, keeps its own
// normal_range rule at a value k of K, as settleNormalRange judges it: the
// bound as it stands with k written on it (apply), and read back (key) once
// keepNormalRange has narrowed it. A value that keeps the rule stays, and
// any other is narrowed to the lowest value that does, for the lower side of
// the interval (low), or the highest, for the upper side, among the
// candidates of the rule (normalRule.candidates). The function returns false
// where no candidate keeps the rule, and the bound is not changed either
// way. node is the OPT node the bound is built from. A bound with no
// normal_range or no normal_status, or one that does not compare with its
// range, keeps k.
func normalAt[T, K any](bound T, node *tcimpl.CompiledNode, key func(T) K, apply func(*T, K), low bool) func(k K) (K, bool) {
	end := highestCandidate
	if low {
		end = lowestCandidate
	}
	return func(k K) (K, bool) {
		c := bound
		apply(&c, k)
		if _, kept := keepNormalRange(node, &c, end); !kept {
			var none K
			return none, false
		}
		return key(c), true
	}
}

// normalBounds are the bounds of a normal_range read as K, with the flags
// BASE Interval.has reads.
type normalBounds[K any] struct {
	lower, upper                   K
	lowerUnbounded, upperUnbounded bool
	lowerIncluded, upperIncluded   bool
}

// normalBoundsOf reads the bounds of iv as K with read, and returns false
// when a bound that is there does not read as K, or when iv has no bound.
func normalBoundsOf[T, K any](iv *rm.Interval[T], read func(T) (K, bool)) (normalBounds[K], bool) {
	b := normalBounds[K]{
		lowerUnbounded: iv.LowerUnbounded, upperUnbounded: iv.UpperUnbounded,
		lowerIncluded: iv.LowerIncluded, upperIncluded: iv.UpperIncluded,
	}
	if b.lowerUnbounded && b.upperUnbounded {
		return b, false
	}
	var ok bool
	if !b.lowerUnbounded {
		if b.lower, ok = read(iv.Lower); !ok {
			return b, false
		}
	}
	if !b.upperUnbounded {
		if b.upper, ok = read(iv.Upper); !ok {
			return b, false
		}
	}
	return b, true
}

// has reports whether the range has v as BASE Interval.has reads it: v is
// above the lower bound, or at it where the range includes it, and below
// the upper bound, or at it where the range includes it, on each side that
// is bounded. ok is false when v does not compare with a bound.
func (r normalBounds[K]) has(compare func(a, b K) (int, bool), v K) (has, ok bool) {
	has = true
	if !r.lowerUnbounded {
		c, ok := compare(v, r.lower)
		if !ok {
			return false, false
		}
		has = c > 0 || (c == 0 && r.lowerIncluded)
	}
	if !r.upperUnbounded {
		c, ok := compare(v, r.upper)
		if !ok {
			return false, false
		}
		has = has && (c < 0 || (c == 0 && r.upperIncluded))
	}
	return has, true
}

// normalSteps are how a normalRule derives candidates of K from the bounds
// of a range.
type normalSteps[K any] struct {
	// rebase returns a bound on the value's own terms, such as a
	// proportion's ratio on the value's denominator. Nil keeps the bound.
	rebase func(bound K) K
	// near returns the value one step above k (up) or below it, and false
	// where there is none.
	near func(k K, up bool) (K, bool)
	// middle returns the value halfway between a and b, and false where
	// there is none. Nil gives none.
	middle func(a, b K) (K, bool)
	// round returns the candidates k gives where the value needs whole
	// numbers, such as a fraction's numerator. Nil keeps k.
	round func(k K) []K
}

// normalRule is one ordered value settleNormalRange keeps consistent with
// its normal_range, read as K.
type normalRule[K any] struct {
	bounds  normalBounds[K]
	value   K
	compare func(a, b K) (int, bool)
	steps   normalSteps[K]
	// own are the values the OPT's own constraint on the value names: its
	// lowest and its highest, then the members of its list.
	own []K
	// fits reports whether the OPT's own constraint on the value accepts k
	// and k keeps the value's other RM rules.
	fits func(k K) bool
	// set writes k on the value.
	set func(k K)
}

// normalEnd is which of the candidates that keep a normal_range rule keep
// takes.
type normalEnd int

const (
	// firstCandidate takes the first, in the order candidates gives.
	firstCandidate normalEnd = iota
	// lowestCandidate takes the lowest and highestCandidate the highest, as
	// the rule's compare orders them, the first of equals.
	lowestCandidate
	highestCandidate
)

// admits reports whether k is inside the range, where inside is set, or
// outside it, and the OPT's own constraint accepts k.
func (r normalRule[K]) admits(k K, inside bool) bool {
	has, ok := r.bounds.has(r.compare, k)
	return ok && has == inside && r.fits(k)
}

// beats reports whether a lies beyond b towards end, which is not
// firstCandidate.
func (r normalRule[K]) beats(a, b K, end normalEnd) bool {
	c, ok := r.compare(a, b)
	return ok && ((end == lowestCandidate && c < 0) || (end == highestCandidate && c > 0))
}

// keep narrows the value so that it is inside the range where status is N
// and outside it otherwise, as settleNormalRange describes, and reports
// whether the value keeps the rule or is left as generated. It takes the
// candidate that end names among those that keep the rule, trying the
// candidates on the side the status points to before the others
// (onSide), so a high value lands above the range wherever the OPT admits
// one there. It reports false when no candidate keeps the rule.
func (r normalRule[K]) keep(status *rm.CodePhrase, end normalEnd) bool {
	inside := status.CodeString == normalCode
	if has, ok := r.bounds.has(r.compare, r.value); !ok || has == inside {
		return true
	}
	side := sideOf(status)
	cands := r.candidates(side)
	if side != anySide {
		var first, rest []K
		for _, k := range cands {
			if r.onSide(k, side) {
				first = append(first, k)
			} else {
				rest = append(rest, k)
			}
		}
		cands = append(first, rest...)
	}
	var best K
	found := false
	for _, k := range cands {
		if !r.admits(k, inside) {
			continue
		}
		if end == firstCandidate {
			r.set(k)
			return true
		}
		if !found || r.beats(k, best, end) {
			best, found = k, true
		}
	}
	if found {
		r.set(best)
	}
	return found
}

// normalSide is the side of its normal_range that a normal_status points a
// value to, where the status needs the value outside it.
type normalSide int

const (
	// anySide is no side: the status is N, or none of the high and low
	// statuses, such as a placeholder code.
	anySide normalSide = iota
	// aboveRange is the side of the upper bound, for a high status.
	aboveRange
	// belowRange is the side of the lower bound, for a low status.
	belowRange
)

// onSide reports whether k is at or past the bound of side: at or above the
// upper bound for aboveRange, at or below the lower bound for belowRange. A
// side the range leaves open has no such value. keep asks it only of a
// candidate outside the range, so at the bound means past an excluded one.
func (r normalRule[K]) onSide(k K, side normalSide) bool {
	switch side {
	case aboveRange:
		c, ok := r.compare(k, r.bounds.upper)
		return !r.bounds.upperUnbounded && ok && c >= 0
	case belowRange:
		c, ok := r.compare(k, r.bounds.lower)
		return !r.bounds.lowerUnbounded && ok && c <= 0
	}
	return false
}

// sideOf returns the side of its range that status points a value to: above
// it for a code string that starts with H, below it for one that starts
// with L, and none for any other, N and a placeholder code included.
func sideOf(status *rm.CodePhrase) normalSide {
	switch {
	case strings.HasPrefix(status.CodeString, highPrefix):
		return aboveRange
	case strings.HasPrefix(status.CodeString, lowPrefix):
		return belowRange
	}
	return anySide
}

// candidates returns the values keep tries, in this order. For the side
// above the range it starts with the value one step above the upper bound,
// and for the side below it with the value one step below the lower bound,
// where that bound is there and the step gives a value; the same value then
// comes again in the order below, and keep takes the first that qualifies.
// The fixed order is: the lower bound, the value one step below it and the
// value one step above it; the same three for the upper bound; the middle
// of the two bounds, where both are there; and the values the OPT's own
// constraint on the value names (own). Each bound is taken on the value's
// own terms first (rebase), and each candidate gives the whole numbers next
// to it where the value needs whole numbers (round). A side the range
// leaves open gives none. A step from a time of day that would cross
// midnight gives none: no value below a bound at 00:00:00, and none above
// one at 23:59:59 (temporalNear).
func (r normalRule[K]) candidates(side normalSide) []K {
	rebase := func(b K) K {
		if r.steps.rebase == nil {
			return b
		}
		return r.steps.rebase(b)
	}
	var raw []K
	add := func(k K, ok bool) {
		if ok {
			raw = append(raw, k)
		}
	}
	var lower, upper K
	if !r.bounds.lowerUnbounded {
		lower = rebase(r.bounds.lower)
	}
	if !r.bounds.upperUnbounded {
		upper = rebase(r.bounds.upper)
	}
	switch {
	case side == aboveRange && !r.bounds.upperUnbounded:
		add(r.steps.near(upper, true))
	case side == belowRange && !r.bounds.lowerUnbounded:
		add(r.steps.near(lower, false))
	}
	if !r.bounds.lowerUnbounded {
		raw = append(raw, lower)
		add(r.steps.near(lower, false))
		add(r.steps.near(lower, true))
	}
	if !r.bounds.upperUnbounded {
		raw = append(raw, upper)
		add(r.steps.near(upper, false))
		add(r.steps.near(upper, true))
	}
	if !r.bounds.lowerUnbounded && !r.bounds.upperUnbounded && r.steps.middle != nil {
		add(r.steps.middle(lower, upper))
	}
	raw = append(raw, r.own...)
	if r.steps.round == nil {
		return raw
	}
	var out []K
	for _, k := range raw {
		out = append(out, r.steps.round(k)...)
	}
	return out
}

// integerSteps step a whole number by one, and give a whole number at the
// middle of two, or next to it where the middle is not whole.
var integerSteps = normalSteps[int64]{
	near: func(n int64, up bool) (int64, bool) {
		if up {
			return n + 1, n < math.MaxInt64
		}
		return n - 1, n > math.MinInt64
	},
	middle: func(a, b int64) (int64, bool) { return a/2 + b/2 + (a%2+b%2)/2, true },
}

// integerOwn returns the values the C_INTEGER of node names: its lowest
// and its highest (integerEndOf), then the members of its list.
func integerOwn(node *tcimpl.CompiledNode) []int64 {
	var own []int64
	for _, low := range []bool{true, false} {
		if n, ok := integerEndOf(node, low); ok {
			own = append(own, n)
		}
	}
	if c, ok := primitiveOf(node).(constraints.CInteger); ok {
		own = append(own, c.List...)
	}
	return own
}

// realOwn returns the values the C_REAL of node names: its lowest and its
// highest, among whole numbers where whole says so (realEndOf), then the
// members of its list.
func realOwn(node *tcimpl.CompiledNode, whole bool) []float64 {
	var own []float64
	for _, low := range []bool{true, false} {
		if n, ok := realEndOf(node, low, whole); ok {
			own = append(own, n)
		}
	}
	if c, ok := primitiveOf(node).(constraints.CReal); ok {
		own = append(own, c.List...)
	}
	return own
}

// finite reports whether f is a number canonical JSON can carry: neither
// infinite nor NaN.
func finite(f float64) bool { return !math.IsInf(f, 0) && !math.IsNaN(f) }

// settleCountNormal keeps a DV_COUNT's magnitude on the side of its
// normal_range its normal_status needs, inside the C_INTEGER on its
// magnitude, and reports false where it cannot (settleNormalRange).
func settleCountNormal(opt *tcimpl.CompiledNode, c *rm.DVCount, end normalEnd) bool {
	if c.NormalStatus == nil || c.NormalRange == nil {
		return true
	}
	bounds, ok := normalBoundsOf(&c.NormalRange.Interval, func(b rm.DVCount) (int64, bool) { return int64(b.Magnitude), true })
	if !ok {
		return true
	}
	return normalRule[int64]{
		bounds:  bounds,
		value:   int64(c.Magnitude),
		compare: countScale.compare,
		steps:   integerSteps,
		own:     integerOwn(bindingChild(opt, "magnitude")),
		fits:    func(m int64) bool { return countScale.fits(opt, m) },
		set:     func(m int64) { c.Magnitude = rm.Integer64(m) },
	}.keep(c.NormalStatus, end)
}

// settleQuantityNormal keeps a DV_QUANTITY's magnitude on the side of its
// normal_range its normal_status needs, in its own units and inside the
// C_REAL on its magnitude, and reports false where it cannot
// (settleNormalRange).
func settleQuantityNormal(opt *tcimpl.CompiledNode, q *rm.DVQuantity, end normalEnd) bool {
	if q.NormalStatus == nil || q.NormalRange == nil {
		return true
	}
	bounds, ok := normalBoundsOf(&q.NormalRange.Interval, func(b rm.DVQuantity) (quantityKey, bool) { return quantityKeyOf(b), true })
	if !ok {
		return true
	}
	value := quantityKeyOf(*q)
	magnitude := bindingChild(opt, "magnitude")
	var own []quantityKey
	for _, m := range realOwn(magnitude, false) {
		own = append(own, quantityKey{magnitude: m, units: value.units})
	}
	return normalRule[quantityKey]{
		bounds:  bounds,
		value:   value,
		compare: quantityScale.compare,
		steps: normalSteps[quantityKey]{
			near: func(k quantityKey, up bool) (quantityKey, bool) {
				step := -1.0
				if up {
					step = 1
				}
				k.magnitude += step
				return k, true
			},
			middle: func(a, b quantityKey) (quantityKey, bool) {
				a.magnitude = a.magnitude/2 + b.magnitude/2
				return a, true
			},
		},
		own:  own,
		fits: func(k quantityKey) bool { return componentFits(magnitude, k.magnitude) },
		set:  func(k quantityKey) { q.Magnitude = rm.Real(k.magnitude) },
	}.keep(q.NormalStatus, end)
}

// settleProportionNormal keeps a DV_PROPORTION's ratio on the side of its
// normal_range its normal_status needs, by its numerator alone, inside the
// C_REAL on its numerator, and reports false where it cannot
// (settleNormalRange). Its type, denominator and precision stay, which the
// walk and settleProportion settled under the OPT, and so do the whole
// numbers its kind or its precision needs (wholeOperands).
func settleProportionNormal(opt *tcimpl.CompiledNode, p *rm.DVProportion, end normalEnd) bool {
	if p.NormalStatus == nil || p.NormalRange == nil {
		return true
	}
	bounds, ok := normalBoundsOf(&p.NormalRange.Interval, func(b rm.DVProportion) (proportionKey, bool) { return proportionKeyOf(b), true })
	if !ok {
		return true
	}
	value := proportionKeyOf(*p)
	whole := wholeOperands(value.kind, value.precision)
	withNumerator := func(n float64) proportionKey {
		k := value
		k.numerator = n
		return k
	}
	var own []proportionKey
	for _, n := range realOwn(bindingChild(opt, "numerator"), whole) {
		own = append(own, withNumerator(n))
	}
	return normalRule[proportionKey]{
		bounds:  bounds,
		value:   value,
		compare: compareProportions,
		steps: normalSteps[proportionKey]{
			// A bound's ratio on the value's own denominator.
			rebase: func(b proportionKey) proportionKey {
				if b.denominator == value.denominator {
					return withNumerator(b.numerator)
				}
				return withNumerator(b.numerator * value.denominator / b.denominator)
			},
			// A higher numerator raises the ratio over a positive
			// denominator.
			near: func(k proportionKey, up bool) (proportionKey, bool) {
				step := -1.0
				if up == (k.denominator > 0) {
					step = 1
				}
				return withNumerator(k.numerator + step), true
			},
			middle: func(a, b proportionKey) (proportionKey, bool) {
				return withNumerator(a.numerator/2 + b.numerator/2), true
			},
			round: func(k proportionKey) []proportionKey {
				if !whole || math.Trunc(k.numerator) == k.numerator {
					return []proportionKey{k}
				}
				return []proportionKey{withNumerator(math.Floor(k.numerator)), withNumerator(math.Ceil(k.numerator))}
			},
		},
		own: own,
		// A bound rebased onto a far smaller denominator can overflow
		// float64, and canonical JSON has no infinity.
		fits: func(k proportionKey) bool { return finite(k.numerator) && proportionFits(opt, k) },
		set:  func(k proportionKey) { p.Numerator = rm.Real(k.numerator) },
	}.keep(p.NormalStatus, end)
}

// settleOrdinalNormal keeps a DV_ORDINAL's value on the side of its
// normal_range its normal_status needs, inside the 32-bit range of an RM
// Integer and the C_INTEGER on its value, and reports false where it
// cannot (settleNormalRange). Its symbol stays.
func settleOrdinalNormal(opt *tcimpl.CompiledNode, o *rm.DVOrdinal, end normalEnd) bool {
	if o.NormalStatus == nil || o.NormalRange == nil {
		return true
	}
	bounds, ok := normalBoundsOf(&o.NormalRange.Interval, func(b rm.DVOrdered) (ordinalKey, bool) {
		x, ok := asOrdinal(b)
		return ordinalKeyOf(x), ok
	})
	if !ok {
		return true
	}
	value := ordinalKeyOf(*o)
	withValue := func(n int64) ordinalKey { return ordinalKey{value: n, symbol: value.symbol} }
	valueNode := bindingChild(opt, "value")
	var own []ordinalKey
	for _, n := range integerOwn(valueNode) {
		own = append(own, withValue(n))
	}
	fromInteger := func(n int64, ok bool) (ordinalKey, bool) { return withValue(n), ok }
	return normalRule[ordinalKey]{
		bounds:  bounds,
		value:   value,
		compare: ordinalScaleIn("").compare,
		steps: normalSteps[ordinalKey]{
			rebase: func(b ordinalKey) ordinalKey { return withValue(b.value) },
			near:   func(k ordinalKey, up bool) (ordinalKey, bool) { return fromInteger(integerSteps.near(k.value, up)) },
			middle: func(a, b ordinalKey) (ordinalKey, bool) { return fromInteger(integerSteps.middle(a.value, b.value)) },
		},
		own:  own,
		fits: func(k ordinalKey) bool { return fitsInteger(k.value) && componentFits(valueNode, k.value) },
		set:  func(k ordinalKey) { o.Value = rm.Integer(k.value) },
	}.keep(o.NormalStatus, end)
}

// temporalStep is a layout of a temporal value and the step
// settleTemporalNormal moves a bound in that layout by: a day, or else a
// second. A time of day (clock) has no day to step into, so a step that would
// cross midnight is not taken.
type temporalStep struct {
	layout string
	day    bool
	clock  bool
}

// The layouts settleTemporalNormal steps a bound in: a full date in the
// extended layout, a day at a time; an extended time with seconds, with a Z
// or ±hh:mm zone or none, a second at a time; and a date-time of both, as
// RFC 3339 or with no zone, a second at a time. These are the layouts the
// generator writes. A bound in any other layout, such as a partial date,
// gives only itself.
var (
	dateSteps     = []temporalStep{{layout: "2006-01-02", day: true}}
	timeSteps     = []temporalStep{{layout: "15:04:05Z07:00", clock: true}, {layout: "15:04:05", clock: true}}
	dateTimeSteps = []temporalStep{{layout: time.RFC3339}, {layout: "2006-01-02T15:04:05"}}
)

// temporalNear returns the value one step after s (up) or before it in
// the first of steps whose layout s has, written in that layout, and false
// where s has none of them, or where the step of a time of day would cross
// midnight: no second before 00:00:00 and none after 23:59:59 is offered, as
// both would be on the other side of the day.
func temporalNear(steps []temporalStep, s string, up bool) (string, bool) {
	sign := -1
	if up {
		sign = 1
	}
	for _, st := range steps {
		t, err := time.Parse(st.layout, s)
		if err != nil {
			continue
		}
		var next time.Time
		if st.day {
			next = t.AddDate(0, 0, sign)
		} else {
			next = t.Add(time.Duration(sign) * time.Second)
		}
		if st.clock && !sameDay(t, next) {
			return "", false
		}
		return next.Format(st.layout), true
	}
	return "", false
}

// sameDay reports whether a and b fall on one calendar day in their own
// zones.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// settleTemporalNormal keeps the value of a DV_DATE, DV_TIME or
// DV_DATE_TIME on the side of its normal_range its normal_status needs,
// where its bounds compare with it under s, and reports false where it
// cannot (settleNormalRange). A C_DATE, C_TIME or C_DATE_TIME names no
// value of its own, so the candidates are the bounds and the values a step
// from them (steps); the OPT's constraint on the value is s.fits. read
// reads a bound's value.
func settleTemporalNormal(opt *tcimpl.CompiledNode, status *rm.CodePhrase, rng *rm.DVInterval[rm.DVOrdered], value *string,
	s scale[string], steps []temporalStep, end normalEnd, read func(rm.DVOrdered) (string, bool),
) bool {
	if status == nil || rng == nil {
		return true
	}
	bounds, ok := normalBoundsOf(&rng.Interval, read)
	if !ok {
		return true
	}
	return normalRule[string]{
		bounds:  bounds,
		value:   *value,
		compare: s.compare,
		steps: normalSteps[string]{
			near: func(k string, up bool) (string, bool) { return temporalNear(steps, k, up) },
		},
		fits: func(k string) bool { return s.fits(opt, k) },
		set:  func(k string) { *value = k },
	}.keep(status, end)
}
