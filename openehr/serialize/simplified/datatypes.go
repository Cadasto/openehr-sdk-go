package simplified

// REQ-053 — leaf datatype mapping: a concrete RM DataValue becomes one or
// more FLAT entries under a leaf path, keyed by the pipe attribute suffix
// (bare for value-only types). Explicit type switch, no reflection (REQ-024).
// The switch handles both value and pointer forms because a DataValue slot
// may hold either (see openehr/rm on substitution slots).

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// leafToFlat writes the FLAT entries for a single leaf value at flatPath.
// rmType is the Web Template leaf type, used only to classify the fallthrough:
// an unmapped clinical DataValue (DV_*) is an error (silently dropping it would
// break the REQ-053 semantics-preserving contract — the |raw fallback is Phase
// 6), whereas an unmapped non-DV leaf (party/context/other RM attribute) is a
// documented deferral and is skipped (see deviations.md).
//
// DV_COUNT and DV_BOOLEAN carry their value as the bare leaf (mapping to RM
// magnitude / value), not a |suffix — per the STABLE Simplified Formats RM
// mappings.
func leafToFlat(out map[string]any, flatPath string, v any, rmType string) error {
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
	case rm.DVCount:
		out[flatPath] = dv.Magnitude
	case *rm.DVCount:
		out[flatPath] = dv.Magnitude
	case rm.DVBoolean:
		out[flatPath] = dv.Value
	case *rm.DVBoolean:
		out[flatPath] = dv.Value
	default:
		if strings.HasPrefix(rmType, "DV_") {
			return fmt.Errorf("%w: %s at %q", ErrUnsupportedDatatype, rmType, flatPath)
		}
	}
	return nil
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
