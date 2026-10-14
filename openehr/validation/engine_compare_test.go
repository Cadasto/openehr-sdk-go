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
//
// With a template the one walk is compared with Validate, the two walks it
// replaces, over the corpus documents with their OPTs, the generator's
// Example instances, the constraint-fixture matrix, the probe inputs with
// their templates, the acceptance roots and the guards. Every difference
// must be one of the kinds REQ-171 § Acceptance allows: a finding from
// inside refused content, a second code for one defect at one path, or a
// shared value's second path.

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
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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

// templatedRoot is one root the harness validates against a compiled
// template both ways.
type templatedRoot struct {
	name string
	root any
	c    *templatecompile.Compiled
}

// minTemplatedRoots is the number of (root, template) pairs each group of
// the template comparison held when it was last counted; a group that holds
// fewer has lost pairs, as [minComparedRoots] says for the roots alone.
var minTemplatedRoots = map[string]int{
	"corpus":              50,
	"generated":           49,
	"constraint_fixtures": 25,
	"probes":              17,
	"acceptance":          11,
	"guards":              4,
}

// templatedRoots returns every (root, compiled template) pair the harness
// compares, by the group it comes from: each vendored corpus document whose
// OPT the corpus holds, the Example instance the generator builds for each
// such template, the instances of the constraint-fixture matrix with their
// templates, the probe inputs with the templates their cases compile, the
// roots of the acceptance tests of the one walk but the one that shares a
// value, and the roots the guards refuse or cannot descend. It fails the test when a group
// holds fewer pairs than [minTemplatedRoots] says.
func templatedRoots(t *testing.T) map[string][]templatedRoot {
	t.Helper()
	groups := map[string][]templatedRoot{}
	for _, doc := range corpusDocuments(t) {
		if doc.c == nil {
			continue
		}
		groups["corpus"] = append(groups["corpus"], templatedRoot{name: doc.rel.Rel, root: doc.root, c: doc.c})
		if root, err := generateExample(t, doc.c); err == nil {
			groups["generated"] = append(groups["generated"], templatedRoot{name: "Example of " + doc.rel.Template, root: root, c: doc.c})
		}
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
		groups["constraint_fixtures"] = append(groups["constraint_fixtures"], templatedRoot{name: id, root: &comp, c: mustCompile(t, id)})
	}

	inputs, err := probes.Inputs()
	if err != nil {
		t.Fatalf("probes.Inputs: %v", err)
	}
	for _, in := range inputs {
		groups["probes"] = append(groups["probes"], templatedRoot{name: in.Probe + " " + in.Name, root: in.Root, c: mustCompileInline(t, string(in.OPT))})
	}

	for _, tc := range missingNodeCases(t) {
		groups["acceptance"] = append(groups["acceptance"], templatedRoot{name: tc.name, root: tc.root, c: tc.c})
	}
	for _, tc := range refusedContentCases(t) {
		groups["acceptance"] = append(groups["acceptance"], templatedRoot{name: tc.name, root: tc.root, c: tc.c})
	}
	for _, tc := range emptyContainerCases(t) {
		groups["acceptance"] = append(groups["acceptance"], templatedRoot{name: "an empty container " + tc.name, root: tc.root, c: tc.c})
	}
	vitalSigns := mustCompile(t, "vital_signs")
	groups["acceptance"] = append(groups["acceptance"], templatedRoot{name: "an OBSERVATION whose guideline_id lacks its namespace", root: guidelineWithoutNamespace(t), c: vitalSigns})

	groups["guards"] = []templatedRoot{
		{name: "nil", c: vitalSigns},
		{name: "a typed-nil COMPOSITION", root: (*rm.Composition)(nil), c: vitalSigns},
		{name: "a template with no root", root: validVitalSignsComposition(), c: &templatecompile.Compiled{}},
		{name: "a value outside the closed RM set", root: struct{ Name string }{Name: "not RM"}, c: vitalSigns},
	}

	for group, want := range minTemplatedRoots {
		if n := len(groups[group]); n < want {
			t.Fatalf("the template comparison found %d %s pairs, want at least %d; its listing has lost pairs", n, group, want)
		}
	}
	return groups
}

// differenceKind is one of the kinds of difference REQ-171 § Acceptance
// allows between the one walk and the two walks it replaces.
type differenceKind uint8

const (
	// insideRefused is a two-walk finding from inside content the template
	// refused: the engine does not enter it.
	insideRefused differenceKind = iota
	// secondCode is a second code for one defect at one path: the two walks
	// carry two codes there, and the engine one of them.
	secondCode
	// sharedSecondPath is a shared value's second path: the engine validates
	// a value two places hold at each. A decoded document holds none.
	sharedSecondPath
	// unexplained is any other difference: a defect in one of the two.
	unexplained
)

// classifiedDifference is one finding the two results do not hold the same
// number of times, with the kind that explains it.
type classifiedDifference struct {
	key findingKey
	// inTwoWalks is set for a finding the two walks hold more often than the
	// engine, unset for one the engine holds more often.
	inTwoWalks bool
	kind       differenceKind
}

// String writes the difference as the harness lists it.
func (d classifiedDifference) String() string {
	side := "engine only"
	if d.inTwoWalks {
		side = "two walks only"
	}
	return fmt.Sprintf("  %s: %s", side, d.key)
}

// isAtOrBelow reports whether path is at, or strictly below, the path under:
// the same path, or one that continues it with an attribute or a predicate.
func isAtOrBelow(path, under string) bool {
	if path == under {
		return true
	}
	rest, ok := strings.CutPrefix(path, under)
	return ok && (strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "["))
}

// classifyDifferences returns, sorted, each finding the two walks' issues and
// the engine's do not hold the same number of times, one entry per surplus
// occurrence, classified by the kinds REQ-171 § Acceptance allows. refused
// holds the paths of the content the engine did not admit and did not enter.
//
// A finding only the two walks hold is inside refused content when it is an
// RM finding or a walk condition at or below a refused path: the floor
// walked into content the engine reported once and left. It is a second code
// for one defect at one path when, at its path, the two walks hold another
// code that the engine holds too, and the engine does not hold its code
// there. A finding only the engine holds has no allowed kind on these roots,
// which share no value.
func classifyDifferences(twoWalks, engine []validation.Issue, refused []string) []classifiedDifference {
	counts := map[findingKey][2]int{}
	codesAt := [2]map[string]map[string]bool{{}, {}}
	for side, issues := range [2][]validation.Issue{twoWalks, engine} {
		for _, k := range findingKeys(issues) {
			c := counts[k]
			c[side]++
			counts[k] = c
			if codesAt[side][k.path] == nil {
				codesAt[side][k.path] = map[string]bool{}
			}
			codesAt[side][k.path][k.code] = true
		}
	}
	var out []classifiedDifference
	for _, k := range slices.SortedFunc(maps.Keys(counts), func(a, b findingKey) int { return strings.Compare(a.String(), b.String()) }) {
		c := counts[k]
		for range max(c[0]-c[1], 0) {
			out = append(out, classifiedDifference{key: k, inTwoWalks: true, kind: classifyTwoWalkOnly(k, codesAt, refused)})
		}
		for range max(c[1]-c[0], 0) {
			out = append(out, classifiedDifference{key: k, kind: unexplained})
		}
	}
	return out
}

// classifyTwoWalkOnly returns the kind of k, a finding the two walks hold
// more often than the engine, given the codes each side holds at each path
// (two walks first) and the paths the engine refused.
func classifyTwoWalkOnly(k findingKey, codesAt [2]map[string]map[string]bool, refused []string) differenceKind {
	if k.layer != validation.LayerTemplate && slices.ContainsFunc(refused, func(r string) bool { return isAtOrBelow(k.path, r) }) {
		return insideRefused
	}
	twoWalks, engine := codesAt[0][k.path], codesAt[1][k.path]
	if engine[k.code] {
		return unexplained
	}
	for code := range engine {
		if code != k.code && twoWalks[code] {
			return secondCode
		}
	}
	return unexplained
}

// REQ-171 § One walk, § Acceptance — with a template the one walk reports
// what the two walks report (the template walk, then the floor, less the
// floor's repeats), finding by finding on path, code, layer and rule, apart
// from the differences REQ-171 § Acceptance allows: a finding from inside
// content the template refused, a second code for one defect at one path,
// and a shared value's second path, which no root here holds. Any other
// difference fails the test, listed. The coverage agrees: the same Stopped,
// the same External paths, and Unevaluated invariants the two walks list too,
// all of them unless the engine refused content. The test logs, per group,
// the pairs and issues compared, the count of each kind and every second
// code, and how many results list their findings, or their External paths,
// in another order, which is reported, not failed: the one walk visits a
// node's attributes in the class's order, the template walk in the
// template's.
func TestREQ171_EngineWithATemplateMatchesTheTwoWalks(t *testing.T) {
	groups := templatedRoots(t)
	var secondCodes []string
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		pairs, issues, reordered, externalReordered := 0, 0, 0, 0
		var kinds [unexplained + 1]int
		for _, tr := range groups[group] {
			twoWalks := validation.Validate(tr.root, tr.c)
			engine, refused := validation.RunEngineRefusing(tr.root, tr.c, validation.Limits{})
			pairs++
			issues += len(twoWalks.Issues)
			diffs := classifyDifferences(twoWalks.Issues, engine.Issues, refused)
			var bad []string
			for _, d := range diffs {
				kinds[d.kind]++
				switch d.kind {
				case unexplained:
					bad = append(bad, d.String())
				case secondCode:
					secondCodes = append(secondCodes, fmt.Sprintf("%s %s: %s dropped beside the engine's %q", group, tr.name, d.key, keysAt(engine.Issues, d.key.path)))
				case insideRefused, sharedSecondPath:
				}
			}
			if len(bad) > 0 {
				t.Errorf("%s %s: the engine's findings differ from Validate's in ways REQ-171 does not allow (refused %q):\n%s", group, tr.name, refused, strings.Join(bad, "\n"))
			}
			if len(diffs) == 0 && !slices.Equal(findingKeys(twoWalks.Issues), findingKeys(engine.Issues)) {
				reordered++
			}
			if !slices.Equal(engine.Coverage.Stopped, twoWalks.Coverage.Stopped) {
				t.Errorf("%s %s: engine Coverage.Stopped = %q, Validate's %q", group, tr.name, engine.Coverage.Stopped, twoWalks.Coverage.Stopped)
			}
			if got, want := slices.Sorted(slices.Values(engine.Coverage.External)), slices.Sorted(slices.Values(twoWalks.Coverage.External)); !slices.Equal(got, want) {
				t.Errorf("%s %s: engine Coverage.External = %q, Validate's %q", group, tr.name, engine.Coverage.External, twoWalks.Coverage.External)
			} else if !slices.Equal(engine.Coverage.External, twoWalks.Coverage.External) {
				externalReordered++
			}
			unevaluated, want := engine.Coverage.Unevaluated, twoWalks.Coverage.Unevaluated
			if len(refused) == 0 && !slices.Equal(unevaluated, want) || slices.ContainsFunc(unevaluated, func(u string) bool { return !slices.Contains(want, u) }) {
				t.Errorf("%s %s: engine Coverage.Unevaluated = %q, Validate's %q (refused %q)", group, tr.name, unevaluated, want, refused)
			}
		}
		t.Logf("%s: compared %d pairs, %d two-walk issues; differences: %d inside refused content, %d second codes, %d shared second paths, %d unexplained; %d results list their findings, %d their External paths, in another order",
			group, pairs, issues, kinds[insideRefused], kinds[secondCode], kinds[sharedSecondPath], kinds[unexplained], reordered, externalReordered)
	}
	for _, s := range secondCodes {
		t.Logf("second code: %s", s)
	}
}

// REQ-171 § Acceptance — the classification can fail: a finding only the
// engine holds, a template finding only the two walks hold below a refusal,
// an RM finding the two walks hold at a path where the engine holds nothing,
// and a code the engine also holds at that path are each unexplained; an RM
// finding at or below a refused path is inside refused content, and one the
// two walks hold beside a code the engine keeps at that path is a second
// code.
func TestREQ171_DifferenceClassificationFailsOnEachUnexplainedDifference(t *testing.T) {
	t.Parallel()
	required := validation.Issue{Path: "/a/items", Code: "required", Layer: validation.LayerTemplate, Rule: "existence"}
	rmCardinality := validation.Issue{Path: "/a/items", Code: "cardinality", Layer: validation.LayerRM, Rule: "cardinality"}
	slotFill := validation.Issue{Path: "/a/items[2]", Code: "slot_fill", Layer: validation.LayerTemplate, Rule: "slot"}
	inside := validation.Issue{Path: "/a/items[2]/name", Code: "required", Layer: validation.LayerRM, Rule: "mandatory"}
	atRefused := validation.Issue{Path: "/a/items[2]", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "ELEMENT.Inv_null_flavour_indicated"}
	templateInside := validation.Issue{Path: "/a/items[2]/name", Code: "node_id_mismatch", Layer: validation.LayerTemplate, Rule: "identity"}
	elsewhere := validation.Issue{Path: "/b", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "DV_TEXT.Mappings_valid"}
	tests := []struct {
		name             string
		twoWalks, engine []validation.Issue
		refused          []string
		want             []differenceKind
	}{
		{name: "the same findings", twoWalks: []validation.Issue{required, slotFill}, engine: []validation.Issue{slotFill, required}, refused: []string{"/a/items[2]"}},
		{name: "an RM finding below a refused item", twoWalks: []validation.Issue{slotFill, inside}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{insideRefused}},
		{name: "an RM finding at a refused item", twoWalks: []validation.Issue{slotFill, atRefused}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{insideRefused}},
		{name: "a template finding below a refused item", twoWalks: []validation.Issue{slotFill, templateInside}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{unexplained}},
		{name: "an RM finding below a path the engine did not refuse", twoWalks: []validation.Issue{slotFill, inside}, engine: []validation.Issue{slotFill}, want: []differenceKind{unexplained}},
		{name: "a second code at one path", twoWalks: []validation.Issue{required, rmCardinality}, engine: []validation.Issue{required}, want: []differenceKind{secondCode}},
		{name: "the one code at a path dropped", twoWalks: []validation.Issue{rmCardinality}, want: []differenceKind{unexplained}},
		{name: "an RM finding at a path the engine holds nothing", twoWalks: []validation.Issue{required, elsewhere}, engine: []validation.Issue{required}, want: []differenceKind{unexplained}},
		{name: "a finding only the engine holds", twoWalks: []validation.Issue{required}, engine: []validation.Issue{required, rmCardinality}, want: []differenceKind{unexplained}},
		{name: "a finding the engine repeats", twoWalks: []validation.Issue{required}, engine: []validation.Issue{required, required}, want: []differenceKind{unexplained}},
	}
	for _, tc := range tests {
		var got []differenceKind
		for _, d := range classifyDifferences(tc.twoWalks, tc.engine, tc.refused) {
			got = append(got, d.kind)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("classifyDifferences(%s) kinds = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// deviceHolding returns the valid vital_signs composition whose device
// CLUSTER, which fills a slot of vital_signs.opt, holds items besides its own
// ELEMENT: what more returns, given the CLUSTER.
func deviceHolding(t *testing.T, more func(device *rm.Cluster) []rm.Item) *rm.Composition {
	t.Helper()
	comp := validVitalSignsComposition()
	tree, ok := bpObservation(t, comp).Protocol.(*rm.ItemTree)
	if !ok || len(tree.Items) != 1 {
		t.Fatalf("fixture protocol is %T, want an ITEM_TREE holding the device CLUSTER", bpObservation(t, comp).Protocol)
	}
	device, ok := tree.Items[0].(*rm.Cluster)
	if !ok {
		t.Fatalf("fixture protocol item is %T, want *rm.Cluster", tree.Items[0])
	}
	device.Items = append(device.Items, more(device)...)
	return comp
}

// REQ-171 § One walk — below a slot the template does not constrain, the one
// walk with a template meets each walk condition where the two walks' floor
// meets it, and reports the same findings, in the same order, with the same
// coverage, apart from the classes the template walk of the two walks, which
// knows no limit, visits after a visit stop: the device
// CLUSTER of vital_signs.opt holding itself twice, holding SECTIONs nested
// deeper than the default depth limit, and holding a CLUSTER that holds
// itself by value, which only the visit limit stops.
func TestREQ171_EngineWithATemplateMatchesTheTwoWalksAtEachWalkCondition(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	loop := rm.Cluster{ArchetypeNodeID: "at0099", Name: rm.DVText{Value: "loop"}}
	loop.Items = make([]rm.Item, 2)
	loop.Items[0], loop.Items[1] = loop, loop
	deep := &rm.Cluster{ArchetypeNodeID: "at0098", Name: rm.DVText{Value: "bottom"}, Items: []rm.Item{&rm.Element{ArchetypeNodeID: "at0097", Name: rm.DVText{Value: "e"}, Value: &rm.DVBoolean{Value: true}}}}
	for range 300 {
		deep = &rm.Cluster{ArchetypeNodeID: "at0098", Name: rm.DVText{Value: "level"}, Items: []rm.Item{deep}}
	}
	tests := []struct {
		name string
		root *rm.Composition
		code string
	}{
		{name: "the device CLUSTER holding itself twice", root: deviceHolding(t, func(d *rm.Cluster) []rm.Item { return []rm.Item{d, d} }), code: "cyclic_value"},
		{name: "the device CLUSTER holding 300 nested CLUSTERs", root: deviceHolding(t, func(*rm.Cluster) []rm.Item { return []rm.Item{deep} }), code: "max_depth"},
		{name: "the device CLUSTER holding a CLUSTER that holds itself by value", root: deviceHolding(t, func(*rm.Cluster) []rm.Item { return []rm.Item{loop} }), code: "max_nodes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			twoWalks := validateWithin(t, "Validate", visitLimitDeadline, func() validation.Result { return validation.Validate(tc.root, c) })
			if n := countCode(twoWalks.Issues, tc.code); n == 0 {
				t.Fatalf("precondition: Validate(%s) reports no %s: the root no longer reaches the condition", tc.name, tc.code)
			}
			engine := validateWithin(t, "the engine", visitLimitDeadline, func() validation.Result { return validation.RunEngine(tc.root, c, validation.Limits{}) })
			twoWalkKeys, engineKeys := findingKeys(twoWalks.Issues), findingKeys(engine.Issues)
			if !slices.Equal(twoWalkKeys, engineKeys) {
				t.Errorf("engine(%s) findings differ from Validate's:\n engine %v\n Validate %v", tc.name, engineKeys, twoWalkKeys)
			}
			if engine.OK != twoWalks.OK || !slices.Equal(engine.Coverage.Stopped, twoWalks.Coverage.Stopped) || !slices.Equal(engine.Coverage.External, twoWalks.Coverage.External) {
				t.Errorf("engine(%s) OK %t, Stopped %q, External %q; Validate's OK %t, Stopped %q, External %q", tc.name, engine.OK, engine.Coverage.Stopped, engine.Coverage.External, twoWalks.OK, twoWalks.Coverage.Stopped, twoWalks.Coverage.External)
			}
			// The template walk of the two walks knows no limit, so after a
			// visit stop it still visits the classes the one walk does not
			// reach: the one walk's Unevaluated is then a part of theirs.
			unevaluated, want := engine.Coverage.Unevaluated, twoWalks.Coverage.Unevaluated
			if tc.code != "max_nodes" && !slices.Equal(unevaluated, want) || slices.ContainsFunc(unevaluated, func(u string) bool { return !slices.Contains(want, u) }) {
				t.Errorf("engine(%s) Coverage.Unevaluated = %q, Validate's %q", tc.name, unevaluated, want)
			}
			if tc.code == "max_nodes" {
				assertStoppedAtMaxNodes(t, "engine("+tc.name+")", engine)
			}
		})
	}
}
