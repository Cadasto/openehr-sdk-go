package instance_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// uidStream is the PCG stream the reproducibility checks build each
// UIDsFrom source on, apart from corpusStream, which the value sources
// use.
const uidStream = 20261022

// seededOptions returns opts with a UIDSource from UIDsFrom and a
// ValueSource, each on a new PCG in the state seed names, so two calls
// with one seed give sources in the same state.
func seededOptions(opts instance.Options, seed uint64) instance.Options {
	opts.UIDSource = instance.UIDsFrom(mrand.NewPCG(seed, uidStream))
	opts.ValueSource = mrand.NewPCG(seed, corpusStream)
	return opts
}

// generateErrors are the errors Generate documents, which tell the class
// of a refusal apart.
var generateErrors = []error{
	instance.ErrNilCompiled,
	instance.ErrUnsupportedPolicy,
	instance.ErrComposerRequired,
	instance.ErrTerritoryRequired,
	instance.ErrArchetypeIDMissing,
	instance.ErrSlotFillUnsupported,
	instance.ErrConstraintUnsatisfiable,
}

// errorClass names the documented error err wraps, or quotes err when it
// wraps none of them.
func errorClass(err error) string {
	for _, sentinel := range generateErrors {
		if errors.Is(err, sentinel) {
			return sentinel.Error()
		}
	}
	return fmt.Sprintf("unclassified %q", err)
}

// uidValues returns the value of every uid member in a decoded tree.
func uidValues(v any) []string {
	var out []string
	switch node := v.(type) {
	case map[string]any:
		if uid, ok := node["uid"].(map[string]any); ok {
			if s, ok := uid["value"].(string); ok {
				out = append(out, s)
			}
		}
		for _, child := range node {
			out = append(out, uidValues(child)...)
		}
	case []any:
		for _, child := range node {
			out = append(out, uidValues(child)...)
		}
	}
	return out
}

// maskUIDs replaces every uid member of a decoded tree, in place, with
// the same placeholder, so two trees that differ only in their uids
// compare equal.
func maskUIDs(v any) {
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if k == "uid" {
				node[k] = "masked"
				continue
			}
			maskUIDs(child)
		}
	case []any:
		for _, child := range node {
			maskUIDs(child)
		}
	}
}

// firstJSONDifference names the first path, in sorted member order, at
// which two decoded trees differ, with the two values there, or returns
// "" when they are equal.
func firstJSONDifference(a, b any, path string) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: an object against %v", path, b)
		}
		for _, k := range slices.Sorted(maps.Keys(x)) {
			if _, ok := y[k]; !ok {
				return fmt.Sprintf("%s/%s: %v against no member", path, k, x[k])
			}
			if d := firstJSONDifference(x[k], y[k], path+"/"+k); d != "" {
				return d
			}
		}
		for _, k := range slices.Sorted(maps.Keys(y)) {
			if _, ok := x[k]; !ok {
				return fmt.Sprintf("%s/%s: no member against %v", path, k, y[k])
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok {
			return fmt.Sprintf("%s: an array against %v", path, b)
		}
		for i := range min(len(x), len(y)) {
			if d := firstJSONDifference(x[i], y[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
		if len(x) != len(y) {
			return fmt.Sprintf("%s: %d members against %d", path, len(x), len(y))
		}
	default:
		if !reflect.DeepEqual(a, b) {
			return fmt.Sprintf("%s: %v against %v", path, a, b)
		}
	}
	return ""
}

// decodeJSON decodes canonical JSON bytes back to any.
func decodeJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return tree
}

// TestREQ107_SeededGenerateReturnsEqualRoots is the REQ-107 Reproducible
// uids check, over every vendored template that parses and compiles, that
// two Generate calls with the same options, a fixed non-zero Now
// included, and a UIDSource from UIDsFrom and a ValueSource each built on
// a source in the same state, return roots with equal canonical JSON. It
// holds under every policy, ExampleFill and RandomFill on several seeds.
// A template Generate refuses must be refused by both calls with the same
// class of error, and is otherwise not compared.
func TestREQ107_SeededGenerateReturnsEqualRoots(t *testing.T) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	var compared, refused, uids int
	var unbuilt []string
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			unbuilt = append(unbuilt, ref.Name)
			continue
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			unbuilt = append(unbuilt, ref.Name)
			continue
		}
		composition := c.Root() != nil && c.Root().RMTypeName() == "COMPOSITION"
		for _, setting := range corpusSettings(composition) {
			name := corpusRun{name: ref.Name, opts: setting.opts, seed: setting.seed}.String()
			first, firstErr := instance.Generate(t.Context(), c, seededOptions(setting.opts, setting.seed))
			second, secondErr := instance.Generate(t.Context(), c, seededOptions(setting.opts, setting.seed))
			if firstErr != nil || secondErr != nil {
				switch {
				case firstErr == nil || secondErr == nil:
					t.Errorf("%s: one seeded Generate call returned error %v and the other %v, want both or neither", name, firstErr, secondErr)
				case errorClass(firstErr) != errorClass(secondErr):
					t.Errorf("%s: two seeded Generate calls were refused with %s and %s, want the same class", name, errorClass(firstErr), errorClass(secondErr))
				}
				refused++
				continue
			}
			firstJSON, err := canjson.Marshal(first)
			if err != nil {
				t.Fatalf("%s: canjson.Marshal: %v", name, err)
			}
			secondJSON, err := canjson.Marshal(second)
			if err != nil {
				t.Fatalf("%s: canjson.Marshal: %v", name, err)
			}
			compared++
			firstTree := decodeJSON(t, firstJSON)
			uids += len(uidValues(firstTree))
			if !bytes.Equal(firstJSON, secondJSON) {
				diff := firstJSONDifference(firstTree, decodeJSON(t, secondJSON), "")
				t.Errorf("%s: two seeded Generate calls returned different roots, first difference at %s", name, diff)
			}
		}
	}
	t.Logf("compared %d pairs of roots holding %d uids; %d settings refused by both calls; %d of %d templates do not parse or compile: %v",
		compared, uids, refused, len(unbuilt), len(refs), unbuilt)
	if compared == 0 || uids == 0 {
		t.Fatalf("compared %d pairs of roots holding %d uids, want some of each", compared, uids)
	}
}

// vitalSignsRandomLarge returns the options the ValueSource checks
// generate vital_signs under: Large and RandomFill, with uids and values
// drawn as given.
func vitalSignsRandomLarge(uids func() *rm.HierObjectID, values mrand.Source) instance.Options {
	return instance.Options{
		Policy:      instance.Large,
		ValueFill:   instance.RandomFill,
		Language:    "en",
		Territory:   "NL",
		Composer:    testComposer(),
		Now:         defaultsNow,
		UIDSource:   uids,
		ValueSource: values,
	}
}

// TestREQ107_NoUIDDrawnFromValueSource is the REQ-107 Reproducible uids
// check that the generator never draws a uid from ValueSource. On
// vital_signs under Large and RandomFill, a run with UIDSource nil draws
// as many values from ValueSource as a run whose UIDSource is UIDsFrom on
// a separate source, and gives the same tree once the uids are masked; and
// two runs with UIDSource nil and value sources in the same state give
// different uids, so the random fallback, not ValueSource, issues them.
func TestREQ107_NoUIDDrawnFromValueSource(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	run := func(uids func() *rm.HierObjectID) (tree any, draws int) {
		t.Helper()
		values := &countingSource{src: mrand.NewPCG(42, 1)}
		out, err := instance.Generate(t.Context(), c, vitalSignsRandomLarge(uids, values))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return decodedJSON(t, out), values.draws
	}
	fallback, fallbackDraws := run(nil)
	seeded, seededDraws := run(instance.UIDsFrom(mrand.NewPCG(42, 2)))
	again, _ := run(nil)

	fallbackUIDs := uidValues(fallback)
	if len(fallbackUIDs) == 0 {
		t.Fatal("vital_signs under Large carries no uid, so nothing was checked")
	}
	if fallbackDraws != seededDraws {
		t.Errorf("ValueSource draws: %d with UIDSource nil, %d with UIDsFrom on a separate source, want equal", fallbackDraws, seededDraws)
	}
	for _, s := range uidValues(again) {
		if slices.Contains(fallbackUIDs, s) {
			t.Errorf("two runs with UIDSource nil and value sources in the same state both issued uid %q, want the random fallback's", s)
		}
	}
	maskUIDs(fallback)
	maskUIDs(seeded)
	if d := firstJSONDifference(fallback, seeded, ""); d != "" {
		t.Errorf("with uids masked, UIDSource nil and UIDsFrom on a separate source give different trees, first difference at %s", d)
	}
}

// TestREQ107_SharedSourceStaysReproducible is the REQ-107 Reproducible
// uids check that one source shared between ValueSource and UIDsFrom
// stays reproducible: two runs on vital_signs under Large and RandomFill,
// each on a shared source in the same state, return roots with equal
// canonical JSON.
func TestREQ107_SharedSourceStaysReproducible(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	run := func() []byte {
		t.Helper()
		src := mrand.NewPCG(42, 1)
		out, err := instance.Generate(t.Context(), c, vitalSignsRandomLarge(instance.UIDsFrom(src), src))
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		raw, err := canjson.Marshal(out)
		if err != nil {
			t.Fatalf("canjson.Marshal: %v", err)
		}
		return raw
	}
	first, second := run(), run()
	if !bytes.Equal(first, second) {
		t.Errorf("two runs on a shared source in the same state returned different roots, first difference at %s",
			firstJSONDifference(decodeJSON(t, first), decodeJSON(t, second), ""))
	}
}
