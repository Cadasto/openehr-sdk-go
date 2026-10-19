package composition_test

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// policyNow is the clock the skeleton-policy tests pin, so two trees of
// one template under one policy differ only in their uids.
var policyNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// policyOptions returns the settings the skeleton-policy tests build
// vital_signs under (a territory, a composer and a fixed clock),
// followed by extra.
func policyOptions(extra ...composition.Option) []composition.Option {
	return append([]composition.Option{
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
		composition.WithNow(policyNow),
	}, extra...)
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

// archetypeID returns the archetype_details.archetype_id.value of a
// decoded locatable, or "" when it carries none.
func archetypeID(v any) string {
	node, _ := v.(map[string]any)
	details, _ := node["archetype_details"].(map[string]any)
	id, _ := details["archetype_id"].(map[string]any)
	s, _ := id["value"].(string)
	return s
}

// archetypeIDCounts counts the archetype id of every locatable in a
// decoded tree.
func archetypeIDCounts(v any, into map[string]int) {
	switch node := v.(type) {
	case map[string]any:
		if id := archetypeID(node); id != "" {
			into[id]++
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

// contentArchetypeIDs returns the sorted archetype ids of the members of
// a decoded composition's content.
func contentArchetypeIDs(tree any) []string {
	root, _ := tree.(map[string]any)
	content, _ := root["content"].([]any)
	ids := make([]string, 0, len(content))
	for _, item := range content {
		ids = append(ids, archetypeID(item))
	}
	slices.Sort(ids)
	return ids
}

// TestREQ101_REQ107_NewSkeletonDefaultsToMedium checks REQ-101's
// skeleton policy when no WithPolicy is given: NewSkeleton builds the
// tree REQ-107's Medium, the instance package's zero Policy, builds. On
// vital_signs that tree holds the first of the template's four optional
// content OBSERVATIONs alone, where Large builds a member of each and
// Required none, and as many JSON values as instance.Generate under
// Medium. Of two optional siblings that share an at-code, the default
// builds the first alone, as Large and Medium do.
func TestREQ101_REQ107_NewSkeletonDefaultsToMedium(t *testing.T) {
	t.Parallel()
	t.Run("vital_signs", func(t *testing.T) {
		t.Parallel()
		c := compileFixture(t, "vital_signs")
		comp, err := composition.NewSkeleton(t.Context(), c, policyOptions()...)
		if err != nil {
			t.Fatalf("NewSkeleton(vital_signs) with no WithPolicy: %v", err)
		}
		tree := decodedJSON(t, comp)

		want := []string{"openEHR-EHR-OBSERVATION.blood_pressure.v1"}
		if got := contentArchetypeIDs(tree); !slices.Equal(got, want) {
			t.Errorf("NewSkeleton(vital_signs) with no WithPolicy: content archetype ids = %v, want %v", got, want)
		}

		medium, err := instance.Generate(t.Context(), c, instance.Options{
			Policy:    instance.Medium,
			Territory: "NL",
			Composer:  testComposer(),
			Now:       policyNow,
		})
		if err != nil {
			t.Fatalf("Generate(vital_signs, Medium): %v", err)
		}
		if got, want := jsonValueCount(tree), jsonValueCount(decodedJSON(t, medium)); got != want {
			t.Errorf("NewSkeleton(vital_signs) with no WithPolicy holds %d JSON values, want Generate(Medium)'s %d", got, want)
		}

		large, err := composition.NewSkeleton(t.Context(), c, policyOptions(composition.WithPolicy(instance.Large))...)
		if err != nil {
			t.Fatalf("NewSkeleton(vital_signs, WithPolicy(Large)): %v", err)
		}
		wantLarge := []string{
			"openEHR-EHR-OBSERVATION.blood_pressure.v1",
			"openEHR-EHR-OBSERVATION.body_temperature.v1",
			"openEHR-EHR-OBSERVATION.heart_rate.v1",
			"openEHR-EHR-OBSERVATION.respiration.v1",
		}
		if got := contentArchetypeIDs(decodedJSON(t, large)); !slices.Equal(got, wantLarge) {
			t.Errorf("NewSkeleton(vital_signs, WithPolicy(Large)) content archetype ids = %v, want %v, so the default is not Large", got, wantLarge)
		}
	})
	t.Run("optional named siblings", func(t *testing.T) {
		t.Parallel()
		optional := occurrences("0", "1")
		c := compileNamedSiblings(t, optional, optional)
		cases := []struct {
			name string
			opts []composition.Option
			want []string
		}{
			{name: "no WithPolicy", opts: policyOptions(), want: []string{"A"}},
			{name: "WithPolicy(Large)", opts: policyOptions(composition.WithPolicy(instance.Large)), want: []string{"A"}},
			{name: "WithPolicy(Medium)", opts: policyOptions(composition.WithPolicy(instance.Medium)), want: []string{"A"}},
		}
		for _, tc := range cases {
			comp, err := composition.NewSkeleton(t.Context(), c, tc.opts...)
			if err != nil {
				t.Fatalf("NewSkeleton(named siblings) with %s: %v", tc.name, err)
			}
			if got := slices.Sorted(maps.Keys(siblingValues(t, comp))); !slices.Equal(got, tc.want) {
				t.Errorf("NewSkeleton(named siblings) with %s holds siblings %v, want %v", tc.name, got, tc.want)
			}
		}
	})
}

// TestREQ101_REQ107_NewSkeletonAcceptsRequired checks that NewSkeleton
// accepts REQ-107's Required through WithPolicy (REQ-101 skeleton policy)
// and builds only what vital_signs makes mandatory: no content
// OBSERVATION, so the composition's own archetype is the only one the
// tree holds, and strictly fewer JSON values than the default skeleton.
// The tree passes the template validator and the RM floor.
func TestREQ101_REQ107_NewSkeletonAcceptsRequired(t *testing.T) {
	t.Parallel()
	c := compileFixture(t, "vital_signs")
	def, err := composition.NewSkeleton(t.Context(), c, policyOptions()...)
	if err != nil {
		t.Fatalf("NewSkeleton(vital_signs) with no WithPolicy: %v", err)
	}
	comp, err := composition.NewSkeleton(t.Context(), c, policyOptions(composition.WithPolicy(instance.Required))...)
	if err != nil {
		t.Fatalf("NewSkeleton(vital_signs, WithPolicy(Required)) = %v, want nil", err)
	}
	tree := decodedJSON(t, comp)

	got := make(map[string]int)
	archetypeIDCounts(tree, got)
	want := map[string]int{"openEHR-EHR-COMPOSITION.encounter.v1": 1}
	if !maps.Equal(got, want) {
		t.Errorf("NewSkeleton(vital_signs, WithPolicy(Required)) archetype ids = %v, want %v", got, want)
	}

	required, medium := jsonValueCount(tree), jsonValueCount(decodedJSON(t, def))
	t.Logf("vital_signs JSON values: required=%d default=%d", required, medium)
	if required >= medium {
		t.Errorf("NewSkeleton(vital_signs, WithPolicy(Required)) holds %d JSON values, want fewer than the default skeleton's %d",
			required, medium)
	}

	if res := validation.ValidateComposition(comp, c); !res.OK {
		t.Errorf("ValidateComposition(NewSkeleton(vital_signs, WithPolicy(Required))) OK = false: %v", res.Issues)
	}
	if res := validation.ValidateRM(comp); !res.OK {
		t.Errorf("ValidateRM(NewSkeleton(vital_signs, WithPolicy(Required))) OK = false: %v", res.Issues)
	}
}

// TestREQ101_REQ107_NewBuilderAcceptsOnlyLarge checks REQ-101's skeleton
// policy for NewBuilder: Set routes a value only into a node the skeleton
// already holds, so it needs a member of every optional archetype, which
// only REQ-107's Large builds. NewBuilder refuses Medium, whose skeleton
// holds one optional archetype under an attribute, also given explicitly
// as the zero value it is, and Required, whose skeleton holds none, and so
// Minimal, with an error wrapping ErrUnsupportedPolicy that names the
// policy, and no Builder. That error does not wrap
// instance.ErrUnsupportedPolicy, since REQ-107 defines both levels. It
// accepts Large, and so Complete. The Large
// skeleton holds the heart-rate OBSERVATION of vital_signs, the third of
// its four optional content archetypes, so a value set there builds
// without error.
func TestREQ101_REQ107_NewBuilderAcceptsOnlyLarge(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		policy  instance.Policy
		refused bool
	}{
		{name: "Required", policy: instance.Required, refused: true},
		{name: "Minimal", policy: instance.Minimal, refused: true},
		{name: "Medium", policy: instance.Medium, refused: true},
		{name: "Large", policy: instance.Large},
		{name: "Complete", policy: instance.Complete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
			b, err := composition.NewBuilder(t.Context(), c, policyOptions(composition.WithPolicy(tc.policy))...)
			if tc.refused {
				if !errors.Is(err, composition.ErrUnsupportedPolicy) {
					t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) error = %v, want one wrapping ErrUnsupportedPolicy", tc.policy, err)
				}
				if errors.Is(err, instance.ErrUnsupportedPolicy) {
					t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) error = %v wraps instance.ErrUnsupportedPolicy, want it not to: the instance package defines %v", tc.policy, err, tc.policy)
				}
				if err != nil && !strings.Contains(err.Error(), tc.policy.String()) {
					t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) error = %q, want it to name the policy %q", tc.policy, err, tc.policy)
				}
				if b != nil {
					t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) returned a Builder with the refusal, want nil", tc.policy)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewBuilder(vital_signs, WithPolicy(%v)) = %v, want nil", tc.policy, err)
			}
			if err := b.Set(heartRatePath, &rm.DVQuantity{Magnitude: 80, Units: "/min"}); err != nil {
				t.Fatalf("Set(%s) under %v = %v, want nil", heartRatePath, tc.policy, err)
			}
			if _, err := b.Build(); err != nil {
				t.Errorf("Build after Set(%s) under %v = %v, want nil", heartRatePath, tc.policy, err)
			}
		})
	}
}

// TestREQ101_NewBuilderDefaultsToLarge checks REQ-101's skeleton policy
// for NewBuilder when no WithPolicy is given: it builds on REQ-107's Large,
// whatever the instance package's zero Policy is. On vital_signs its
// skeleton holds a member of each of the four optional content
// OBSERVATIONs, and as many JSON values as instance.Generate under Large.
// That an explicit WithPolicy of another level is refused, the zero value
// included, is TestREQ101_REQ107_NewBuilderAcceptsOnlyLarge's check.
func TestREQ101_NewBuilderDefaultsToLarge(t *testing.T) {
	t.Parallel()
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(t.Context(), c, policyOptions()...)
	if err != nil {
		t.Fatalf("NewBuilder(vital_signs) with no WithPolicy = %v, want nil", err)
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	tree := decodedJSON(t, skeleton)

	want := []string{
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
		"openEHR-EHR-OBSERVATION.body_temperature.v1",
		"openEHR-EHR-OBSERVATION.heart_rate.v1",
		"openEHR-EHR-OBSERVATION.respiration.v1",
	}
	if got := contentArchetypeIDs(tree); !slices.Equal(got, want) {
		t.Errorf("NewBuilder(vital_signs) with no WithPolicy: skeleton content archetype ids = %v, want %v", got, want)
	}
	large, err := instance.Generate(t.Context(), c, instance.Options{
		Policy:    instance.Large,
		Territory: "NL",
		Composer:  testComposer(),
		Now:       policyNow,
	})
	if err != nil {
		t.Fatalf("Generate(vital_signs, Large): %v", err)
	}
	if got, want := jsonValueCount(tree), jsonValueCount(decodedJSON(t, large)); got != want {
		t.Errorf("NewBuilder(vital_signs) with no WithPolicy: skeleton holds %d JSON values, want Generate(Large)'s %d", got, want)
	}
}

// undefinedPolicies are Policy values REQ-107 does not define: one past
// its last level, one below its first and one far past it. Policy's
// String names each "unknown", so a refusal prints their number.
var undefinedPolicies = []instance.Policy{instance.Policy(3), instance.Policy(-1), instance.Policy(99)}

// TestREQ101_NewBuilderRefusesAnUndefinedPolicy checks REQ-101's skeleton
// policy for a Policy value that is none of REQ-107's three: NewBuilder
// refuses it with an error wrapping ErrUnsupportedPolicy and
// instance.ErrUnsupportedPolicy that prints the value's number, and
// returns no Builder. NewSkeleton refuses such a value too, so the
// error's composition.NewBuilder prefix is what shows that NewBuilder
// refused it itself, rather than passing it on to NewSkeleton.
func TestREQ101_NewBuilderRefusesAnUndefinedPolicy(t *testing.T) {
	t.Parallel()
	for _, p := range undefinedPolicies {
		n := strconv.Itoa(int(p))
		t.Run(n, func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
			b, err := composition.NewBuilder(t.Context(), c, policyOptions(composition.WithPolicy(p))...)
			if b != nil {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%s)) returned a Builder, want nil", n)
			}
			if !errors.Is(err, composition.ErrUnsupportedPolicy) {
				t.Fatalf("NewBuilder(vital_signs, WithPolicy(%s)) error = %v, want one wrapping ErrUnsupportedPolicy", n, err)
			}
			if !errors.Is(err, instance.ErrUnsupportedPolicy) {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%s)) error = %v, want one wrapping instance.ErrUnsupportedPolicy too", n, err)
			}
			if msg := err.Error(); !strings.HasPrefix(msg, "composition.NewBuilder: ") || !strings.HasSuffix(msg, ": "+n) {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%s)) error = %q, want NewBuilder's own refusal ending in the number %s", n, msg, n)
			}
		})
	}
}

// TestREQ101_NewSkeletonRefusesAnUndefinedPolicy checks REQ-101's
// skeleton policy for NewSkeleton: a Policy value that is none of
// REQ-107's three is refused with an error wrapping ErrUnsupportedPolicy
// and instance.ErrUnsupportedPolicy that prints the value's number, and
// no composition.
func TestREQ101_NewSkeletonRefusesAnUndefinedPolicy(t *testing.T) {
	t.Parallel()
	for _, p := range undefinedPolicies {
		n := strconv.Itoa(int(p))
		t.Run(n, func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
			comp, err := composition.NewSkeleton(t.Context(), c, policyOptions(composition.WithPolicy(p))...)
			if comp != nil {
				t.Errorf("NewSkeleton(vital_signs, WithPolicy(%s)) returned a composition, want nil", n)
			}
			if !errors.Is(err, composition.ErrUnsupportedPolicy) {
				t.Fatalf("NewSkeleton(vital_signs, WithPolicy(%s)) error = %v, want one wrapping ErrUnsupportedPolicy", n, err)
			}
			if !errors.Is(err, instance.ErrUnsupportedPolicy) {
				t.Errorf("NewSkeleton(vital_signs, WithPolicy(%s)) error = %v, want one wrapping instance.ErrUnsupportedPolicy too", n, err)
			}
			if msg := err.Error(); !strings.HasSuffix(msg, ": "+n) {
				t.Errorf("NewSkeleton(vital_signs, WithPolicy(%s)) error = %q, want it to end in the number %s", n, msg, n)
			}
		})
	}
}

// TestREQ101_NewBuilderRefusesBeforeBuildingASkeleton checks that
// NewBuilder refuses REQ-107's Medium and Required before it builds a
// skeleton (REQ-101 skeleton policy). With no composer and no territory,
// building the skeleton fails with instance.ErrComposerRequired or
// instance.ErrTerritoryRequired, so a refusal made after that step would
// return one of those instead of ErrUnsupportedPolicy.
func TestREQ101_NewBuilderRefusesBeforeBuildingASkeleton(t *testing.T) {
	t.Parallel()
	for _, policy := range []instance.Policy{instance.Medium, instance.Required} {
		t.Run(policy.String(), func(t *testing.T) {
			t.Parallel()
			c := compileFixture(t, "vital_signs")
			b, err := composition.NewBuilder(t.Context(), c, composition.WithPolicy(policy))
			if b != nil {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) with no composer returned a Builder, want nil", policy)
			}
			if !errors.Is(err, composition.ErrUnsupportedPolicy) {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) with no composer: error = %v, want one wrapping ErrUnsupportedPolicy", policy, err)
			}
			if errors.Is(err, instance.ErrComposerRequired) || errors.Is(err, instance.ErrTerritoryRequired) {
				t.Errorf("NewBuilder(vital_signs, WithPolicy(%v)) with no composer: error = %v, want the refusal before a skeleton is built", policy, err)
			}
		})
	}
}
