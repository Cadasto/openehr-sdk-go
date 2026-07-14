package webtemplate

import (
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// inputsFor maps a compiled value node to the WebTemplate inputs for the
// core clinical datatype subset (REQ-106). Input suffix and type mirror the
// EHRbase v2.3 reference exactly; list values, terminology and numeric-range
// validation are populated where the constraint carries them. Deeper input
// contents (date patterns, default values, duration/precision ranges,
// per-unit validation, term bindings) are documented deviations — see
// deviations.md.
func inputsFor(v *templatecompile.CompiledNode) []Input {
	switch v.RMTypeName() {
	case "DV_TEXT":
		return []Input{{Type: "TEXT"}}
	case "DV_CODED_TEXT":
		return codedTextInputs(v)
	case "DV_QUANTITY":
		return quantityInputs(v)
	case "DV_COUNT":
		return []Input{{Type: "INTEGER", Validation: intRangeValidation(childConstraint[constraints.CInteger](v, "magnitude").Range)}}
	case "DV_ORDINAL":
		return ordinalInputs(v)
	case "DV_DATE_TIME":
		return []Input{{Type: "DATETIME", Validation: patternValidation(childConstraint[constraints.CDateTime](v, "value").Pattern)}}
	case "DV_DATE":
		return []Input{{Type: "DATE", Validation: patternValidation(childConstraint[constraints.CDate](v, "value").Pattern)}}
	case "DV_TIME":
		return []Input{{Type: "TIME", Validation: patternValidation(childConstraint[constraints.CTime](v, "value").Pattern)}}
	case "DV_BOOLEAN":
		return []Input{{Type: "BOOLEAN"}}
	case "DV_DURATION":
		return durationInputs(v)
	case "DV_PROPORTION":
		return proportionInputs(v)
	case "PARTY_PROXY":
		return partyProxyInputs()
	}
	return nil
}

// codedTextInputs emits the single "code" input for DV_CODED_TEXT. The
// C_CODE_PHRASE constraint lives on the value node's defining_code child;
// an absent or code-less constraint leaves the list open. Local
// (archetype-internal) terminology is implied and omitted, mirroring the
// reference, which carries `terminology` only for external bindings.
func codedTextInputs(v *templatecompile.CompiledNode) []Input {
	in := Input{Suffix: "code", Type: "CODED_TEXT"}
	cp := childConstraint[constraints.CodePhrase](v, "defining_code")
	if cp.Terminology != localTerminology {
		in.Terminology = cp.Terminology
	}
	for _, code := range cp.CodeList {
		in.List = append(in.List, listItem(v, code))
	}
	in.ListOpen = len(in.List) == 0
	return []Input{in}
}

// localTerminology is the archetype-internal terminology id; the
// reference omits `terminology` for it (only external bindings such as
// "openehr" are surfaced).
const localTerminology = "local"

// quantityInputs emits the magnitude (DECIMAL) + unit (CODED_TEXT) pair.
func quantityInputs(v *templatecompile.CompiledNode) []Input {
	mag := Input{Suffix: "magnitude", Type: "DECIMAL"}
	unit := Input{Suffix: "unit", Type: "CODED_TEXT"}
	if dq, ok := v.PrimitiveConstraint().(constraints.DvQuantity); ok {
		for _, u := range dq.Units {
			unit.List = append(unit.List, InputListItem{Value: u.Units, Label: u.Units})
		}
		if len(dq.Units) == 1 {
			mag.Validation = rangeValidation(dq.Units[0].Magnitude)
		}
	}
	return []Input{mag, unit}
}

// ordinalInputs emits the single CODED_TEXT input carrying the ordinal list.
func ordinalInputs(v *templatecompile.CompiledNode) []Input {
	in := Input{Type: "CODED_TEXT"}
	if ord, ok := v.PrimitiveConstraint().(constraints.CDvOrdinal); ok {
		for _, sym := range ord.Values {
			item := listItem(v, sym.Symbol.CodeString)
			ordinal := sym.Value
			item.Ordinal = &ordinal
			in.List = append(in.List, item)
		}
	}
	return []Input{in}
}

// durationInputs emits the INTEGER duration fields. When the C_DURATION
// pattern (on the value child) constrains which components are allowed, only
// those fields are emitted, in the pattern's letter order (e.g. "PYMWD" →
// year, month, week, day). An empty pattern emits all seven fields.
func durationInputs(v *templatecompile.CompiledNode) []Input {
	fields := durationFields(childConstraint[constraints.CDuration](v, "value").Pattern)
	out := make([]Input, 0, len(fields))
	for _, f := range fields {
		out = append(out, Input{Suffix: f, Type: "INTEGER"})
	}
	return out
}

// durationFields returns the allowed duration component names in order. An
// ISO-8601-style pattern (e.g. "PYMWD", "PYMDTHMS") maps each letter to a
// field; "M" is month before the "T" separator and minute after it. An empty
// pattern yields all seven fields in EHRbase's default order.
func durationFields(pattern string) []string {
	if pattern == "" {
		return []string{"year", "month", "day", "week", "hour", "minute", "second"}
	}
	var out []string
	afterT := false
	for _, r := range pattern {
		switch r {
		case 'T':
			afterT = true
		case 'Y':
			out = append(out, "year")
		case 'M':
			if afterT {
				out = append(out, "minute")
			} else {
				out = append(out, "month")
			}
		case 'W':
			out = append(out, "week")
		case 'D':
			out = append(out, "day")
		case 'H':
			out = append(out, "hour")
		case 'S':
			out = append(out, "second")
		}
	}
	return out
}

// proportionInputs emits the numerator + denominator DECIMAL pair. When
// the OPT fixes the proportion kind to percent (type C_INTEGER list [2])
// and carries no explicit denominator constraint, the reference derives
// the denominator bound `>=100 <=100` from the kind — mirrored here.
func proportionInputs(v *templatecompile.CompiledNode) []Input {
	num := Input{Suffix: "numerator", Type: "DECIMAL", Validation: rangeValidation(childConstraint[constraints.CReal](v, "numerator").Range)}
	den := Input{Suffix: "denominator", Type: "DECIMAL", Validation: rangeValidation(childConstraint[constraints.CReal](v, "denominator").Range)}
	if den.Validation == nil {
		den.Validation = kindDenominatorValidation(childConstraint[constraints.CInteger](v, "type").List)
	}
	return []Input{num, den}
}

// proportionKindPercent is the openEHR PROPORTION_KIND pk_percent value:
// the denominator is fixed at 100.
const proportionKindPercent = 100

// kindDenominatorValidation derives the fixed denominator bound implied
// by a single-valued proportion-kind constraint, or nil. Only the percent
// kind (2) is mirrored — the only kind the reference fixture pins
// (deviations.md lists the rest). Min and Max are distinct allocations:
// Build returns a mutable tree for post-processing, so the bounds must
// not alias.
func kindDenominatorValidation(kinds []int64) *Validation {
	if len(kinds) != 1 || kinds[0] != 2 {
		return nil
	}
	lo, hi := float64(proportionKindPercent), float64(proportionKindPercent)
	return &Validation{Range: &Range{Min: &lo, MinOp: ">=", Max: &hi, MaxOp: "<="}}
}

// intRangeValidation is rangeValidation for INTEGER inputs: the
// reference normalises exclusive integer bounds to inclusive
// (>10 → >=11, <15 → <=14), mirrored here.
func intRangeValidation(nr constraints.NumericRange) *Validation {
	v := rangeValidation(nr)
	if v == nil {
		return nil
	}
	if v.Range.Min != nil && v.Range.MinOp == ">" {
		*v.Range.Min++
		v.Range.MinOp = ">="
	}
	if v.Range.Max != nil && v.Range.MaxOp == "<" {
		*v.Range.Max--
		v.Range.MaxOp = "<="
	}
	return v
}

// patternValidation wraps a temporal constraint pattern (e.g.
// "yyyy-mm-ddTHH:MM:SS") as input validation, or nil when unconstrained.
// The reference copies the OPT pattern verbatim.
func patternValidation(pattern string) *Validation {
	if pattern == "" {
		return nil
	}
	return &Validation{Pattern: pattern}
}

// partyProxyInputs emits the four fixed TEXT identity inputs.
func partyProxyInputs() []Input {
	return []Input{
		{Suffix: "id", Type: "TEXT"},
		{Suffix: "id_scheme", Type: "TEXT"},
		{Suffix: "id_namespace", Type: "TEXT"},
		{Suffix: "name", Type: "TEXT"},
	}
}

// listItem builds a coded list entry, resolving the code's label and
// localized text from the archetype terms attached to the value node.
func listItem(v *templatecompile.CompiledNode, code string) InputListItem {
	item := InputListItem{Value: code}
	if t, ok := v.Term(code, ""); ok {
		item.Label = t.Items["text"]
	}
	return item
}

// childNode returns the first child of v reached through attribute attr.
func childNode(v *templatecompile.CompiledNode, attr string) *templatecompile.CompiledNode {
	a := v.Attribute(attr)
	if a == nil {
		return nil
	}
	if cs := a.Children(); len(cs) > 0 {
		return cs[0]
	}
	return nil
}

// childConstraint returns the typed primitive constraint on v's child reached
// through attribute attr, or the zero value when absent.
func childConstraint[T any](v *templatecompile.CompiledNode, attr string) T {
	var zero T
	c := childNode(v, attr)
	if c == nil {
		return zero
	}
	if pc, ok := c.PrimitiveConstraint().(T); ok {
		return pc
	}
	return zero
}

// rangeValidation converts a numeric range constraint into input validation,
// or nil when the range constrains nothing — unbounded on both sides, or
// the zero NumericRange that childConstraint yields when the constraint is
// absent (IsBounded covers both; a zero range must not become the
// impossible interval 0<x<0).
func rangeValidation(nr constraints.NumericRange) *Validation {
	if !nr.IsBounded() {
		return nil
	}
	r := &Range{}
	if !nr.LowerUnbounded {
		lo := nr.Lower
		r.Min = &lo
		r.MinOp = ">"
		if nr.LowerInclusive {
			r.MinOp = ">="
		}
	}
	if !nr.UpperUnbounded {
		hi := nr.Upper
		r.Max = &hi
		r.MaxOp = "<"
		if nr.UpperInclusive {
			r.MaxOp = "<="
		}
	}
	return &Validation{Range: r}
}
