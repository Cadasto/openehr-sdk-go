// Hand-written companion to the generated DataValueText marker interface
// in data_types_text_gen.go. The generator emits the interface itself
// plus the per-concrete `isDataValueText()` markers; helpers below give
// callers an ergonomic read surface without forcing a type-switch at
// every site, and a wire-decode entrypoint that defaults `_type` to
// DV_TEXT (the supertype) when the canonical JSON omits the
// discriminator — a common shorthand for plain runtime names.

package rm

import (
	"encoding/json"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// DVTextValue returns the displayable text from any [DataValueText]
// implementation. For a `*DVText` the field is `Value` directly; for a
// `*DVCodedText` the same field is reached through the embedded
// `DVText`. Returns "" when the input is nil.
//
// Use this at consumer sites that only care about the display string —
// validation, logging, narrative composition. Sites that need the
// coded reference should type-switch on the interface and pull
// `defining_code` from a `*DVCodedText`.
func DVTextValue(d DataValueText) string {
	switch v := d.(type) {
	case nil:
		return ""
	case *DVText:
		if v == nil {
			return ""
		}
		return v.Value
	case *DVCodedText:
		if v == nil {
			return ""
		}
		return v.Value
	}
	return ""
}

// DVTextDefiningCode returns a non-nil pointer to the defining code
// when the [DataValueText] is a `*DVCodedText`; otherwise nil. Useful
// for downstream code that may want to consume the terminology binding
// (CDR query, audit, reporting) without committing to a full type
// switch.
func DVTextDefiningCode(d DataValueText) *CodePhrase {
	if dct, ok := d.(*DVCodedText); ok && dct != nil {
		dc := dct.DefiningCode
		return &dc
	}
	return nil
}

// DecodeDataValueText decodes a canonical-JSON wire payload into a
// concrete [DataValueText]. The decode path is identical to
// `typereg.DecodeAs[DataValueText]` except for one shorthand: when the
// payload omits the `_type` discriminator entirely, it defaults to
// `DV_TEXT` (the supertype). This matches a long-standing CDR / SDK
// canonical-JSON convention — a bare `{"value":"..."}` in a name slot
// is understood as a plain DV_TEXT — and keeps existing wire-cassettes
// compatible with the Phase-2 substitutability path (REQ-058).
//
// Used by generated UnmarshalJSON code at every field site whose BMM
// type promotes to [DataValueText].
func DecodeDataValueText(data []byte) (DataValueText, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var probe struct {
		Type string `json:"_type"`
	}
	if err := json.Unmarshal(data, &probe); err == nil && probe.Type == "" {
		// Default to DV_TEXT — canonical-JSON shorthand for plain text
		// in a DataValueText-typed slot.
		var dvt DVText
		if err := json.Unmarshal(data, &dvt); err != nil {
			return nil, err
		}
		return &dvt, nil
	}
	return typereg.DecodeAs[DataValueText](data)
}
