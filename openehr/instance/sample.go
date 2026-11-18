package instance

import (
	"fmt"
	"math"
	mrand "math/rand/v2"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// sampler draws pseudo-random numbers for RandomFill value generation.
// A nil rng routes to the package-global, auto-seeded math/rand/v2
// generator (so successive Generate calls differ); a non-nil rng wraps
// a caller-supplied [Options.ValueSource] for byte-reproducible output.
type sampler struct {
	rng *mrand.Rand
	// quantityUnit, when set, is the unit both sides of one quantity
	// interval draw. Empty means the constraint's own unit list.
	quantityUnit string
}

// newSampler builds a sampler from an optional source. A nil source, or
// one that holds a nil pointer such as a nil *rand.PCG, yields a sampler
// backed by the global generator.
func newSampler(src mrand.Source) sampler {
	// idiom.md § No panics: a typed-nil source would panic on its first
	// draw, so it is read as nil.
	if isNilSource(src) {
		return sampler{}
	}
	return sampler{rng: mrand.New(src)}
}

func (s sampler) n[T int | int64](limit T) T {
	if limit <= 0 {
		return 0
	}
	if s.rng != nil {
		return s.rng.N(limit)
	}
	return mrand.N(limit)
}

func (s sampler) intN(n int) int       { return s.n(n) }
func (s sampler) int64N(n int64) int64 { return s.n(n) }

func (s sampler) float64() float64 {
	if s.rng != nil {
		return s.rng.Float64()
	}
	return mrand.Float64()
}

// between draws a float64 from lo up to hi. The Go spec lets a compiler
// fuse the multiply and the add into one instruction that rounds once, and
// arm64 does; the explicit conversion of the product forbids that, so one
// seed draws the same bits on every architecture.
func (s sampler) between(lo, hi float64) float64 {
	return lo + float64(s.float64()*(hi-lo))
}

// sampleValue returns a value drawn from within pc's constraint, in the
// same Go shape pc.ExampleValue() yields (so [generator.applyPrimitiveExample]
// handles both identically). The draw is self-checked against pc.Validate,
// so the result is always valid by construction.
//
// It falls back to pc.ExampleValue() — silently, and identically on every
// call — whenever the constraint carries nothing enumerable to sample
// (unconstrained primitive, pattern-only CString, a CDate, CTime or
// CDateTime with a Range, a CDuration that accepts no count of one
// designator but zero) or a drawn candidate fails pc.Validate.
// Several *bounded* shapes therefore also degrade to the fixed example
// with no signal: an empty list∩range intersection, a single-value range,
// a collapsed exclusive-integer window, and the exclusive-lower real edge
// (a [lo,hi) draw can land on the excluded bound). Output stays valid, but
// RandomFill can equal ExampleFill for such a leaf. REQ-107.
func sampleValue(pc constraints.PrimitiveConstraint, s sampler) any {
	v := sampleByConstraint(pc, s)
	if v == nil || len(pc.Validate(v)) != 0 {
		return pc.ExampleValue()
	}
	return v
}

// sampleByConstraint produces a candidate value per constraint type, or
// nil when the constraint carries no enumerable/bounded information to
// sample from (the caller then falls back to ExampleValue).
func sampleByConstraint(pc constraints.PrimitiveConstraint, s sampler) any {
	switch c := pc.(type) {
	case constraints.CBoolean:
		switch {
		case c.TrueValid && c.FalseValid:
			return s.intN(2) == 0
		case c.TrueValid:
			return true
		case c.FalseValid:
			return false
		}
		return nil

	case constraints.CInteger:
		if len(c.List) > 0 {
			allowed := c.List
			if c.Range.IsBounded() {
				allowed = filterInts(c.List, c.Range)
			}
			if len(allowed) > 0 {
				return allowed[s.intN(len(allowed))]
			}
			return nil
		}
		if lo, hi, ok := intSampleBounds(c.Range); ok {
			return lo + s.int64N(hi-lo+1)
		}
		return int64(s.intN(100)) // unbounded: a small varied value

	case constraints.CReal:
		if len(c.List) > 0 {
			allowed := c.List
			if c.Range.IsBounded() {
				allowed = filterFloats(c.List, c.Range)
			}
			if len(allowed) > 0 {
				return allowed[s.intN(len(allowed))]
			}
			return nil
		}
		if lo, hi, ok := floatSampleBounds(c.Range); ok {
			return s.between(lo, hi)
		}
		return s.float64() * 100 // unbounded: a small varied value

	case constraints.CString:
		if len(c.List) > 0 {
			return c.List[s.intN(len(c.List))]
		}
		return nil // pattern-only / unbounded: ExampleValue handles it

	case constraints.CodePhrase:
		return sampleCode(c, s)
	case constraints.CodeReference:
		// The code phrase it carries, in the terminology its URI names
		// where that phrase names none, as applyPrimitiveExample values it.
		cp, _ := constraints.CodePhraseWithTerminologyOf(c)
		return sampleCode(cp, s)

	case constraints.CDvOrdinal:
		// A leaf's C_DV_ORDINAL comes narrowed (narrowedLeaf) to the pairs
		// the generator chooses among, those with a code where the list has
		// one (ordinalChoices), so the draw is one of them.
		if len(c.Values) > 0 {
			return c.Values[s.intN(len(c.Values))].Value
		}
		return nil

	case constraints.DvQuantity:
		if len(c.Units) == 0 {
			// Any magnitude in the example's units: the units of the
			// property, or 1.
			q, ok := c.ExampleValue().(constraints.QuantityValue)
			if !ok {
				return nil
			}
			q.Magnitude = s.quantityMagnitude(openRange, openRange)
			return q
		}
		u, ok := quantityUnitEntry(c, s.quantityUnit)
		if !ok {
			u = c.Units[s.intN(len(c.Units))]
		}
		mag := s.quantityMagnitude(u.Magnitude, u.Precision)
		return constraints.QuantityValue{Magnitude: mag, Units: u.Units, Precision: -1}

	case constraints.CDate:
		return firstAccepted(pc, c.Range.IsBounded(), s.randomDate())
	case constraints.CTime:
		return firstAccepted(pc, c.Range.IsBounded(), s.randomTime())
	case constraints.CDateTime:
		return firstAccepted(pc, c.Range.IsBounded(), s.randomDateTime())
	case constraints.CDuration:
		return s.duration(c)
	}
	// Any unknown constraint: ExampleValue sentinel.
	return nil
}

// durationDesignators are the designators a drawn duration is written in,
// in written order: a count goes between the prefix and the letter.
var durationDesignators = [...]struct{ prefix, letter string }{
	{"P", "Y"}, {"P", "M"}, {"P", "W"}, {"P", "D"}, {"PT", "H"}, {"PT", "M"}, {"PT", "S"},
}

// durationWindow is the largest count a drawn duration has of its one
// designator where its constraint has no range, and how many counts it
// draws among from the bound of a range bounded on one side.
const durationWindow = 12

// maxDurationCount is the largest count durationCounts gives, so a count
// read from a very long bound stays an int64; Validate then judges it.
const maxDurationCount = math.MaxInt32

// duration draws a duration that is not zero for c, a count of one
// designator c accepts it in. Where c has no range, the count is from 1 to
// durationWindow, drawn, of the first designator c accepts it in, from one
// drawn at random on in written order and round again, so the pattern
// keeps it. Where c has a range, the count is drawn among those of the
// designator inside it (durationInRange), so the draw spreads across the
// range. Else the draw is the count 1 of the first designator c accepts, in
// written order; else nil, so sampleValue falls back to c's example.
func (s sampler) duration(c constraints.CDuration) any {
	n, start := 1+s.intN(durationWindow), s.intN(len(durationDesignators))
	if c.Range.IsBounded() {
		if v, ok := s.durationInRange(c, start); ok {
			return v
		}
	} else {
		for i := range len(durationDesignators) {
			d := durationDesignators[(start+i)%len(durationDesignators)]
			if v := fmt.Sprintf("%s%d%s", d.prefix, n, d.letter); len(c.Validate(v)) == 0 {
				return v
			}
		}
	}
	for _, d := range durationDesignators {
		if v := d.prefix + "1" + d.letter; len(c.Validate(v)) == 0 {
			return v
		}
	}
	return nil
}

// durationInRange draws a duration inside c's range: a count of one
// designator, drawn among the counts of it the range admits
// (durationCounts), that c accepts. It tries first the designators the
// range's bounds are written in, and then every designator, each set from
// the one at start on in written order and round again, so a range in
// years gives years where c's pattern admits them. It returns false where
// no draw fits c.
func (s sampler) durationInRange(c constraints.CDuration, start int) (string, bool) {
	every := make([]int, len(durationDesignators))
	for i := range every {
		every[i] = i
	}
	for _, set := range [][]int{rangeDesignators(c.Range), every} {
		for i := range len(set) {
			d := durationDesignators[set[(start+i)%len(set)]]
			lo, hi, ok := durationCounts(c.Range, d.prefix, d.letter)
			if !ok {
				continue
			}
			if v := fmt.Sprintf("%s%d%s", d.prefix, lo+s.int64N(hi-lo+1), d.letter); len(c.Validate(v)) == 0 {
				return v, true
			}
		}
	}
	return "", false
}

// rangeDesignators returns the designators the bounds of r are written in,
// as indexes into durationDesignators in written order: a letter before T
// in a bound is a date designator, and one after it a time designator. An
// unbounded side gives none.
func rangeDesignators(r constraints.TemporalRange) []int {
	var bounds []string
	if !r.LowerUnbounded {
		bounds = append(bounds, r.Lower)
	}
	if !r.UpperUnbounded {
		bounds = append(bounds, r.Upper)
	}
	var out []int
	for i, d := range durationDesignators {
		for _, b := range bounds {
			part, clock, _ := strings.Cut(b, "T")
			if d.prefix == "PT" {
				part = clock
			}
			if strings.Contains(part, d.letter) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// durationCounts returns the counts, from lo to hi, of a duration written
// as one count of the designator letter after prefix that lie inside r and
// are above 0: those between r's bounds where it has both; durationWindow
// of them from its bound where it has one, up from a lower bound and down
// from an upper one; and none where it is open, a bound does not parse, or
// no such count lies inside it. A count at an excluded bound is not one.
func durationCounts(r constraints.TemporalRange, prefix, letter string) (lo, hi int64, ok bool) {
	if !r.IsBounded() {
		return 0, 0, false
	}
	unit, ok := durationLength(prefix + "1" + letter)
	if !ok {
		return 0, 0, false
	}
	lo, hi = 1, durationWindow
	if !r.LowerUnbounded {
		length, ok := durationLength(r.Lower)
		if !ok {
			return 0, 0, false
		}
		lo = max(1, durationCount(math.Ceil(length/unit)))
		hi = lo + durationWindow - 1
	}
	if !r.UpperUnbounded {
		length, ok := durationLength(r.Upper)
		if !ok {
			return 0, 0, false
		}
		hi = durationCount(math.Floor(length / unit))
		if r.LowerUnbounded {
			lo = max(1, hi-durationWindow+1)
		}
	}
	// A count at an excluded bound, or one float64 rounding put just past a
	// bound, leaves the range: step inside it.
	inside := func(n int64) bool {
		return len(constraints.CDuration{Range: r}.Validate(fmt.Sprintf("%s%d%s", prefix, n, letter))) == 0
	}
	for range 2 {
		if lo <= hi && !inside(lo) {
			lo++
		}
		if lo <= hi && !inside(hi) {
			hi--
		}
	}
	return lo, hi, lo <= hi && inside(lo) && inside(hi)
}

// durationLength returns the length in seconds of the ISO 8601 duration s,
// BASE Iso8601_duration.to_seconds as the range of a C_DURATION reads it,
// and false where s does not parse.
func durationLength(s string) (float64, bool) {
	d, err := iso8601.ParseDuration(s)
	if err != nil {
		return 0, false
	}
	return d.ToSeconds(), true
}

// durationCount returns x as a count, held between 0 and maxDurationCount.
func durationCount(x float64) int64 {
	switch {
	case math.IsNaN(x) || x < 0:
		return 0
	case x > maxDurationCount:
		return maxDurationCount
	}
	return int64(x)
}

// firstAccepted returns the first of forms, one drawn value written in
// fewer fields each time, that pc accepts, so a draw is written to the
// pattern of a CDate, CTime or CDateTime. It returns nil when
// ranged, since the draw does not see the range, or when pc accepts no
// form; the caller then falls back to pc.ExampleValue, which keeps to both.
func firstAccepted(pc constraints.PrimitiveConstraint, ranged bool, forms []string) any {
	if ranged {
		return nil
	}
	for _, f := range forms {
		if len(pc.Validate(f)) == 0 {
			return f
		}
	}
	return nil
}

// quantityUnitEntry returns the first entry whose units equal name.
// An empty name, or a name the constraint does not list, is not an entry:
// the caller then draws a unit of its own. DvQuantity.Validate accepts a
// value inside any entry for its units, so drawing inside the first one
// gives a valid value.
func quantityUnitEntry(c constraints.DvQuantity, name string) (constraints.QuantityUnit, bool) {
	if name == "" {
		return constraints.QuantityUnit{}, false
	}
	for _, u := range c.Units {
		if u.Units == name {
			return u, true
		}
	}
	return constraints.QuantityUnit{}, false
}

// quantityDraws is how many times magnitude draws before it falls back to
// the middle of its window.
const quantityDraws = 4

// quantityMagnitude draws a magnitude for a quantity entry whose magnitude
// range is r and whose precision range is precision, rounded to the
// decimals precision gives (quantityDecimals), as magnitude draws one.
func (s sampler) quantityMagnitude(r, precision constraints.NumericRange) float64 {
	return s.magnitude(r, quantityDecimals(precision))
}

// count draws a DV_COUNT's magnitude under c, the C_INTEGER on it. A list,
// or a range bounded on both sides, is drawn from as any C_INTEGER is
// (sampleValue). A range that is open, or bounded on one side, is drawn
// from as a quantity's is (magnitude), with no decimals, so the draw lies
// away from the bound, or from 1 to 100 where the range is open, and is
// never 0. A draw c rejects gives way to the value ExampleFill gives
// (countExample).
func (s sampler) count(c constraints.CInteger) any {
	if len(c.List) > 0 || bothBounds(c.Range) {
		return sampleValue(c, s)
	}
	if n, ok := wholeMagnitude(s.magnitude(c.Range, 0)); ok && len(c.Validate(n)) == 0 {
		return n
	}
	return countExample(c, c.ExampleValue())
}

// magnitude draws a magnitude, of a quantity or a count, whose range is r,
// rounded to decimals. It draws from the range's window (quantityWindow),
// inset by a tenth of the window's width from each side the range bounds,
// and rounds the draw. A rounded draw that lands on 0, on a bound of the
// range or outside it is drawn again, up to quantityDraws times, and then
// the middle of the window, rounded, is tried. Where neither suits, the
// last draw is kept unrounded, which the window keeps inside the range;
// the caller then checks it against the constraint. A range that bounds
// nothing, the zero value included, is open.
func (s sampler) magnitude(r constraints.NumericRange, decimals int) float64 {
	if !r.IsBounded() {
		r = openRange
	}
	lo, hi := quantityWindow(r)
	inset := float64(hi-lo) / 10
	if !r.LowerUnbounded {
		lo += inset
	}
	if !r.UpperUnbounded {
		hi -= inset
	}
	suits := func(v float64) bool {
		return v != 0 && r.Contains(v) &&
			(r.LowerUnbounded || v != r.Lower) && (r.UpperUnbounded || v != r.Upper)
	}
	var drawn float64
	for range quantityDraws {
		drawn = s.between(lo, hi)
		if v := roundTo(drawn, decimals); suits(v) {
			return v
		}
	}
	if v := roundTo(float64(lo+hi)/2, decimals); suits(v) {
		return v
	}
	return drawn
}

// openRange is a numeric range open on both sides.
var openRange = constraints.NumericRange{LowerUnbounded: true, UpperUnbounded: true}

// quantityWindow returns the window a quantity magnitude is drawn from:
// the range itself where it is bounded on both sides, sampleWindow from
// its bound where it is bounded on one, and 1 to 100 where it is open.
func quantityWindow(r constraints.NumericRange) (lo, hi float64) {
	switch {
	case !r.LowerUnbounded && !r.UpperUnbounded:
		return r.Lower, r.Upper
	case !r.LowerUnbounded:
		return r.Lower, r.Lower + sampleWindow
	case !r.UpperUnbounded:
		return r.Upper - sampleWindow, r.Upper
	}
	return 1, sampleWindow
}

// maxQuantityDecimals is how many decimals a drawn magnitude keeps where
// the template's precision does not ask for fewer.
const maxQuantityDecimals = 2

// quantityDecimals returns how many decimals a magnitude drawn under the
// precision range precision keeps: the range's upper bound where it has
// one below maxQuantityDecimals, so none where it admits 0 alone and one
// where it admits up to 1, and maxQuantityDecimals otherwise. A precision
// of -1 means no limit, so an upper bound below 0 asks for no fewer.
func quantityDecimals(precision constraints.NumericRange) int {
	if precision.UpperUnbounded {
		return maxQuantityDecimals
	}
	upper := int(math.Floor(precision.Upper))
	if !precision.UpperInclusive && float64(upper) == precision.Upper {
		upper--
	}
	if upper < 0 || upper > maxQuantityDecimals {
		return maxQuantityDecimals
	}
	return upper
}

// roundTo rounds v to decimals decimal places. The scale is a power of
// ten, so a whole number comes back exact.
func roundTo(v float64, decimals int) float64 {
	scale := math.Pow10(decimals)
	return math.Round(v*scale) / scale
}

// sampleWindow is how far from its bounded side the sampler draws from a
// one-sided numeric range: a range bounded below alone draws up to this far
// above its lower bound, and one bounded above alone down to this far below
// its upper.
const sampleWindow = 100

// intSampleBounds returns an inclusive integer [lo, hi] window to draw
// from. A one-sided range gets a finite window of sampleWindow anchored on
// the bounded side. Returns ok=false for an unbounded range.
func intSampleBounds(r constraints.NumericRange) (lo, hi int64, ok bool) {
	if !r.IsBounded() {
		return 0, 0, false
	}
	switch {
	case !r.LowerUnbounded && !r.UpperUnbounded:
		lo = int64(r.Lower)
		if !r.LowerInclusive {
			lo++
		}
		hi = int64(r.Upper)
		if !r.UpperInclusive {
			hi--
		}
	case !r.LowerUnbounded:
		lo = int64(r.Lower)
		if !r.LowerInclusive {
			lo++
		}
		hi = lo + sampleWindow
	default: // only upper bounded
		hi = int64(r.Upper)
		if !r.UpperInclusive {
			hi--
		}
		lo = hi - sampleWindow
	}
	if hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// floatSampleBounds returns a [lo, hi) window for a real range, with a
// window of sampleWindow on the bounded side of a one-sided range. Returns
// ok=false for an unbounded range.
func floatSampleBounds(r constraints.NumericRange) (lo, hi float64, ok bool) {
	if !r.IsBounded() {
		return 0, 0, false
	}
	switch {
	case !r.LowerUnbounded && !r.UpperUnbounded:
		lo, hi = r.Lower, r.Upper
	case !r.LowerUnbounded:
		lo, hi = r.Lower, r.Lower+sampleWindow
	default:
		lo, hi = r.Upper-sampleWindow, r.Upper
	}
	if hi <= lo {
		return 0, 0, false
	}
	return lo, hi, true
}

func filterInts(list []int64, r constraints.NumericRange) []int64 {
	out := make([]int64, 0, len(list))
	for _, n := range list {
		if r.Contains(float64(n)) {
			out = append(out, n)
		}
	}
	return out
}

func filterFloats(list []float64, r constraints.NumericRange) []float64 {
	out := make([]float64, 0, len(list))
	for _, f := range list {
		if r.Contains(f) {
			out = append(out, f)
		}
	}
	return out
}

// randomDate draws an extended ISO 8601 date the CDate validator accepts
// and returns it with its day, then without it, then as its year alone.
// Day is capped at 28 so every month is valid.
func (s sampler) randomDate() []string {
	y, m, d := 2000+s.intN(31), 1+s.intN(12), 1+s.intN(28)
	return []string{
		fmt.Sprintf("%04d-%02d-%02d", y, m, d),
		fmt.Sprintf("%04d-%02d", y, m),
		fmt.Sprintf("%04d", y),
	}
}

// randomTime draws an extended ISO 8601 time and returns it as hh:mm:ss,
// hh:mm and hh, each first without a zone and then with Z.
func (s sampler) randomTime() []string {
	h, m, sec := s.intN(24), s.intN(60), s.intN(60)
	var out []string
	for _, t := range []string{fmt.Sprintf("%02d:%02d:%02d", h, m, sec), fmt.Sprintf("%02d:%02d", h, m), fmt.Sprintf("%02d", h)} {
		out = append(out, t, t+"Z")
	}
	return out
}

// randomDateTime draws an RFC 3339 timestamp (UTC) the CDateTime
// validator accepts and returns it first, then with fewer fields: its time
// as randomTime writes it, with Z first, and then its date alone as
// randomDate writes it.
func (s sampler) randomDateTime() []string {
	dates, times := s.randomDate(), s.randomTime()
	var out []string
	for i := 0; i < len(times); i += 2 {
		out = append(out, dates[0]+"T"+times[i+1], dates[0]+"T"+times[i])
	}
	return append(out, dates...)
}

// sampleCode draws a member of the code phrase constraint c's code list,
// under its terminology or under local where it names none, or returns nil
// when the list is empty.
func sampleCode(c constraints.CodePhrase, s sampler) any {
	if len(c.CodeList) == 0 {
		return nil
	}
	term := c.Terminology
	if term == "" {
		term = "local"
	}
	return constraints.CodedTermRef{Terminology: term, CodeString: c.CodeList[s.intN(len(c.CodeList))]}
}
