package instance

import (
	"fmt"
	mrand "math/rand/v2"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// sampler draws pseudo-random numbers for RandomFill value generation.
// A nil rng routes to the package-global, auto-seeded math/rand/v2
// generator (so successive Generate calls differ); a non-nil rng wraps
// a caller-supplied [Options.ValueSource] for byte-reproducible output.
type sampler struct {
	rng *mrand.Rand
}

// newSampler builds a sampler from an optional source. A nil source
// yields a sampler backed by the global generator.
func newSampler(src mrand.Source) sampler {
	if src == nil {
		return sampler{}
	}
	return sampler{rng: mrand.New(src)}
}

func (s sampler) intN(n int) int {
	if n <= 0 {
		return 0
	}
	if s.rng != nil {
		return s.rng.IntN(n)
	}
	return mrand.IntN(n)
}

func (s sampler) int64N(n int64) int64 {
	if n <= 0 {
		return 0
	}
	if s.rng != nil {
		return s.rng.Int64N(n)
	}
	return mrand.Int64N(n)
}

func (s sampler) float64() float64 {
	if s.rng != nil {
		return s.rng.Float64()
	}
	return mrand.Float64()
}

// sampleValue returns a value drawn from within pc's constraint, in the
// same Go shape pc.ExampleValue() yields (so [generator.applyPrimitiveExample]
// handles both identically). The draw is self-checked against pc.Validate,
// so the result is always valid by construction.
//
// It falls back to pc.ExampleValue() — silently, and identically on every
// call — whenever the constraint carries nothing enumerable to sample
// (unconstrained primitive, pattern-only CString, CDuration) or a drawn
// candidate fails pc.Validate. Several *bounded* shapes therefore also
// degrade to the fixed example with no signal: an empty list∩range
// intersection, a single-value range, a collapsed exclusive-integer
// window, and the exclusive-lower real edge (a [lo,hi) draw can land on
// the excluded bound). Output stays valid, but RandomFill can equal
// ExampleFill for such a leaf. SDK-GAP-14.
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
			return lo + s.float64()*(hi-lo)
		}
		return s.float64() * 100 // unbounded: a small varied value

	case constraints.CString:
		if len(c.List) > 0 {
			return c.List[s.intN(len(c.List))]
		}
		return nil // pattern-only / unbounded: ExampleValue handles it

	case constraints.CodePhrase:
		if len(c.CodeList) > 0 {
			term := c.Terminology
			if term == "" {
				term = "local"
			}
			return constraints.CodedTermRef{Terminology: term, CodeString: c.CodeList[s.intN(len(c.CodeList))]}
		}
		return nil

	case constraints.CDvOrdinal:
		if len(c.Values) > 0 {
			return c.Values[s.intN(len(c.Values))].Value
		}
		return nil

	case constraints.DvQuantity:
		if len(c.Units) > 0 {
			u := c.Units[s.intN(len(c.Units))]
			mag := sampleMagnitude(u.Magnitude, s)
			return constraints.QuantityValue{Magnitude: mag, Units: u.Units, Precision: -1}
		}
		return nil

	case constraints.CDate:
		return s.randomDate()
	case constraints.CTime:
		return s.randomTime()
	case constraints.CDateTime:
		return s.randomDateTime()
	}
	// CDuration and any unknown constraint: ExampleValue sentinel.
	return nil
}

// sampleMagnitude draws an in-range magnitude for a quantity unit,
// falling back to the example magnitude for an unbounded range.
func sampleMagnitude(r constraints.NumericRange, s sampler) float64 {
	if lo, hi, ok := floatSampleBounds(r); ok {
		return lo + s.float64()*(hi-lo)
	}
	return exampleMagnitudeFallback(r)
}

// intSampleBounds returns an inclusive integer [lo, hi] window to draw
// from. A one-sided range gets a finite window anchored on the bounded
// side. Returns ok=false for an unbounded range.
func intSampleBounds(r constraints.NumericRange) (lo, hi int64, ok bool) {
	if !r.IsBounded() {
		return 0, 0, false
	}
	const window = 100
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
		hi = lo + window
	default: // only upper bounded
		hi = int64(r.Upper)
		if !r.UpperInclusive {
			hi--
		}
		lo = hi - window
	}
	if hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// floatSampleBounds returns a [lo, hi) window for a real range, with a
// finite window on the bounded side of a one-sided range. Returns
// ok=false for an unbounded range.
func floatSampleBounds(r constraints.NumericRange) (lo, hi float64, ok bool) {
	if !r.IsBounded() {
		return 0, 0, false
	}
	const window = 100
	switch {
	case !r.LowerUnbounded && !r.UpperUnbounded:
		lo, hi = r.Lower, r.Upper
	case !r.LowerUnbounded:
		lo, hi = r.Lower, r.Lower+window
	default:
		lo, hi = r.Upper-window, r.Upper
	}
	if hi <= lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// exampleMagnitudeFallback mirrors the constraints package's
// example-magnitude rule for an unbounded quantity range.
func exampleMagnitudeFallback(r constraints.NumericRange) float64 {
	if !r.LowerUnbounded {
		return r.Lower
	}
	if !r.UpperUnbounded {
		return r.Upper
	}
	return 0
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

// randomDate returns an extended ISO 8601 date the CDate validator
// accepts. Day is capped at 28 so every month is valid.
func (s sampler) randomDate() string {
	return fmt.Sprintf("%04d-%02d-%02d", 2000+s.intN(31), 1+s.intN(12), 1+s.intN(28))
}

// randomTime returns an extended ISO 8601 time (hh:mm:ss).
func (s sampler) randomTime() string {
	return fmt.Sprintf("%02d:%02d:%02d", s.intN(24), s.intN(60), s.intN(60))
}

// randomDateTime returns an RFC 3339 timestamp (UTC) the CDateTime
// validator accepts.
func (s sampler) randomDateTime() string {
	return fmt.Sprintf("%sT%sZ", s.randomDate(), s.randomTime())
}
