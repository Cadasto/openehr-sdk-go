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

// capturedKeys maps a leaf rmType to the canonical top-level attribute keys its
// FLAT suffix form fully represents. A value carrying any canonical key outside
// this set (mappings, normal_range, magnitude_status, accuracy, …) is not
// faithfully expressible as suffixes, so it is emitted as a lossless |raw
// fragment instead. This keeps the codec semantics-preserving (REQ-053) while
// still producing the human-readable suffix form for the common, undecorated case.
var capturedKeys = map[string]map[string]bool{
	"DV_TEXT":       {"value": true},
	"DV_CODED_TEXT": {"value": true, "defining_code": true},
	"DV_DATE_TIME":  {"value": true},
	"DV_DATE":       {"value": true},
	"DV_TIME":       {"value": true},
	"DV_DURATION":   {"value": true},
	"DV_URI":        {"value": true},
	"DV_EHR_URI":    {"value": true},
	"DV_QUANTITY":   {"magnitude": true, "units": true},
	"DV_COUNT":      {"magnitude": true},
	"DV_BOOLEAN":    {"value": true},
	"DV_ORDINAL":    {"symbol": true, "value": true},
	"DV_PROPORTION": {"numerator": true, "denominator": true, "type": true},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
}

// leafToFlat writes the FLAT entries for a single leaf value at flatPath. rmType
// is the Web Template leaf type. A DV_* value whose canonical form is fully
// captured by the suffix mapping (the common case) is emitted as suffixes; a
// decorated DV_* value (extra attributes) or an unmapped DV_* type is embedded
// losslessly as a |raw canonical fragment; a non-DV_ leaf (party / context /
// other RM attribute) is a documented skip (see deviations.md).
//
// DV_COUNT and DV_BOOLEAN carry their value as the bare leaf (mapping to RM
// magnitude / value), not a |suffix — per the STABLE Simplified Formats RM
// mappings.
func leafToFlat(out map[string]any, flatPath string, v any, rmType string) error {
	if captured, known := capturedKeys[rmType]; known {
		extra, err := hasUncapturedKeys(v, captured)
		if err != nil {
			return err
		}
		if !extra {
			emitCoreLeaf(out, flatPath, v, rmType)
			return nil
		}
	}
	if strings.HasPrefix(rmType, "DV_") {
		raw, err := rawFragment(v, rmType)
		if err != nil {
			return err
		}
		out[flatPath+"|raw"] = raw
	}
	return nil
}

// hasUncapturedKeys reports whether v's canonical JSON carries any top-level
// attribute beyond captured (and _type) — i.e. whether the suffix form would
// lose data.
func hasUncapturedKeys(v any, captured map[string]bool) (bool, error) {
	b, err := canjson.Marshal(v)
	if err != nil {
		return false, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return false, err
	}
	for k := range m {
		if k == "_type" || captured[k] {
			continue
		}
		return true, nil
	}
	return false, nil
}

// emitCoreLeaf writes the suffix form for a fully-captured leaf value. It is
// only reached for values whose canonical keys are within capturedKeys[rmType].
func emitCoreLeaf(out map[string]any, flatPath string, v any, rmType string) {
	switch dv := v.(type) {
	case rm.DVText:
		emitText(out, flatPath, dv.Value, rmType)
	case *rm.DVText:
		emitText(out, flatPath, dv.Value, rmType)
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
	case rm.DVDuration:
		out[flatPath] = dv.Value
	case *rm.DVDuration:
		out[flatPath] = dv.Value
	case rm.DVURI:
		out[flatPath] = dv.Value
	case *rm.DVURI:
		out[flatPath] = dv.Value
	case rm.DVEHRURI:
		out[flatPath] = dv.Value
	case *rm.DVEHRURI:
		out[flatPath] = dv.Value
	case rm.DVOrdinal:
		ordinalToFlat(out, flatPath, dv)
	case *rm.DVOrdinal:
		ordinalToFlat(out, flatPath, *dv)
	case rm.DVProportion:
		proportionToFlat(out, flatPath, dv)
	case *rm.DVProportion:
		proportionToFlat(out, flatPath, *dv)
	case rm.DVIdentifier:
		identifierToFlat(out, flatPath, dv)
	case *rm.DVIdentifier:
		identifierToFlat(out, flatPath, *dv)
	}
}

// emitText writes a DV_TEXT value: a bare leaf normally, but under the |other
// suffix when the leaf's Web Template type is DV_CODED_TEXT — an open-value-set
// free-text entry stored as DV_TEXT (spec §Open Value-Sets and |other).
func emitText(out map[string]any, flatPath, value, rmType string) {
	if rmType == "DV_CODED_TEXT" {
		out[flatPath+"|other"] = value
		return
	}
	out[flatPath] = value
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

// ordinalToFlat emits the |code, |value and |ordinal suffixes for a DV_ORDINAL
// leaf (symbol coded text + the integer position).
func ordinalToFlat(out map[string]any, flatPath string, dv rm.DVOrdinal) {
	out[flatPath+"|code"] = dv.Symbol.DefiningCode.CodeString
	out[flatPath+"|value"] = dv.Symbol.Value
	out[flatPath+"|ordinal"] = int64(dv.Value)
}

// proportionToFlat emits the |numerator, |denominator and |type suffixes for a
// DV_PROPORTION leaf. The derived bare magnitude and the status suffixes are
// not emitted (they are recomputed from numerator/denominator) — see
// deviations.md.
func proportionToFlat(out map[string]any, flatPath string, dv rm.DVProportion) {
	out[flatPath+"|numerator"] = float64(dv.Numerator)
	out[flatPath+"|denominator"] = float64(dv.Denominator)
	out[flatPath+"|type"] = int64(dv.Type)
}

// identifierToFlat emits the |id and optional |issuer, |assigner, |type
// suffixes for a DV_IDENTIFIER leaf.
func identifierToFlat(out map[string]any, flatPath string, dv rm.DVIdentifier) {
	out[flatPath+"|id"] = dv.ID
	if dv.Issuer != nil {
		out[flatPath+"|issuer"] = *dv.Issuer
	}
	if dv.Assigner != nil {
		out[flatPath+"|assigner"] = *dv.Assigner
	}
	if dv.Type != nil {
		out[flatPath+"|type"] = *dv.Type
	}
}
