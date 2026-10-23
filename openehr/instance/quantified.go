package instance

import (
	"errors"
	"fmt"
	"math"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// defaultMagnitudeStatus is the magnitude_status the generator writes on a
// quantified value whose OPT gives it none: a point value, the meaning the
// RM assumes when the attribute is not there.
const defaultMagnitudeStatus = "="

// magnitudeStatuses are the strings DV_QUANTIFIED Magnitude_status_valid
// admits as a magnitude_status, in the order valid_magnitude_status lists
// them, which starts with the default.
var magnitudeStatuses = []string{defaultMagnitudeStatus, "<", ">", "<=", ">=", "~"}

// errNoMagnitudeStatus reports a C_STRING on a quantified value's
// magnitude_status that accepts none of the strings DV_QUANTIFIED
// Magnitude_status_valid admits.
var errNoMagnitudeStatus = errors.New("the C_STRING accepts no string DV_QUANTIFIED Magnitude_status_valid admits")

// quantified reports whether v is a DV_QUANTIFIED value the generator
// builds: a quantity, a count, a proportion, a date, a time, a date-time
// or a duration. Each has a magnitude_status.
func quantified(v any) bool {
	switch v.(type) {
	case *rm.DVQuantity, *rm.DVCount, *rm.DVProportion, *rm.DVDate, *rm.DVTime, *rm.DVDateTime, *rm.DVDuration:
		return true
	}
	return false
}

// errNoPercentageAccuracy reports a DV_AMOUNT whose OPT holds its
// accuracy_is_percent at true and admits no accuracy that is a
// percentage: more than 0 and at most 100.
var errNoPercentageAccuracy = errors.New("the OPT holds accuracy_is_percent at true and admits no accuracy more than 0 and at most 100")

// amountAccuracy returns the accuracy and the accuracy_is_percent of v
// where v is a DV_AMOUNT the generator builds: a quantity, a count, a
// proportion or a duration.
func amountAccuracy(v any) (accuracy **rm.Real, percent **bool, ok bool) {
	switch a := v.(type) {
	case *rm.DVQuantity:
		return &a.Accuracy, &a.AccuracyIsPercent, true
	case *rm.DVCount:
		return &a.Accuracy, &a.AccuracyIsPercent, true
	case *rm.DVProportion:
		return &a.Accuracy, &a.AccuracyIsPercent, true
	case *rm.DVDuration:
		return &a.Accuracy, &a.AccuracyIsPercent, true
	}
	return nil, nil, false
}

// settleAccuracy makes the accuracy the walk wrote on a DV_AMOUNT meet the
// two DV_AMOUNT rules where accuracy_is_percent is true:
// Accuracy_is_percent_validity (an accuracy of 0 is no percentage) and
// Accuracy_validity (a percentage is at most 100), so a percentage accuracy
// is more than 0 and at most 100. An accuracy that already is one stays.
// Else the accuracy is narrowed to the first such value the OPT's
// constraint on accuracy accepts (percentageAccuracy), and where it
// accepts none, accuracy_is_percent becomes false where the OPT's
// constraint on it accepts that, so the accuracy is an absolute one. Where
// neither can be done, it returns an error wrapping
// ErrConstraintUnsatisfiable that names the RM type, the attribute and the
// OPT path, and no value. A value whose accuracy or accuracy_is_percent
// the walk did not write, or whose accuracy_is_percent is false, is left
// alone: neither rule binds it. opt is the OPT node of v.
func settleAccuracy(opt *tcimpl.CompiledNode, v any) error {
	accuracy, percent, ok := amountAccuracy(v)
	if !ok || *accuracy == nil || *percent == nil || !**percent {
		return nil
	}
	accuracyNode := bindingChild(opt, "accuracy")
	if a, found := percentageAccuracy(accuracyNode, float64(**accuracy)); found {
		narrowed := rm.Real(a)
		*accuracy = &narrowed
		return nil
	}
	if componentFits(bindingChild(opt, "accuracy_is_percent"), false) {
		absolute := false
		*percent = &absolute
		return nil
	}
	return fmt.Errorf("%w: %s.accuracy at %s: %w", ErrConstraintUnsatisfiable, rmTypeOf(v), leafPath(accuracyNode), errNoPercentageAccuracy)
}

// validPercentage reports whether a is an accuracy both DV_AMOUNT rules
// admit as a percentage: more than 0 and at most 100.
func validPercentage(a float64) bool { return a > 0 && a <= 100 }

// percentageAccuracy returns a percentage accuracy (validPercentage) that
// node, the OPT node of the accuracy, accepts: the first of these that is
// one and that node accepts: current, the accuracy the walk wrote; 100
// where current is above it; the members of the C_REAL's list, in order;
// 1; 100; and the middle of the part of the C_REAL's range from 0 to 100.
// It reports false when none is.
func percentageAccuracy(node *tcimpl.CompiledNode, current float64) (float64, bool) {
	candidates := []float64{current}
	if current > 100 {
		candidates = append(candidates, 100)
	}
	c, isReal := primitiveOf(node).(constraints.CReal)
	if isReal {
		candidates = append(candidates, c.List...)
	}
	candidates = append(candidates, 1, 100)
	if isReal && c.Range.IsBounded() {
		lo, hi := 0.0, 100.0
		if !c.Range.LowerUnbounded {
			lo = math.Max(lo, c.Range.Lower)
		}
		if !c.Range.UpperUnbounded {
			hi = math.Min(hi, c.Range.Upper)
		}
		candidates = append(candidates, (lo+hi)/2)
	}
	for _, a := range candidates {
		if validPercentage(a) && componentFits(node, a) {
			return a, true
		}
	}
	return 0, false
}
