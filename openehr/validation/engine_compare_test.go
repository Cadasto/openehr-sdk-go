package validation_test

// engine_compare_test.go: REQ-171 § One walk, § Acceptance — the comparison
// harness. The one walk runs beside the floor until the comparison is done:
// over the vendored corpus, a root of every class the registry knows, the
// constraint-fixture matrix, every probe input and the roots the floor's own
// tests pin, a walk with no template reports the issues ValidateRM reports,
// finding by finding on path, code, layer and rule. Decoded documents, zero
// values, the generator's Example instances and the probe inputs hold no
// shared pointer, so the one walk's only difference from the floor, a shared
// value validated at each place, cannot arise, and the two must agree
// exactly. A root that reaches a walk condition (a cycle, the depth limit,
// the visit limit) is compared the same way, apart from the wording of the
// condition's Detail.

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// findingKey is what makes two issues the same finding for the comparison:
// the path, the code, the layer and the rule.
type findingKey struct {
	path, code string
	layer      validation.Layer
	rule       string
}

// String writes the key as the harness reports it.
func (k findingKey) String() string {
	return fmt.Sprintf("%s %s layer=%q rule=%q", k.code, k.path, k.layer, k.rule)
}

// findingKeys returns the key of each of issues, in result order.
func findingKeys(issues []validation.Issue) []findingKey {
	keys := make([]findingKey, len(issues))
	for i, iss := range issues {
		keys[i] = findingKey{path: iss.Path, code: iss.Code, layer: iss.Layer, rule: iss.Rule}
	}
	return keys
}

// multisetDiff returns, sorted, one line for each finding the two lists do
// not hold the same number of times: how often the floor holds it and how
// often the engine does.
func multisetDiff(floor, engine []findingKey) []string {
	counts := map[findingKey][2]int{}
	for _, k := range floor {
		c := counts[k]
		c[0]++
		counts[k] = c
	}
	for _, k := range engine {
		c := counts[k]
		c[1]++
		counts[k] = c
	}
	var out []string
	for k, c := range counts {
		if c[0] != c[1] {
			out = append(out, fmt.Sprintf("  floor %d, engine %d: %s", c[0], c[1], k))
		}
	}
	slices.Sort(out)
	return out
}

// comparedRoot is one root the harness validates both ways.
type comparedRoot struct {
	name string
	root any
}

// minComparedRoots is the number of roots each group of the harness held
// when it was last counted. A group that holds fewer has lost roots, a
// document that no longer decodes or an Example the generator no longer
// builds, and the harness would pass for that reason rather than for the
// walk it compares.
var minComparedRoots = map[string]int{
	"corpus":              79,
	"generated":           49,
	"ehr_status":          20,
	"constraint_fixtures": 25,
	"probes":              17,
	"floor_pinned":        2,
}

// comparisonRoots returns every root the harness compares, by the group it
// comes from: the vendored corpus documents, the Example instance the
// generator builds for each corpus template, the corpus EHR_STATUS samples,
// the zero value of every RM class the registry knows, the instances of the
// constraint-fixture matrix, the probe inputs, the roots the floor's own
// tests pin, and the roots the guards refuse. It fails the test when a group
// holds fewer roots than [minComparedRoots] says.
func comparisonRoots(t *testing.T) map[string][]comparedRoot {
	t.Helper()
	groups := map[string][]comparedRoot{}

	for _, doc := range corpusDocuments(t) {
		groups["corpus"] = append(groups["corpus"], comparedRoot{name: doc.rel.Rel, root: doc.root})
		if doc.c == nil {
			continue
		}
		if root, err := generateExample(t, doc.c); err == nil {
			groups["generated"] = append(groups["generated"], comparedRoot{name: "Example of " + doc.rel.Template, root: root})
		}
	}

	statuses, err := filepath.Glob(filepath.Join(fixtures.CorpusRoot(), "rm", "ehr_status_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range statuses {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var status rm.EHRStatus
		if err := canjson.Unmarshal(body, &status); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		groups["ehr_status"] = append(groups["ehr_status"], comparedRoot{name: filepath.Base(path), root: &status})
	}

	for _, name := range typereg.Default.Names() {
		ctor, ok := typereg.Default.Lookup(name)
		if !ok {
			t.Fatalf("typereg.Default lists %s but has no constructor for it", name)
		}
		root := ctor()
		typ := reflect.TypeOf(root)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.PkgPath() != rmPackage {
			continue
		}
		groups["registry"] = append(groups["registry"], comparedRoot{name: "zero " + name, root: root})
	}

	ids, err := fixtures.ConstraintTemplateIDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		raw, err := os.ReadFile(fixtures.CompositionJSON(id))
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		var comp rm.Composition
		if err := canjson.Unmarshal(raw, &comp); err != nil {
			t.Fatalf("decode %s: %v", id, err)
		}
		groups["constraint_fixtures"] = append(groups["constraint_fixtures"], comparedRoot{name: id, root: &comp})
	}

	inputs, err := probes.Inputs()
	if err != nil {
		t.Fatalf("probes.Inputs: %v", err)
	}
	for _, in := range inputs {
		groups["probes"] = append(groups["probes"], comparedRoot{name: in.Probe + " " + in.Name, root: in.Root})
	}

	// The floor's own tests pin these: the entries of a Hash attribute,
	// walked in key order and written by their position, one of them broken;
	// and a list of a primitive type, counted and never read item by item.
	groups["floor_pinned"] = []comparedRoot{
		{name: "RESOURCE_DESCRIPTION whose fourth details entry lacks its purpose", root: describedResource([]string{"key-echo", "key-alpha", "key-delta", "key-charlie", "key-bravo"}, "key-delta")},
		{name: "DV_MULTIMEDIA with 1 MiB of data", root: largeMultimedia()},
	}

	groups["guards"] = []comparedRoot{
		{name: "nil"},
		{name: "a typed-nil COMPOSITION", root: (*rm.Composition)(nil)},
		{name: "a value outside the closed RM set", root: struct{ Name string }{Name: "not RM"}},
	}

	for group, want := range minComparedRoots {
		if n := len(groups[group]); n < want {
			t.Fatalf("the harness found %d %s roots, want at least %d; its listing has lost roots", n, group, want)
		}
	}
	if n := len(groups["registry"]); n != registeredRMClasses {
		t.Fatalf("the harness built %d registry roots, want %d: a class was skipped", n, registeredRMClasses)
	}
	return groups
}

// REQ-171 § One walk — with no template the one walk is the floor: over every
// root the harness loads, it reports the issues ValidateRM reports, as
// multisets keyed by path, code, layer and rule, and in the same order, with
// the same coverage. None of these roots holds a shared pointer, so no
// difference is allowed; the test lists each one it finds.
func TestREQ171_EngineMatchesTheFloorWithNoTemplate(t *testing.T) {
	groups := comparisonRoots(t)
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		roots, issues := 0, 0
		for _, cr := range groups[group] {
			floor := validation.ValidateRM(cr.root)
			engine := validation.RunEngine(cr.root, nil, validation.Limits{})
			roots++
			issues += len(floor.Issues)
			floorKeys, engineKeys := findingKeys(floor.Issues), findingKeys(engine.Issues)
			if diff := multisetDiff(floorKeys, engineKeys); len(diff) > 0 {
				t.Errorf("%s %s: the engine's findings differ from ValidateRM's (%d against %d):\n%s", group, cr.name, len(engineKeys), len(floorKeys), strings.Join(diff, "\n"))
				continue
			}
			if !slices.Equal(floorKeys, engineKeys) {
				t.Errorf("%s %s: the engine reports ValidateRM's findings in another order:\n engine %v\n floor  %v", group, cr.name, engineKeys, floorKeys)
				continue
			}
			if !reflect.DeepEqual(engine, floor) {
				t.Errorf("%s %s: the engine's result differs from ValidateRM's beyond the finding keys:\n engine %+v\n floor  %+v", group, cr.name, engine, floor)
			}
		}
		t.Logf("%s: compared %d roots, %d issues", group, roots, issues)
	}
}

// REQ-171 § One walk — the harness can fail: a list that drops a finding,
// adds one, or repeats one is reported as a difference, and a list that holds
// the same findings in another order is not.
func TestREQ171_EngineComparisonFailsOnEachDifference(t *testing.T) {
	t.Parallel()
	a := findingKey{path: "/a", code: "required", layer: validation.LayerRM, rule: "mandatory"}
	b := findingKey{path: "/b", code: "rm_invariant", layer: validation.LayerRM, rule: "ELEMENT.Inv_null_flavour_indicated"}
	bOtherRule := findingKey{path: "/b", code: "rm_invariant", layer: validation.LayerRM, rule: "DV_TEXT.Mappings_valid"}
	tests := []struct {
		name          string
		floor, engine []findingKey
		differs       bool
	}{
		{name: "the same findings", floor: []findingKey{a, b}, engine: []findingKey{a, b}},
		{name: "the same findings in another order", floor: []findingKey{a, b}, engine: []findingKey{b, a}},
		{name: "a finding dropped", floor: []findingKey{a, b}, engine: []findingKey{a}, differs: true},
		{name: "a finding added", floor: []findingKey{a}, engine: []findingKey{a, b}, differs: true},
		{name: "a finding repeated", floor: []findingKey{a}, engine: []findingKey{a, a}, differs: true},
		{name: "another rule at one path", floor: []findingKey{b}, engine: []findingKey{bOtherRule}, differs: true},
	}
	for _, tc := range tests {
		if got := multisetDiff(tc.floor, tc.engine); (len(got) > 0) != tc.differs {
			t.Errorf("multisetDiff(%s) = %q, want a difference: %t", tc.name, got, tc.differs)
		}
	}
}

// largeMultimediaSize is the size of the data and of the integrity check of
// [largeMultimedia]: more bytes than the default visit limit counts visits.
const largeMultimediaSize = 1 << 20

// largeMultimedia returns a DV_MULTIMEDIA whose data and integrity_check are
// Arrays of Octet of [largeMultimediaSize] bytes each, one item per byte.
func largeMultimedia() *rm.DVMultimedia {
	return &rm.DVMultimedia{
		MediaType:      rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}, CodeString: "image/png"},
		Size:           largeMultimediaSize,
		Data:           make([]byte, largeMultimediaSize),
		IntegrityCheck: make([]byte, largeMultimediaSize),
	}
}

// REQ-171 § Structural access — the one walk counts a list of a primitive
// type and never reads it item by item, as the floor does: a DV_MULTIMEDIA
// with a megabyte of data validates with no max_nodes, and the walk
// allocates less than the payload's size, where boxing each byte as an item
// would allocate many times that.
func TestREQ171_EngineCountsAPrimitiveListWithoutReadingIt(t *testing.T) {
	root := largeMultimedia()
	var r validation.Result
	allocated := allocatedBytes(func() { r = validation.RunEngine(root, nil, validation.Limits{}) })
	if containsCode(r.Issues, "max_nodes") || len(r.Coverage.Stopped) != 0 {
		t.Errorf("engine(DV_MULTIMEDIA with %d bytes of data) issues = %q, Coverage.Stopped = %q; want no max_nodes and nothing stopped", largeMultimediaSize, issueKeys(r.Issues), r.Coverage.Stopped)
	}
	if allocated >= largeMultimediaSize {
		t.Errorf("engine(DV_MULTIMEDIA with %d bytes of data and of integrity_check) allocated %d bytes, want fewer than %d: the bytes are read one item at a time", largeMultimediaSize, allocated, largeMultimediaSize)
	}
}

// walkConditionRoot is a root the harness compares whose walk reaches a
// walk condition, the one code ValidateRM reports for it.
type walkConditionRoot struct {
	name string
	root any
	code string
}

// walkConditionRoots returns one root for each walk condition: a SECTION
// that holds itself, SECTIONs nested deeper than the default depth limit,
// and an INSTRUCTION whose protocol holds a CLUSTER that holds itself by
// value, which only the visit limit stops, before the walk reaches the
// INSTRUCTION's absent narrative.
func walkConditionRoots() []walkConditionRoot {
	cyclic := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "s"}}
	cyclic.Items = []rm.ContentItem{cyclic}
	loop := rm.Cluster{ArchetypeNodeID: "at0099", Name: rm.DVText{Value: "loop"}}
	loop.Items = make([]rm.Item, 2)
	loop.Items[0], loop.Items[1] = loop, loop
	return []walkConditionRoot{
		{name: "a SECTION holding itself", root: cyclic, code: "cyclic_value"},
		{name: "300 nested SECTIONs", root: nestedSections(300), code: "max_depth"},
		{
			name: "an INSTRUCTION whose protocol holds a CLUSTER holding itself by value, narrative absent",
			root: &rm.Instruction{
				ArchetypeNodeID: "openEHR-EHR-INSTRUCTION.request.v0",
				Name:            rm.DVText{Value: "request"},
				Protocol:        &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "protocol"}, Items: []rm.Item{loop}},
			},
			code: "max_nodes",
		},
	}
}

// withoutDetail returns r with the Detail of each issue cleared, so two
// results compare on everything else.
func withoutDetail(r validation.Result) validation.Result {
	r.Issues = slices.Clone(r.Issues)
	for i := range r.Issues {
		r.Issues[i].Detail = ""
	}
	return r
}

// REQ-171 § One walk — at each walk condition the one walk with no template
// reports what ValidateRM reports: the same findings, in the same order, with
// the same coverage, apart from the wording of the condition's Detail. The
// visit limit ends the whole walk, so the INSTRUCTION's absent narrative,
// which the walk would check after the protocol, is reported by neither.
func TestREQ171_EngineMatchesTheFloorAtEachWalkCondition(t *testing.T) {
	t.Parallel()
	for _, tc := range walkConditionRoots() {
		t.Run(tc.name, func(t *testing.T) {
			floor := validateWithin(t, "ValidateRM", visitLimitDeadline, func() validation.Result { return validation.ValidateRM(tc.root) })
			if n := countCode(floor.Issues, tc.code); n != 1 {
				t.Fatalf("precondition: ValidateRM(%s) reports %s %d times, want once: the root no longer reaches the condition", tc.name, tc.code, n)
			}
			engine := walkEngine(t, visitLimitDeadline, tc.root, validation.Limits{})
			floorKeys, engineKeys := findingKeys(floor.Issues), findingKeys(engine.Issues)
			if !slices.Equal(floorKeys, engineKeys) {
				t.Errorf("engine(%s) findings differ from ValidateRM's:\n engine %v\n floor  %v", tc.name, engineKeys, floorKeys)
			}
			if !reflect.DeepEqual(withoutDetail(engine), withoutDetail(floor)) {
				t.Errorf("engine(%s) result differs from ValidateRM's beyond the Detail:\n engine %+v\n floor  %+v", tc.name, engine, floor)
			}
			if tc.code == "max_nodes" {
				assertStoppedAtMaxNodes(t, "engine("+tc.name+")", engine)
			}
		})
	}
}
