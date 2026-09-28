package rmread

// interval_void.go: the Void test for an interval bound. A concrete-typed
// Go interval cannot hold a nil bound, so it spells Void as the bound
// type's Go zero; the bare DVInterval[DVOrdered] spells it as a nil (or
// typed-nil) interface. The predicates compare field by field, with no
// reflection (REQ-024), and TestIntervalBoundVoidPredicates guards them
// against a field added to an RM type later (REQ-112).

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

func isVoidOrdered(v rm.DVOrdered) bool {
	return v == nil || IsTypedNilPointer(v)
}

func isVoidDVCount(v rm.DVCount) bool {
	return v.Accuracy == nil && v.AccuracyIsPercent == nil && v.Magnitude == 0 &&
		v.MagnitudeStatus == nil && v.NormalRange == nil && v.NormalStatus == nil &&
		v.OtherReferenceRanges == nil
}

func isVoidDVDate(v rm.DVDate) bool {
	return v.Accuracy == nil && v.MagnitudeStatus == nil && v.NormalRange == nil &&
		v.NormalStatus == nil && v.OtherReferenceRanges == nil && v.Value == ""
}

func isVoidDVDateTime(v rm.DVDateTime) bool {
	return v.Accuracy == nil && v.MagnitudeStatus == nil && v.NormalRange == nil &&
		v.NormalStatus == nil && v.OtherReferenceRanges == nil && v.Value == ""
}

func isVoidDVTime(v rm.DVTime) bool {
	return v.Accuracy == nil && v.MagnitudeStatus == nil && v.NormalRange == nil &&
		v.NormalStatus == nil && v.OtherReferenceRanges == nil && v.Value == ""
}

func isVoidDVDuration(v rm.DVDuration) bool {
	return v.Accuracy == nil && v.AccuracyIsPercent == nil && v.MagnitudeStatus == nil &&
		v.NormalRange == nil && v.NormalStatus == nil && v.OtherReferenceRanges == nil &&
		v.Value == ""
}

func isVoidDVOrdinal(v rm.DVOrdinal) bool {
	return v.NormalRange == nil && v.NormalStatus == nil && v.OtherReferenceRanges == nil &&
		isVoidDVCodedText(v.Symbol) && v.Value == 0
}

func isVoidDVScale(v rm.DVScale) bool {
	return v.NormalRange == nil && v.NormalStatus == nil && v.OtherReferenceRanges == nil &&
		isVoidDVCodedText(v.Symbol) && v.Value == 0
}

func isVoidDVProportion(v rm.DVProportion) bool {
	return v.Accuracy == nil && v.AccuracyIsPercent == nil && v.Denominator == 0 &&
		v.MagnitudeStatus == nil && v.NormalRange == nil && v.NormalStatus == nil &&
		v.Numerator == 0 && v.OtherReferenceRanges == nil && v.Precision == nil && v.Type == 0
}

func isVoidDVQuantity(v rm.DVQuantity) bool {
	return v.Accuracy == nil && v.AccuracyIsPercent == nil && v.Magnitude == 0 &&
		v.MagnitudeStatus == nil && v.NormalRange == nil && v.NormalStatus == nil &&
		v.OtherReferenceRanges == nil && v.Precision == nil && v.Units == "" &&
		v.UnitsDisplayName == nil && v.UnitsSystem == nil
}

// isVoidDVCodedText and the two below it serve the DV_ORDINAL and DV_SCALE
// `symbol`, the one struct-typed field among the bound types.
func isVoidDVCodedText(v rm.DVCodedText) bool {
	return isVoidDVText(v.DVText) && isVoidCodePhrase(v.DefiningCode)
}

func isVoidDVText(v rm.DVText) bool {
	return v.Encoding == nil && v.Formatting == nil && v.Hyperlink == nil &&
		v.Language == nil && v.Mappings == nil && v.Value == ""
}

func isVoidCodePhrase(v rm.CodePhrase) bool {
	return v.CodeString == "" && v.PreferredTerm == nil && v.TerminologyID == (rm.TerminologyID{})
}
