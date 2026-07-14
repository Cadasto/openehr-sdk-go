package simplified

// REQ-053 — leaf datatype mapping: a concrete RM DataValue becomes one or
// more FLAT entries under a leaf path, keyed by the pipe attribute suffix
// (bare for value-only types). Explicit type switch, no reflection (REQ-024).
// The switch handles both value and pointer forms because a DataValue slot
// may hold either (see openehr/rm on substitution slots).

import "github.com/cadasto/openehr-sdk-go/openehr/rm"

// leafToFlat writes the FLAT entries for a single leaf value at flatPath.
// Unhandled datatypes are left for later cycles (|raw fallback, Task 6).
func leafToFlat(out map[string]any, flatPath string, v any) {
	switch dv := v.(type) {
	case rm.DVText:
		out[flatPath] = dv.Value
	case *rm.DVText:
		out[flatPath] = dv.Value
	case rm.DVCodedText:
		codedToFlat(out, flatPath, dv)
	case *rm.DVCodedText:
		codedToFlat(out, flatPath, *dv)
	case rm.DVDateTime:
		out[flatPath] = dv.Value
	case *rm.DVDateTime:
		out[flatPath] = dv.Value
	case rm.DVDate:
		out[flatPath] = dv.Value
	case *rm.DVDate:
		out[flatPath] = dv.Value
	case rm.DVTime:
		out[flatPath] = dv.Value
	case *rm.DVTime:
		out[flatPath] = dv.Value
	case rm.DVQuantity:
		quantityToFlat(out, flatPath, dv)
	case *rm.DVQuantity:
		quantityToFlat(out, flatPath, *dv)
	}
}

// codedToFlat emits the |code, |value and (external only) |terminology suffix
// entries for a DV_CODED_TEXT leaf.
func codedToFlat(out map[string]any, flatPath string, dv rm.DVCodedText) {
	out[flatPath+"|code"] = dv.DefiningCode.CodeString
	out[flatPath+"|value"] = dv.Value
	if term := dv.DefiningCode.TerminologyID.Value; term != "" {
		out[flatPath+"|terminology"] = term
	}
}

// quantityToFlat emits the |magnitude and |unit suffix entries for a
// DV_QUANTITY leaf.
func quantityToFlat(out map[string]any, flatPath string, dv rm.DVQuantity) {
	out[flatPath+"|magnitude"] = float64(dv.Magnitude)
	out[flatPath+"|unit"] = dv.Units
}
