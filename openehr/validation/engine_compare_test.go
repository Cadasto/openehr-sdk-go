package validation_test

// engine_compare_test.go: REQ-171 § One walk, § Acceptance — the comparison
// harness. The one walk runs beside the floor until the comparison is done:
// over the vendored corpus, a root of every class the registry knows, the
// constraint-fixture matrix, every probe input and the roots the floor's own
// tests pin, a walk with no template reports the issues ValidateRM reports,
// finding by finding on path, code, layer and rule. Decoded documents, zero
// values and the generator's Example instances hold no shared pointer. One
// probe input does, SystolicTwice, whose two systolic ELEMENTs hold one
// DV_QUANTITY, but that value breaks no rule, so the one walk's only
// difference from the floor, a defect in a shared value reported at each
// place, cannot arise on these roots, and the two must agree exactly. A root
// that reaches a walk condition (a cycle, the depth limit, the visit limit)
// is compared the same way, apart from the wording of the condition's
// Detail.
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
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
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
// the same coverage. The one shared pointer these roots hold, in the probe
// input SystolicTwice, breaks no rule, so no difference is allowed; the test
// lists each one it finds.
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
	"acceptance":          18,
	"guards":              4,
}

// templatedRoots returns every (root, compiled template) pair the harness
// compares, by the group it comes from: each vendored corpus document whose
// OPT the corpus holds, the Example instance the generator builds for each
// such template, the instances of the constraint-fixture matrix with their
// templates, the probe inputs with the templates their cases compile, the
// roots of the acceptance tests of the one walk, and the roots the guards
// refuse or cannot descend. It leaves out two acceptance roots whose
// difference is none of the kinds REQ-171 § Acceptance allows, each pinned by
// its own test: an empty Hash the two walks read as `required` from the BMM
// bit the template carries, which the RM rules alone do not report
// ([TestREQ171_EngineReadsAnEmptyHashAsTheRMRulesDo]), and a reference's
// missing part, which the two walks report as `required` beside the
// reference's own rule ([TestREQ171_EngineReportsAMissingReferencePartOnce]).
// It fails the test when a group holds fewer pairs than [minTemplatedRoots]
// says.
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
	for _, tc := range rmMandatoryCases(t) {
		groups["acceptance"] = append(groups["acceptance"], templatedRoot{name: tc.name, root: tc.root, c: tc.c})
	}
	vitalSigns := mustCompile(t, "vital_signs")
	groups["acceptance"] = append(groups["acceptance"],
		templatedRoot{name: "an OBSERVATION whose guideline_id lacks its namespace", root: guidelineWithoutNamespace(t), c: vitalSigns},
		templatedRoot{name: "two systolic ELEMENTs holding one DV_QUANTITY whose precision is below -1", root: sharedPrecision(t), c: vitalSigns},
	)

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
	// carry two codes there, and the engine one of them. The harness knows
	// one such pair, the presence and the count of a container (see
	// [classifyTwoWalkOnly]).
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
// holds the paths of the content the engine did not admit and did not enter;
// sameValue reports whether two issue paths of the root reach one Go value,
// and may be nil, which reports none.
//
// A finding only the two walks hold is inside refused content when it is an
// RM finding or a walk condition at or below a refused path: the floor
// walked into content the engine reported once and left. It is a second code
// for one defect at one path as [classifyTwoWalkOnly] says. A finding only
// the engine holds is a shared value's second path when the two walks hold a
// finding of the same code, layer and rule at a sibling item path, one that
// differs from its path in one item's predicate alone, and the two paths
// reach one Go value: the floor walked that value once, at the first path.
func classifyDifferences(twoWalks, engine []validation.Issue, refused []string, sameValue func(p, q string) bool) []classifiedDifference {
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
			kind := unexplained
			if sameValue != nil && slices.ContainsFunc(findingKeys(twoWalks), func(o findingKey) bool {
				return o.code == k.code && o.layer == k.layer && o.rule == k.rule && siblingItemPaths(k.path, o.path) && sameValue(k.path, o.path)
			}) {
				kind = sharedSecondPath
			}
			out = append(out, classifiedDifference{key: k, kind: kind})
		}
	}
	return out
}

// classifyTwoWalkOnly returns the kind of k, a finding the two walks hold
// more often than the engine, given the codes each side holds at each path
// (two walks first) and the paths the engine refused.
//
// The one second code it allows is the presence and the count of one
// container: k is the RM layer's `required` or `cardinality` by the rule
// cardinality, an empty container the RM requires, and the engine holds,
// at k's path, a `required` or a `cardinality` the two walks hold too, and
// not k's code. Any other finding the engine drops at a path where it holds
// another code, an RM rule's finding beside a template one among them, is
// unexplained: it is a rule the engine lost.
func classifyTwoWalkOnly(k findingKey, codesAt [2]map[string]map[string]bool, refused []string) differenceKind {
	if k.layer != validation.LayerTemplate && slices.ContainsFunc(refused, func(r string) bool { return isAtOrBelow(k.path, r) }) {
		return insideRefused
	}
	presenceOrCount := func(code string) bool { return code == "required" || code == "cardinality" }
	if k.layer != validation.LayerRM || k.rule != "cardinality" || !presenceOrCount(k.code) {
		return unexplained
	}
	twoWalks, engine := codesAt[0][k.path], codesAt[1][k.path]
	if engine[k.code] {
		return unexplained
	}
	for code := range engine {
		if presenceOrCount(code) && twoWalks[code] {
			return secondCode
		}
	}
	return unexplained
}

// pathSegments splits an issue path into its segments, each an attribute
// name and the predicate that follows it, if any: "/a[x]/b" is "a[x]" and
// "b". A slash inside a predicate, in a quoted name, does not split it. The
// root path "/" has no segment.
func pathSegments(path string) []string {
	var out []string
	depth, quoted, start := 0, false, 1
	for i := 1; i < len(path); i++ {
		switch c := path[i]; {
		case quoted && c == '\\':
			i++
		case c == '\'' && depth > 0:
			quoted = !quoted
		case quoted:
		case c == '[':
			depth++
		case c == ']':
			depth--
		case c == '/' && depth == 0:
			out = append(out, path[start:i])
			start = i + 1
		}
	}
	if start < len(path) {
		out = append(out, path[start:])
	}
	return out
}

// siblingItemPaths reports whether the issue paths p and q differ in one
// segment alone, and there in the predicate of one attribute's item: two
// items of one list, or what lies at one place below each.
func siblingItemPaths(p, q string) bool {
	ps, qs := pathSegments(p), pathSegments(q)
	if len(ps) != len(qs) {
		return false
	}
	differ := 0
	for i := range ps {
		if ps[i] == qs[i] {
			continue
		}
		differ++
		pa, _, pPredicated := strings.Cut(ps[i], "[")
		qa, _, qPredicated := strings.Cut(qs[i], "[")
		if pa != qa || !pPredicated || !qPredicated {
			return false
		}
	}
	return differ == 1
}

// valuesAlong returns the Go value each segment of the issue path reaches
// from root, read through the generated access as the walks read it: a
// single value by its attribute, an item of a list or an entry of a Hash by
// its predicate, which is its 1-based position, or the archetype_node_id the
// item carries, with the name it carries when the predicate pins one. ok is
// false when a segment reaches nothing.
func valuesAlong(root any, path string) (values []any, ok bool) {
	cur := root
	for _, seg := range pathSegments(path) {
		attr, pred, hasPred := strings.Cut(seg, "[")
		if !hasPred {
			v, present := rmaccess.Single(cur, attr)
			if !present {
				return nil, false
			}
			cur = v
			values = append(values, cur)
			continue
		}
		items, present := rmaccess.Multiple(cur, attr)
		if !present {
			if _, items, present = rmaccess.Hash(cur, attr); !present {
				return nil, false
			}
		}
		item, found := itemByPredicate(items, strings.TrimSuffix(pred, "]"))
		if !found {
			return nil, false
		}
		cur = item
		values = append(values, cur)
	}
	return values, true
}

// itemByPredicate returns the item of items a path predicate names: its
// 1-based position, or the first item that carries the archetype_node_id the
// predicate names and, when the predicate pins one, the name.
func itemByPredicate(items []any, pred string) (any, bool) {
	if n, err := strconv.Atoi(pred); err == nil {
		if n < 1 || n > len(items) {
			return nil, false
		}
		return items[n-1], true
	}
	id, name, named := strings.Cut(pred, ",'")
	if named {
		name = strings.TrimSuffix(name, "'")
		name = strings.NewReplacer(`\'`, `'`, `\\`, `\`).Replace(name)
	}
	for _, item := range items {
		l, isLocatable := item.(rm.Locatable)
		if !isLocatable || rmaccess.IsTypedNilPointer(item) || l.GetArchetypeNodeID() != id {
			continue
		}
		if !named || itemName(item) == name {
			return item, true
		}
	}
	return nil, false
}

// itemName returns the value of a LOCATABLE item's name, or "" when it has
// none.
func itemName(item any) string {
	v, ok := rmaccess.Single(item, "name")
	if !ok {
		return ""
	}
	like, ok := v.(rm.DVTextLike)
	if !ok {
		return ""
	}
	text, ok := rm.AsDVText(like)
	if !ok {
		return ""
	}
	return text.Value
}

// reachSameValue reports whether the issue paths p and q of root, sibling
// item paths ([siblingItemPaths]), reach one Go value: from the segment
// where they differ down, at some segment both reach the same pointer, so
// everything below it is one value.
func reachSameValue(root any, p, q string) bool {
	pv, okP := valuesAlong(root, p)
	qv, okQ := valuesAlong(root, q)
	if !okP || !okQ || len(pv) != len(qv) {
		return false
	}
	ps, qs := pathSegments(p), pathSegments(q)
	from := 0
	for from < len(ps) && ps[from] == qs[from] {
		from++
	}
	for i := from; i < len(pv); i++ {
		v := reflect.ValueOf(pv[i])
		if v.Kind() == reflect.Pointer && !v.IsNil() && pv[i] == qv[i] {
			return true
		}
	}
	return false
}

// REQ-171 § One walk, § Acceptance — with a template the one walk reports
// what the two walks report (the template walk, then the floor, less the
// floor's repeats), finding by finding on path, code, layer and rule, apart
// from the differences REQ-171 § Acceptance allows: a finding from inside
// content the template refused, a second code for one defect at one path,
// and a shared value's second path. Any other difference fails the test,
// listed, and so does a second code outside the acceptance roots, which are
// the only roots built to hold one. The coverage agrees: the same Stopped,
// the same External paths, and Unevaluated invariants the two walks list
// too, all of them unless the engine refused content. The test logs, per
// group, the pairs and issues compared, the count of each kind and every
// second code, and how many results list their findings, their template
// findings, or their External paths in another order, which is reported,
// not failed: the one walk reports a node's RM rules beside its template
// findings, where the two walks report every RM finding last, and it
// reports a list's counts before the findings of its items, by the step
// order of REQ-171 § One walk.
func TestREQ171_EngineWithATemplateMatchesTheTwoWalks(t *testing.T) {
	groups := templatedRoots(t)
	var secondCodes []string
	for _, group := range slices.Sorted(maps.Keys(groups)) {
		pairs, issues, reordered, templateReordered, externalReordered := 0, 0, 0, 0, 0
		var kinds [unexplained + 1]int
		for _, tr := range groups[group] {
			twoWalks := validation.Validate(tr.root, tr.c)
			engine, refused := validation.RunEngineRefusing(tr.root, tr.c, validation.Limits{})
			pairs++
			issues += len(twoWalks.Issues)
			diffs := classifyDifferences(twoWalks.Issues, engine.Issues, refused, func(p, q string) bool { return reachSameValue(tr.root, p, q) })
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
			if len(diffs) == 0 && !slices.Equal(templateFindingKeys(twoWalks.Issues), templateFindingKeys(engine.Issues)) {
				templateReordered++
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
		t.Logf("%s: compared %d pairs, %d two-walk issues; differences: %d inside refused content, %d second codes, %d shared second paths, %d unexplained; %d results list their findings, %d their template findings, %d their External paths, in another order",
			group, pairs, issues, kinds[insideRefused], kinds[secondCode], kinds[sharedSecondPath], kinds[unexplained], reordered, templateReordered, externalReordered)
		if group != "acceptance" && kinds[secondCode] > 0 {
			t.Errorf("%s: %d second codes, where only the acceptance roots are built to hold one; a second code here hides a rule the engine lost", group, kinds[secondCode])
		}
	}
	for _, s := range secondCodes {
		t.Logf("second code: %s", s)
	}
}

// templateFindingKeys returns the key of each issue on the template layer,
// in result order.
func templateFindingKeys(issues []validation.Issue) []findingKey {
	var out []findingKey
	for _, k := range findingKeys(issues) {
		if k.layer == validation.LayerTemplate {
			out = append(out, k)
		}
	}
	return out
}

// REQ-171 § Acceptance — the classification can fail: a finding only the
// engine holds, a template finding only the two walks hold below a refusal,
// an RM finding the two walks hold at a path where the engine holds nothing,
// and a code the engine also holds at that path are each unexplained; an RM
// finding at or below a refused path is inside refused content, and one the
// two walks hold beside a code the engine keeps at that path is a second
// code only for the presence and the count of one container. An RM rule's
// finding the engine drops beside a template finding at its path is
// unexplained: COMPOSITION.Category_validity lost beside the category's code
// list is the defect the classification exists to catch, and so is a
// reference part's `required` dropped beside the reference's own rule. A
// finding only the engine holds is a shared value's second path only where
// the two walks hold its code, layer and rule at a sibling item path that
// reaches the same Go value.
func TestREQ171_DifferenceClassificationFailsOnEachUnexplainedDifference(t *testing.T) {
	t.Parallel()
	required := validation.Issue{Path: "/a/items", Code: "required", Layer: validation.LayerTemplate, Rule: "existence"}
	rmCardinality := validation.Issue{Path: "/a/items", Code: "cardinality", Layer: validation.LayerRM, Rule: "cardinality"}
	rmRequiredEmpty := validation.Issue{Path: "/a/items", Code: "required", Layer: validation.LayerRM, Rule: "cardinality"}
	occurrences := validation.Issue{Path: "/a/items", Code: "cardinality", Layer: validation.LayerTemplate, Rule: "occurrences"}
	slotFill := validation.Issue{Path: "/a/items[2]", Code: "slot_fill", Layer: validation.LayerTemplate, Rule: "slot"}
	inside := validation.Issue{Path: "/a/items[2]/name", Code: "required", Layer: validation.LayerRM, Rule: "mandatory"}
	atRefused := validation.Issue{Path: "/a/items[2]", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "ELEMENT.Inv_null_flavour_indicated"}
	templateInside := validation.Issue{Path: "/a/items[2]/name", Code: "node_id_mismatch", Layer: validation.LayerTemplate, Rule: "identity"}
	elsewhere := validation.Issue{Path: "/b", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "DV_TEXT.Mappings_valid"}
	categoryCoded := validation.Issue{Path: "/category/defining_code", Code: "code_not_in_value_set", Layer: validation.LayerRM, Rule: "COMPOSITION.Category_validity"}
	categoryList := validation.Issue{Path: "/category/defining_code", Code: "primitive_not_in_list", Layer: validation.LayerTemplate, Rule: "not_in_list"}
	partRequired := validation.Issue{Path: "/composer/external_ref/namespace", Code: "required", Layer: validation.LayerRM, Rule: "mandatory"}
	partRule := validation.Issue{Path: "/composer/external_ref/namespace", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "mandatory"}
	precisionFirst := validation.Issue{Path: "/a/items[1]/value", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "precision_range"}
	precisionSecond := validation.Issue{Path: "/a/items[2]/value", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "precision_range"}
	precisionElsewhere := validation.Issue{Path: "/b/items[2]/value", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "precision_range"}
	otherRuleFirst := validation.Issue{Path: "/a/items[1]/value", Code: "rm_invariant", Layer: validation.LayerRM, Rule: "DV_INTERVAL.Limits_consistent"}
	shared := func(string, string) bool { return true }
	unshared := func(string, string) bool { return false }
	tests := []struct {
		name             string
		twoWalks, engine []validation.Issue
		refused          []string
		sameValue        func(p, q string) bool
		want             []differenceKind
	}{
		{name: "the same findings", twoWalks: []validation.Issue{required, slotFill}, engine: []validation.Issue{slotFill, required}, refused: []string{"/a/items[2]"}},
		{name: "an RM finding below a refused item", twoWalks: []validation.Issue{slotFill, inside}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{insideRefused}},
		{name: "an RM finding at a refused item", twoWalks: []validation.Issue{slotFill, atRefused}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{insideRefused}},
		{name: "a template finding below a refused item", twoWalks: []validation.Issue{slotFill, templateInside}, engine: []validation.Issue{slotFill}, refused: []string{"/a/items[2]"}, want: []differenceKind{unexplained}},
		{name: "an RM finding below a path the engine did not refuse", twoWalks: []validation.Issue{slotFill, inside}, engine: []validation.Issue{slotFill}, want: []differenceKind{unexplained}},
		{name: "a second code at one path", twoWalks: []validation.Issue{required, rmCardinality}, engine: []validation.Issue{required}, want: []differenceKind{secondCode}},
		{name: "the RM's required for an empty list beside the template's occurrences", twoWalks: []validation.Issue{rmRequiredEmpty, occurrences}, engine: []validation.Issue{occurrences}, want: []differenceKind{secondCode}},
		{name: "the one code at a path dropped", twoWalks: []validation.Issue{rmCardinality}, want: []differenceKind{unexplained}},
		{name: "an RM finding at a path the engine holds nothing", twoWalks: []validation.Issue{required, elsewhere}, engine: []validation.Issue{required}, want: []differenceKind{unexplained}},
		{name: "a coded RM rule dropped beside a template finding at its path", twoWalks: []validation.Issue{categoryCoded, categoryList}, engine: []validation.Issue{categoryList}, want: []differenceKind{unexplained}},
		{name: "a reference part's required dropped beside the reference's rule", twoWalks: []validation.Issue{partRequired, partRule}, engine: []validation.Issue{partRule}, want: []differenceKind{unexplained}},
		{name: "a finding only the engine holds", twoWalks: []validation.Issue{required}, engine: []validation.Issue{required, rmCardinality}, want: []differenceKind{unexplained}},
		{name: "a finding the engine repeats", twoWalks: []validation.Issue{required}, engine: []validation.Issue{required, required}, want: []differenceKind{unexplained}},
		{name: "a finding only the engine holds, at a sibling item of one Go value", twoWalks: []validation.Issue{precisionFirst}, engine: []validation.Issue{precisionFirst, precisionSecond}, sameValue: shared, want: []differenceKind{sharedSecondPath}},
		{name: "a finding only the engine holds, at a sibling item of another Go value", twoWalks: []validation.Issue{precisionFirst}, engine: []validation.Issue{precisionFirst, precisionSecond}, sameValue: unshared, want: []differenceKind{unexplained}},
		{name: "a finding only the engine holds, at an item of one Go value that is no sibling", twoWalks: []validation.Issue{precisionFirst}, engine: []validation.Issue{precisionFirst, precisionElsewhere}, sameValue: shared, want: []differenceKind{unexplained}},
		{name: "a finding only the engine holds, beside another rule at a sibling item of one Go value", twoWalks: []validation.Issue{otherRuleFirst}, engine: []validation.Issue{otherRuleFirst, precisionSecond}, sameValue: shared, want: []differenceKind{unexplained}},
	}
	for _, tc := range tests {
		var got []differenceKind
		for _, d := range classifyDifferences(tc.twoWalks, tc.engine, tc.refused, tc.sameValue) {
			got = append(got, d.kind)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("classifyDifferences(%s) kinds = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// REQ-171 § Acceptance — the harness tells a shared value's second path by
// the Go value two sibling item paths reach: in SystolicTwice the two
// systolic ELEMENTs are two values that hold one DV_QUANTITY, so their
// paths reach one value at their value and not at the ELEMENTs themselves;
// two paths that differ in more than one item's predicate are no siblings;
// and a path that reaches nothing reaches no value.
func TestREQ171_SharedValueOfSiblingItemPaths(t *testing.T) {
	t.Parallel()
	root := probes.SystolicTwice()
	const list = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items"
	tests := []struct {
		name    string
		p, q    string
		sibling bool
		same    bool
	}{
		{name: "the shared DV_QUANTITY", p: list + "[1]/value", q: list + "[2]/value", sibling: true, same: true},
		{name: "the two ELEMENTs, copies of one", p: list + "[1]", q: list + "[2]", sibling: true},
		{name: "an item that is not there", p: list + "[1]/value", q: list + "[3]/value", sibling: true},
		{name: "two predicates apart", p: list + "[1]/value", q: "/content[openEHR-EHR-OBSERVATION.other.v1]/data/events[at0006]/data/items[2]/value"},
		{name: "a name with a slash in its predicate", p: "/content[at0001,'a/b']/value", q: "/content[at0001,'a/c']/value", sibling: true},
	}
	for _, tc := range tests {
		if got := siblingItemPaths(tc.p, tc.q); got != tc.sibling {
			t.Errorf("siblingItemPaths(%s: %s, %s) = %t, want %t", tc.name, tc.p, tc.q, got, tc.sibling)
		}
		if got := reachSameValue(root, tc.p, tc.q); got != tc.same {
			t.Errorf("reachSameValue(SystolicTwice, %s: %s, %s) = %t, want %t", tc.name, tc.p, tc.q, got, tc.same)
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

// atVisitStop returns keys and stopped with the path of each max_nodes
// finding, and of the stop it lists, that lies at or below inside replaced by
// a mark that names inside, so two results that stop at the visit limit at
// different nodes of inside compare equal.
func atVisitStop(keys []findingKey, stopped []string, inside string) ([]findingKey, []string) {
	mark := "<a node of " + inside + ">"
	keys, stopped = slices.Clone(keys), slices.Clone(stopped)
	for i, k := range keys {
		if k.code != "max_nodes" || !isAtOrBelow(k.path, inside) {
			continue
		}
		if at := slices.Index(stopped, k.path); at >= 0 {
			stopped[at] = mark
		}
		keys[i].path = mark
	}
	return keys, stopped
}

// REQ-171 § One walk — below a slot the template does not constrain, the one
// walk with a template meets each walk condition where the two walks' floor
// meets it, and reports the same findings, in the same order, with the same
// coverage, apart from the classes the template walk of the two walks, which
// knows no limit, visits after a visit stop: the device
// CLUSTER of vital_signs.opt holding itself twice, holding SECTIONs nested
// deeper than the default depth limit, and holding a CLUSTER that holds
// itself by value, which only the visit limit stops. The visit limit counts
// the values visited, and at a node the template binds the one walk visits
// the declared attributes in the template's order, where the floor visits
// them in the class's: the one walk reaches the CLUSTER that holds itself
// after another number of visits, so both stop inside it, once, at nodes
// that may differ, and they may have visited other classes before the stop.
func TestREQ171_EngineWithATemplateMatchesTheTwoWalksAtEachWalkCondition(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	const loopPath = bloodPressurePath + "/protocol/items[openEHR-EHR-CLUSTER.device.v1]/items[at0099]"
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
			twoWalkStopped, engineStopped := twoWalks.Coverage.Stopped, engine.Coverage.Stopped
			if tc.code == "max_nodes" {
				twoWalkKeys, twoWalkStopped = atVisitStop(twoWalkKeys, twoWalkStopped, loopPath)
				engineKeys, engineStopped = atVisitStop(engineKeys, engineStopped, loopPath)
			}
			if !slices.Equal(twoWalkKeys, engineKeys) {
				t.Errorf("engine(%s) findings differ from Validate's:\n engine %v\n Validate %v", tc.name, engineKeys, twoWalkKeys)
			}
			if engine.OK != twoWalks.OK || !slices.Equal(engineStopped, twoWalkStopped) || !slices.Equal(engine.Coverage.External, twoWalks.Coverage.External) {
				t.Errorf("engine(%s) OK %t, Stopped %q, External %q; Validate's OK %t, Stopped %q, External %q", tc.name, engine.OK, engineStopped, engine.Coverage.External, twoWalks.OK, twoWalkStopped, twoWalks.Coverage.External)
			}
			// Which classes a walk visits before a visit stop depends on the
			// order it visits a bound node's attributes in, so the classes
			// are compared at the other conditions only.
			if unevaluated, want := engine.Coverage.Unevaluated, twoWalks.Coverage.Unevaluated; tc.code != "max_nodes" && !slices.Equal(unevaluated, want) {
				t.Errorf("engine(%s) Coverage.Unevaluated = %q, Validate's %q", tc.name, unevaluated, want)
			}
			if tc.code == "max_nodes" {
				assertStoppedAtMaxNodes(t, "engine("+tc.name+")", engine)
			}
		})
	}
}
