package simplified_test

// REQ-053 — a refusal names a duplicate sibling as the body wrote it. Decode
// reads the reference's `<id>2` as the Web Template's `<id>_1` before any key
// is read, and every refusal after that names the key, a path below the
// sibling, or the sibling's id the way the body spells it.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// siblingSpelling is one way of spelling the duplicate siblings of
// test_all_types the cases write: the EVALUATION test_all_types_1, the SECTION
// test_all_types_2, the ACTION test_all_types_1 below the SECTION, the
// OBSERVATION's second time leaf time_1, and the second value type of
// contextCoded in [codedAlternativesAllTypes], coded_text_value_1.
type siblingSpelling struct {
	evaluation, section, action, time, alternative string
}

var (
	referenceSpelling   = siblingSpelling{"test_all_types2", "test_all_types3", "test_all_types2", "time2", "coded_text_value2"}
	webTemplateSpelling = siblingSpelling{"test_all_types_1", "test_all_types_2", "test_all_types_1", "time_1", "coded_text_value_1"}
)

// ids returns the ids s spells the duplicates with.
func (s siblingSpelling) ids() []string {
	return []string{s.evaluation, s.section, s.action, s.time, s.alternative}
}

// evaluationPath is the EVALUATION's instance path.
func (s siblingSpelling) evaluationPath() string {
	return "test_all_types/" + s.evaluation + ":0"
}

// evaluationText is the DV_TEXT leaf below the EVALUATION's clusters.
func (s siblingSpelling) evaluationText() string {
	return s.evaluationPath() + "/cluster_1/cluster_2/cluster_3/text_2"
}

// actionPath is the ACTION's instance path below the SECTION.
func (s siblingSpelling) actionPath() string {
	return "test_all_types/" + s.section + ":0/section_2/section_3/" + s.action + ":0"
}

// TestREQ053RefusalNamesDuplicateSiblingAsWritten — REQ-053: for every
// refusal family that names a key, a path or an id, a body that spells a
// duplicate sibling the reference's way is refused naming it that way and
// never by the Web Template's id, and the error still wraps the family's
// sentinel. The control, the same body in the Web Template's spelling, is
// named in that spelling.
func TestREQ053RefusalNamesDuplicateSiblingAsWritten(t *testing.T) {
	allTypes, allTypesCompiled := allTypesSet(t)
	alternatives, alternativesCompiled := codedAlternativesAllTypes(t)
	tests := []struct {
		name string
		wt   *webtemplate.WebTemplate
		opts []simplified.Option
		// leaves are the body's keys in spelling s, flatNamed what a FLAT
		// refusal of them names.
		leaves    func(s siblingSpelling) map[string]any
		flatNamed func(s siblingSpelling) []string
		sentinel  error
	}{
		{
			name: "a value that does not fit, below a sibling",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.evaluationText(): 5}
			},
			flatNamed: func(s siblingSpelling) []string { return []string{`"` + s.evaluationText() + `"`} },
			sentinel:  simplified.ErrUnsupportedDatatype,
		},
		{
			name: "a value that does not fit, at a sibling leaf",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{shorthandObs + "/" + s.time: 5}
			},
			flatNamed: func(s siblingSpelling) []string { return []string{`"` + shorthandObs + "/" + s.time + `"`} },
			sentinel:  simplified.ErrUnsupportedDatatype,
		},
		{
			name: "a bare value spelled with a bare |, named without it",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{shorthandObs + "/" + s.time + "|": 5}
			},
			flatNamed: func(s siblingSpelling) []string { return []string{`"` + shorthandObs + "/" + s.time + `"`} },
			sentinel:  simplified.ErrUnsupportedDatatype,
		},
		{
			name: "an interval bound that does not fit",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.evaluationPath() + "/interval_count/lower": "x"}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + s.evaluationPath() + "/interval_count/lower" + `"`}
			},
			sentinel: simplified.ErrUnsupportedDatatype,
		},
		{
			name: "a coded text the template does not complete",
			wt:   allTypes, opts: []simplified.Option{simplified.WithTemplate(allTypesCompiled)},
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.evaluationText() + "|code": "c1", s.evaluationText() + "|value": "a term"}
			},
			flatNamed: func(s siblingSpelling) []string { return []string{`"` + s.evaluationText() + `"`} },
			sentinel:  simplified.ErrUnsupportedDatatype,
		},
		{
			name: "a coded value type the template does not complete",
			wt:   alternatives, opts: []simplified.Option{simplified.WithTemplate(alternativesCompiled)},
			leaves: func(s siblingSpelling) map[string]any {
				key := contextCoded + "/" + s.alternative
				return map[string]any{key + "|code": "at0007", key + "|value": "value2"}
			},
			flatNamed: func(s siblingSpelling) []string { return []string{`"` + contextCoded + "/" + s.alternative + `"`} },
			sentinel:  simplified.ErrUnsupportedDatatype,
		},
		{
			name: "a merged careflow step with no value",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				step := s.actionPath() + "/ism_transition/careflow_step"
				return map[string]any{step + "|code": "at9999", step + "|terminology": "SNOMED-CT"}
			},
			// The path is the Web Template node's, which carries no index.
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"test_all_types/` + s.section + "/section_2/section_3/" + s.action + `/ism_transition/careflow_step"`}
			},
			sentinel: simplified.ErrUnsupportedDatatype,
		},
		{
			name: "two spellings of one bare value",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.evaluationText(): "a", s.evaluationText() + "|value": "b"}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + s.evaluationText() + `"`, `"` + s.evaluationText() + `|value"`}
			},
			sentinel: simplified.ErrUnknownPath,
		},
		{
			name: "two value types of one ELEMENT",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				choice := s.evaluationPath() + "/choice"
				return map[string]any{choice + "/quantity_value|magnitude": 1, choice + "/quantity_value|unit": "mm", choice + "/count_value": 2}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{
					`"` + s.evaluationPath() + `/choice/count_value"`,
					`"` + s.evaluationPath() + `/choice/quantity_value"`,
					`"test_all_types:0/` + s.evaluation + `:0/choice:0"`,
				}
			},
			sentinel: simplified.ErrUnknownPath,
		},
		{
			name: "two value types of one ELEMENT, one a sibling",
			wt:   alternatives,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{
					contextCoded + "/coded_text_value|code": "at0007", contextCoded + "/coded_text_value|value": "value2",
					contextCoded + "/" + s.alternative + "|code": "at0007", contextCoded + "/" + s.alternative + "|value": "value2",
				}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + contextCoded + "/" + s.alternative + `"`, "its value type " + s.alternative}
			},
			sentinel: simplified.ErrUnknownPath,
		},
		{
			name: "an index past a single-valued attribute's bound",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.actionPath() + "/ism_transition:1/current_state|code": "526"}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + s.actionPath() + `/ism_transition:1/current_state"`}
			},
			sentinel: simplified.ErrUnknownPath,
		},
		{
			name: "an index past the bound of a sibling value type",
			wt:   alternatives,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{contextCoded + "/" + s.alternative + ":1|code": "at0007"}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + contextCoded + "/" + s.alternative + `:1"`, `FLAT id "` + s.alternative + `"`}
			},
			sentinel: simplified.ErrUnknownPath,
		},
		{
			name: "an ISM transition attribute the SDK does not model",
			wt:   allTypes,
			leaves: func(s siblingSpelling) map[string]any {
				return map[string]any{s.actionPath() + "/planned/transition|code": "541"}
			},
			flatNamed: func(s siblingSpelling) []string {
				return []string{`"` + s.actionPath() + `/planned/transition|code"`}
			},
			sentinel: simplified.ErrNotModelled,
		},
	}
	for _, f := range formats {
		for _, tc := range tests {
			t.Run(f.name+"/"+tc.name, func(t *testing.T) {
				for _, spelled := range []struct {
					name           string
					written, other siblingSpelling
				}{
					{name: "reference spelling", written: referenceSpelling, other: webTemplateSpelling},
					{name: "Web Template spelling", written: webTemplateSpelling, other: referenceSpelling},
				} {
					_, err := decodeFormat(t, f.name, tc.wt, completionBody(tc.leaves(spelled.written)), tc.opts...)
					if !errors.Is(err, tc.sentinel) {
						t.Fatalf("%s: decode = %v, want an error wrapping %v", spelled.name, err, tc.sentinel)
					}
					msg := err.Error()
					for _, id := range spelled.other.ids() {
						if containsID(msg, id) {
							t.Errorf("%s: refusal %q names %q, an id the body does not write", spelled.name, msg, id)
						}
					}
					if f.name != "FLAT" {
						continue
					}
					for _, named := range tc.flatNamed(spelled.written) {
						if !strings.Contains(msg, named) {
							t.Errorf("%s: refusal %q does not name %s", spelled.name, msg, named)
						}
					}
				}
			})
		}
	}
}

// containsID reports whether msg holds id as a whole id: not followed by a
// letter, a digit or an underscore, so test_all_types_1 is not found in
// test_all_types_10.
func containsID(msg, id string) bool {
	for i := 0; ; {
		j := strings.Index(msg[i:], id)
		if j < 0 {
			return false
		}
		end := i + j + len(id)
		if end == len(msg) || !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789_", rune(msg[end])) {
			return true
		}
		i = end
	}
}
