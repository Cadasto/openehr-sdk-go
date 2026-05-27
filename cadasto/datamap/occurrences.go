package datamap

import "github.com/cadasto/openehr-sdk-go/openehr/template"

// occInfo is the datamap view of an OPT occurrences/cardinality interval.
// A nil lower means the OPT declared no occurrences block for the node.
type occInfo struct {
	lower          *int
	upper          *int
	upperUnbounded bool
}

// fromMultiplicity maps an openehr/template *Multiplicity (nil when the OPT
// declared no block) to occInfo.
func fromMultiplicity(m *template.Multiplicity) occInfo {
	if m == nil {
		return occInfo{}
	}
	lo := m.Lower()
	info := occInfo{lower: &lo, upperUnbounded: m.UpperUnbounded()}
	if !m.UpperUnbounded() {
		up := m.Upper()
		info.upper = &up
	}
	return info
}

// withOccurrences appends minOccurs / maxOccurs to a schema fragment, mirroring
// the dmv2 SchemaBuilder (maxOccurs is JSON null when the upper bound is
// unbounded).
func withOccurrences(schema map[string]any, occ occInfo) {
	if occ.lower != nil {
		schema["minOccurs"] = *occ.lower
	}
	if occ.upperUnbounded {
		schema["maxOccurs"] = nil
	} else if occ.upper != nil {
		schema["maxOccurs"] = *occ.upper
	}
}

// arraySchema wraps an item schema in an array with min/maxItems + occurrences.
func arraySchema(itemSchema map[string]any, occ occInfo) map[string]any {
	o := map[string]any{
		"type":  "array",
		"items": itemSchema,
	}
	if occ.lower != nil {
		o["minItems"] = *occ.lower
	}
	if !occ.upperUnbounded && occ.upper != nil {
		o["maxItems"] = *occ.upper
	}
	withOccurrences(o, occ)
	return o
}
