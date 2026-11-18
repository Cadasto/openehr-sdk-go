package instance

import (
	"math"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// openMagnitude is the magnitude ExampleFill gives a quantity or a count
// whose range does not bound it on both sides, where the range admits it,
// and the one the generator writes on a quantity or a count before a
// constraint the walk reaches values it.
const openMagnitude = 10

// representativeMagnitude returns the magnitude ExampleFill gives a
// quantity or a count whose magnitude range is r and which keeps decimals
// decimals, 0 for a count. Where r has both bounds it is the middle of r,
// rounded to decimals. Otherwise it is openMagnitude where r admits it,
// and else the bound of r nearest to it. A value on a bound r excludes
// moves one step inside it, a unit in the last decimal kept (stepFrom).
// The result can still lie outside r, as where r is narrower than one
// step, so the caller checks it against the constraint.
func representativeMagnitude(r constraints.NumericRange, decimals int) float64 {
	var v float64
	switch {
	case bothBounds(r):
		v = roundTo((r.Lower+r.Upper)/2, decimals)
	case r.Contains(openMagnitude):
		return openMagnitude
	case !r.LowerUnbounded:
		// Bounded below alone, at or above openMagnitude.
		v = r.Lower
	default:
		// Bounded above alone, at or below openMagnitude.
		v = r.Upper
	}
	if !r.LowerUnbounded && !r.LowerInclusive && v == r.Lower {
		v = stepFrom(v, decimals, true)
	}
	if !r.UpperUnbounded && !r.UpperInclusive && v == r.Upper {
		v = stepFrom(v, decimals, false)
	}
	return v
}

// bothBounds reports whether r bounds a value on both sides.
func bothBounds(r constraints.NumericRange) bool {
	return r.IsBounded() && !r.LowerUnbounded && !r.UpperUnbounded
}

// stepFrom returns v moved one step, a unit in the decimals-th decimal,
// up or down. Where v has no more than decimals decimals, the result is
// rounded to them, so it carries no float64 noise from the step.
func stepFrom(v float64, decimals int, up bool) float64 {
	step := math.Pow10(-decimals)
	if !up {
		step = -step
	}
	if roundTo(v, decimals) == v {
		return roundTo(v+step, decimals)
	}
	return v + step
}

// quantityExample returns the quantity ExampleFill gives under c. Of c's
// entries, in order, the first whose range admits the magnitude
// representativeMagnitude gives it, rounded to the decimals its precision
// range gives (quantityDecimals), takes that magnitude; an entry before it
// that admits its own example value (ExampleValue) takes that instead.
// Where c lists no entry, the quantity is openMagnitude in the units of
// c's example value. Where no entry admits either, it is c's example value.
func quantityExample(c constraints.DvQuantity) any {
	ex := c.ExampleValue()
	if len(c.Units) == 0 {
		q, ok := ex.(constraints.QuantityValue)
		if !ok {
			return ex
		}
		q.Magnitude = openMagnitude
		return q
	}
	for _, u := range c.Units {
		entry := constraints.DvQuantity{Units: []constraints.QuantityUnit{u}}
		q := constraints.QuantityValue{Magnitude: representativeMagnitude(u.Magnitude, quantityDecimals(u.Precision)), Units: u.Units, Precision: -1}
		if len(entry.Validate(q)) == 0 {
			return q
		}
		if own := entry.ExampleValue(); len(entry.Validate(own)) == 0 {
			return own
		}
	}
	return ex
}

// countExample returns the magnitude ExampleFill gives a DV_COUNT under c,
// the C_INTEGER on its magnitude: the magnitude representativeMagnitude
// gives c's range with no decimals, where c accepts it, and ex, c's own
// example value, otherwise. A c with a list keeps ex, the list entry its
// example value gives.
func countExample(c constraints.CInteger, ex any) any {
	if len(c.List) > 0 {
		return ex
	}
	n, ok := wholeMagnitude(representativeMagnitude(c.Range, 0))
	if !ok || len(c.Validate(n)) != 0 {
		return ex
	}
	return n
}

// wholeMagnitude returns v as an int64, and false where v is not a whole
// number an int64 holds.
func wholeMagnitude(v float64) (int64, bool) {
	if math.IsNaN(v) || v != math.Trunc(v) || v < -(1<<63) || v >= 1<<63 {
		return 0, false
	}
	return int64(v), true
}

// exampleQuantityMagnitude sets the magnitude of q, a DV_QUANTITY the OPT
// builds attribute by attribute or leaves without a constraint, opt being
// its OPT node, to the one ExampleFill gives: the magnitude
// representativeMagnitude gives the range of the constraint on q's
// magnitude, or no range where it has none, rounded to the decimals q's
// precision gives (drawnDecimals). It runs once the walk has written the
// precision, wherever the OPT lists it. Where the constraint on the
// magnitude lists values, or rejects that value, the magnitude the walk
// wrote stays.
func exampleQuantityMagnitude(opt *tcimpl.CompiledNode, q *rm.DVQuantity) {
	node := bindingChild(opt, "magnitude")
	var r constraints.NumericRange
	if c, ok := primitiveOf(node).(constraints.CReal); ok {
		if len(c.List) > 0 {
			return
		}
		r = c.Range
	}
	if v := representativeMagnitude(r, drawnDecimals(q.Precision)); componentFits(node, v) {
		q.Magnitude = rm.Real(v)
	}
}

// randomQuantityMagnitude sets the magnitude of q, a DV_QUANTITY the OPT
// builds attribute by attribute or leaves without a constraint, opt being
// its OPT node, to the one RandomFill gives. It runs once the walk has
// written the precision, wherever the OPT lists it. Where nothing
// constrains the magnitude, or a C_REAL with no list does
// (quantityMagnitudeRange), the magnitude is drawn as a C_DV_QUANTITY
// entry's is (magnitude), from 1 to 100 where the range is open and
// otherwise inside it away from its bounds, rounded to the decimals q's
// precision gives (drawnDecimals), so it is never 0. A draw the constraint
// rejects, and a magnitude under any other constraint, which the walk drew
// from it, keep the magnitude the walk wrote, rounded (roundQuantity).
func (g *generator) randomQuantityMagnitude(opt *tcimpl.CompiledNode, q *rm.DVQuantity) {
	node := bindingChild(opt, "magnitude")
	if r, drawn := quantityMagnitudeRange(primitiveOf(node)); drawn {
		if v := g.valueSampler.magnitude(r, drawnDecimals(q.Precision)); componentFits(node, v) {
			q.Magnitude = rm.Real(v)
			return
		}
	}
	roundQuantity(opt, q)
}

// quantityMagnitudeRange returns the range RandomFill draws a
// DV_QUANTITY's magnitude from where pc is the constraint on that
// magnitude, and whether it draws it that way rather than as any leaf
// under pc (sampleValue): an open range where pc is nil, and the range of
// a C_REAL with no list.
func quantityMagnitudeRange(pc constraints.PrimitiveConstraint) (constraints.NumericRange, bool) {
	switch c := pc.(type) {
	case nil:
		return openRange, true
	case constraints.CReal:
		if len(c.List) == 0 {
			return c.Range, true
		}
	}
	return constraints.NumericRange{}, false
}
