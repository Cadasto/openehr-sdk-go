package instance_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// corpusPolicyOptions are the settings a corpus composition case runs
// under for one policy: a territory, a composer and a fixed clock.
func corpusPolicyOptions(policy instance.Policy) instance.Options {
	return instance.Options{
		Policy:    policy,
		Territory: "NL",
		Composer:  testComposer(),
		Now:       defaultsNow,
	}
}

// decodedJSON returns v's canonical JSON decoded back to any.
func decodedJSON(t *testing.T, v any) any {
	t.Helper()
	raw, err := canjson.Marshal(v)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return tree
}

// jsonValueCount counts every JSON value in a decoded tree: each object,
// each array and each scalar. Object keys are not values.
func jsonValueCount(v any) int {
	n := 1
	switch node := v.(type) {
	case map[string]any:
		for _, child := range node {
			n += jsonValueCount(child)
		}
	case []any:
		for _, child := range node {
			n += jsonValueCount(child)
		}
	}
	return n
}

// archetypeIDCounts counts the archetype_details.archetype_id.value of
// every locatable in a decoded tree.
func archetypeIDCounts(v any, into map[string]int) {
	switch node := v.(type) {
	case map[string]any:
		if details, ok := node["archetype_details"].(map[string]any); ok {
			if id, ok := details["archetype_id"].(map[string]any); ok {
				if s, ok := id["value"].(string); ok {
					into[s]++
				}
			}
		}
		for _, child := range node {
			archetypeIDCounts(child, into)
		}
	case []any:
		for _, child := range node {
			archetypeIDCounts(child, into)
		}
	}
}

// policyLevel is what one policy builds on a corpus template: the number
// of JSON values in its canonical JSON and the archetype ids it holds.
type policyLevel struct {
	values     int
	archetypes map[string]int
}

// TestREQ107_PoliciesBuildTheirLevels is the REQ-107 check, on the corpus
// templates vital_signs and social, that the three policies build the
// three levels of the ITS-REST template example: Required holds fewer JSON
// values than Medium, and Medium fewer than Large. Large builds a member of
// every optional archetype the template names; Medium builds at most one
// optional archetype under an attribute, so each composition's content
// holds only its first OBSERVATION, with what that OBSERVATION holds;
// Required builds no optional archetype, only the composition's own and
// its context. Each tree passes the template validator and the RM floor
// (PROBE-027).
func TestREQ107_PoliciesBuildTheirLevels(t *testing.T) {
	const encounter = "openEHR-EHR-COMPOSITION.encounter.v1"
	cases := []struct {
		name string
		want map[instance.Policy]policyLevel
	}{
		{
			name: "vital_signs",
			want: map[instance.Policy]policyLevel{
				instance.Large: {values: 516, archetypes: map[string]int{
					encounter: 1,
					"openEHR-EHR-OBSERVATION.blood_pressure.v1":       1,
					"openEHR-EHR-OBSERVATION.body_temperature.v1":     1,
					"openEHR-EHR-OBSERVATION.heart_rate.v1":           1,
					"openEHR-EHR-OBSERVATION.respiration.v1":          1,
					"openEHR-EHR-CLUSTER.device.v1":                   2,
					"openEHR-EHR-CLUSTER.environmental_conditions.v1": 1,
				}},
				instance.Medium: {values: 186, archetypes: map[string]int{
					encounter: 1,
					"openEHR-EHR-OBSERVATION.blood_pressure.v1": 1,
					"openEHR-EHR-CLUSTER.device.v1":             1,
				}},
				instance.Required: {values: 53, archetypes: map[string]int{encounter: 1}},
			},
		},
		{
			name: "social",
			want: map[instance.Policy]policyLevel{
				instance.Large: {values: 673, archetypes: map[string]int{
					encounter: 1,
					"openEHR-EHR-OBSERVATION.livingsituation.v1":        1,
					"openEHR-EHR-OBSERVATION.languageproficiency.v1":    1,
					"openEHR-EHR-OBSERVATION.participationinsociety.v1": 1,
					"openEHR-EHR-OBSERVATION.helpfromothers.v1":         1,
					"openEHR-EHR-EVALUATION.familysituation.v1":         1,
					"openEHR-EHR-EVALUATION.education_summary.v1":       1,
					"openEHR-EHR-CLUSTER.contact_persons_context.v1":    1,
				}},
				instance.Medium: {values: 139, archetypes: map[string]int{
					encounter: 1,
					"openEHR-EHR-OBSERVATION.livingsituation.v1": 1,
				}},
				instance.Required: {values: 53, archetypes: map[string]int{encounter: 1}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileFixture(t, tc.name)
			got := make(map[instance.Policy]int)
			for _, policy := range everyPolicy {
				comp := generateComposition(t, c, corpusPolicyOptions(policy))
				tree := decodedJSON(t, comp)
				archetypes := make(map[string]int)
				archetypeIDCounts(tree, archetypes)
				got[policy] = jsonValueCount(tree)
				want := tc.want[policy]
				if got[policy] != want.values {
					t.Errorf("Generate(%s, %v) holds %d JSON values, want %d", tc.name, policy, got[policy], want.values)
				}
				if !maps.Equal(archetypes, want.archetypes) {
					t.Errorf("Generate(%s, %v) archetype ids = %v, want %v", tc.name, policy, archetypes, want.archetypes)
				}
				if comp.Context == nil || comp.Context.StartTime.Value == "" {
					t.Errorf("Generate(%s, %v) context = %+v, want an EVENT_CONTEXT with a start_time", tc.name, policy, comp.Context)
				}
				if res := validation.ValidateComposition(comp, c); !res.OK {
					t.Errorf("ValidateComposition(Generate(%s, %v)) OK = false: %v", tc.name, policy, res.Issues)
				}
				if res := validation.ValidateRM(comp); !res.OK {
					t.Errorf("ValidateRM(Generate(%s, %v)) OK = false: %v", tc.name, policy, res.Issues)
				}
			}
			if got[instance.Required] >= got[instance.Medium] || got[instance.Medium] >= got[instance.Large] {
				t.Errorf("Generate(%s) JSON values: required=%d medium=%d large=%d, want required < medium < large",
					tc.name, got[instance.Required], got[instance.Medium], got[instance.Large])
			}
		})
	}
}

// TestREQ107_ZeroPolicyBuildsMedium is the REQ-107 check of Levels and
// names on vital_signs: Options with no Policy builds the medium level, the
// tree Medium builds in TestREQ107_PoliciesBuildTheirLevels, with 186 JSON
// values and, besides the composition's own archetype, only the first
// optional content OBSERVATION and the device CLUSTER it holds.
func TestREQ107_ZeroPolicyBuildsMedium(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	comp := generateComposition(t, c, instance.Options{
		Territory: "NL",
		Composer:  testComposer(),
		Now:       defaultsNow,
	})
	tree := decodedJSON(t, comp)
	if got, want := jsonValueCount(tree), 186; got != want {
		t.Errorf("Generate(vital_signs) with no Policy holds %d JSON values, want Medium's %d", got, want)
	}
	got := make(map[string]int)
	archetypeIDCounts(tree, got)
	want := map[string]int{
		"openEHR-EHR-COMPOSITION.encounter.v1":      1,
		"openEHR-EHR-OBSERVATION.blood_pressure.v1": 1,
		"openEHR-EHR-CLUSTER.device.v1":             1,
	}
	if !maps.Equal(got, want) {
		t.Errorf("Generate(vital_signs) with no Policy archetype ids = %v, want Medium's %v", got, want)
	}
}

// TestREQ107_RequiredHoldsNoOptionalContentArchetype is the REQ-107 check
// that Required builds no member of an optional archetype the template
// names: vital_signs and social give COMPOSITION.content an existence of
// 0..1, and each content OBSERVATION or EVALUATION occurrences of 0..*
// (four OBSERVATIONs in vital_signs; four OBSERVATIONs and two EVALUATIONs
// in social), so the only archetype the tree holds is the composition's
// own, once.
func TestREQ107_RequiredHoldsNoOptionalContentArchetype(t *testing.T) {
	for _, name := range []string{"vital_signs", "social"} {
		t.Run(name, func(t *testing.T) {
			c := compileFixture(t, name)
			comp := generateComposition(t, c, corpusPolicyOptions(instance.Required))
			got := make(map[string]int)
			archetypeIDCounts(decodedJSON(t, comp), got)
			want := map[string]int{"openEHR-EHR-COMPOSITION.encounter.v1": 1}
			if !maps.Equal(got, want) {
				t.Errorf("Generate(%s, Required) archetype ids = %v, want %v", name, got, want)
			}
			if len(comp.Content) != 0 {
				t.Errorf("Generate(%s, Required) content = %v, want none", name, contentIDs(comp.Content))
			}
		})
	}
}

// TestREQ107_RequiredSkipsAnOptionalAttributeWithAnAllowedChild is the
// REQ-107 check of the visit rule's Required arm: an attribute that is
// optional (not BMM-mandatory, existence or cardinality lower bound 0)
// is not visited under Required, although the OPT names an allowed child
// under it, while Large and Medium visit it. Each case holds in both
// compile modes, and Required's output passes the RM floor and the
// template validator.
func TestREQ107_RequiredSkipsAnOptionalAttributeWithAnAllowedChild(t *testing.T) {
	cases := []struct {
		name    string
		opt     string
		present func(t *testing.T, out any) bool
	}{
		{
			name: "single-valued OBSERVATION protocol",
			opt:  optTemplate("OBSERVATION", optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0001"))),
			present: func(t *testing.T, out any) bool {
				t.Helper()
				o, ok := out.(*rm.Observation)
				if !ok {
					t.Fatalf("generated root is %T, want *rm.Observation", out)
				}
				return o.Protocol != nil && !rm.IsTypedNil(o.Protocol)
			},
		},
		{
			name: "multi-valued ITEM_TREE items",
			opt: optTemplate("ITEM_TREE", optMultipleLowerZero("items",
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1))),
			present: func(t *testing.T, out any) bool {
				t.Helper()
				return len(itemIDs(t, out)) > 0
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					want := policy != instance.Required
					if got := tc.present(t, out); got != want {
						t.Errorf("Generate(%v): attribute present = %t, want %t", policy, got, want)
					}
					if policy == instance.Required {
						checkValid(t, out, c)
					}
				})
			}
		}
	}
}

// TestREQ107_RequiredSizesMembersByOccurrencesLower is the REQ-107 check
// that, under Required, a visited multi-valued attribute gets
// occurrences.lower members of each allowed OPT child that is not a slot,
// so an optional child gets none of its own, and is still topped up to its
// cardinality lower bound with members of its first allowed child that is
// not a slot. Large and Medium give each child max(occurrences.lower, 1).
// Each case
// holds in both compile modes, and the output passes the RM floor and the
// template validator.
func TestREQ107_RequiredSizesMembersByOccurrencesLower(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want map[instance.Policy][]string
	}{
		{
			name: "occurrences lower 2 beside an optional child",
			opt: optTemplate("CLUSTER", optCardinal("items", 1, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 2, -1),
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 1))),
			want: map[instance.Policy][]string{
				instance.Large:    {"at0001", "at0001", "at0002"},
				instance.Medium:   {"at0001", "at0001", "at0002"},
				instance.Required: {"at0001", "at0001"},
			},
		},
		{
			name: "cardinality lower 1 over optional children only",
			opt: optTemplate("CLUSTER", optCardinal("items", 1, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1),
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 1))),
			want: map[instance.Policy][]string{
				instance.Large:    {"at0001", "at0002"},
				instance.Medium:   {"at0001", "at0002"},
				instance.Required: {"at0001"},
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got, want := itemIDs(t, out), tc.want[policy]; !slices.Equal(got, want) {
						t.Errorf("Generate(%v): items = %v, want %v", policy, got, want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_RequiredElementWithOptionalValueGetsNullFlavour is the
// REQ-107 check that an RM rule still wins under Required: an ELEMENT
// whose value the OPT leaves optional, with an allowed child, gets no value
// under Required, so it gets the null_flavour openehr 271|no information|
// (Inv_null_flavour_indicated), while Large and Medium build the value. It
// holds in
// both compile modes, and the output passes the RM floor and the template
// validator.
func TestREQ107_RequiredElementWithOptionalValueGetsNullFlavour(t *testing.T) {
	opt := optTemplate("ELEMENT", optOptionalSingleOver("value", optNode("DV_TEXT", "")))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range everyPolicy {
			t.Run(fmt.Sprintf("implicit=%t/%v", implicit, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				el, ok := out.(*rm.Element)
				if !ok {
					t.Fatalf("generated root is %T, want *rm.Element", out)
				}
				hasValue := el.Value != nil && !rm.IsTypedNil(el.Value)
				if want := policy != instance.Required; hasValue != want {
					t.Errorf("Generate(%v): ELEMENT.value present = %t, want %t", policy, hasValue, want)
				}
				if policy == instance.Required {
					if el.NullFlavour == nil {
						t.Fatal("Generate(Required): ELEMENT.null_flavour absent, want openehr::271")
					}
					checkOpenEHRCode(t, "ELEMENT.null_flavour", *el.NullFlavour, terminology.NullFlavours, "271")
				}
				checkValid(t, out, c)
			})
		}
	}
}

// TestREQ107_PolicyValuesAndNames is the REQ-107 check of Levels and
// names: Medium is the zero value, Required follows it, Large comes after
// both, and each names itself for diagnostics.
func TestREQ107_PolicyValuesAndNames(t *testing.T) {
	cases := []struct {
		policy instance.Policy
		value  int
		name   string
	}{
		{policy: instance.Medium, value: 0, name: "medium"},
		{policy: instance.Required, value: 1, name: "required"},
		{policy: instance.Large, value: 2, name: "large"},
		{policy: instance.Policy(99), value: 99, name: "unknown"},
	}
	for _, tc := range cases {
		if int(tc.policy) != tc.value {
			t.Errorf("Policy %s = %d, want %d", tc.name, int(tc.policy), tc.value)
		}
		if got := tc.policy.String(); got != tc.name {
			t.Errorf("Policy(%d).String() = %q, want %q", int(tc.policy), got, tc.name)
		}
	}
}
