package simplified

// REQ-053 — leaf datatype mapping: a concrete RM DataValue becomes one or
// more FLAT entries under a leaf path, keyed by the pipe attribute suffix
// (bare for value-only types). Explicit type switch, no reflection (REQ-024).
// The switch handles both value and pointer forms because a DataValue slot
// may hold either (see openehr/rm on substitution slots).

import (
	"encoding/json"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
		// |raw fallback: a clinical datatype the core switch does not map is
		// embedded as its canonical-JSON fragment (which carries its own _type)
		// rather than dropped — the codec stays lossless (REQ-053). Non-DV_
		// leaves (party/context/other RM attributes) remain deferred; see
		// deviations.md.
		if strings.HasPrefix(rmType, "DV_") {
			raw, err := rawFragment(v, rmType)
			if err != nil {
				return err
			}
			out[flatPath+"|raw"] = raw
		}
	}
	return nil
}

// rawFragment serialises v to its openEHR canonical JSON (via canjson) and
// re-parses it as a generic value, so it nests inside the FLAT/STRUCTURED map
// under a |raw key. canjson emits _type only for pointer/polymorphic forms, so
// the fragment is stamped with rmType (the Web Template leaf type) when the
// value form omits it — decode requires _type on a |raw fragment.
func rawFragment(v any, rmType string) (any, error) {
	b, err := canjson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var frag any
	if err := json.Unmarshal(b, &frag); err != nil {
		return nil, err
	}
	if m, ok := frag.(map[string]any); ok {
		if _, has := m["_type"]; !has {
			m["_type"] = rmType
		}
	}
	return frag, nil
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
