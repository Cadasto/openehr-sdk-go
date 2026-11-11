package webtemplate_test

// REQ-106 / PROBE-075 — structural parity of the built tree against the
// vendored EHRbase reference (id / rmType / nodeId / aqlPath / min / max).

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

func compileFixture(t *testing.T, path string) *templatecompile.Compiled {
	t.Helper()
	opt, err := readOPTFile(path)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile %s: %v", path, err)
	}
	return c
}

// A nil compiled template must surface the sentinel, never panic (REQ-106).
func TestBuildNil(t *testing.T) {
	if _, err := webtemplate.Build(nil); !errors.Is(err, webtemplate.ErrEmptyTemplate) {
		t.Errorf("Build(nil) err = %v, want ErrEmptyTemplate", err)
	}
	if _, err := webtemplate.Marshal(nil); !errors.Is(err, webtemplate.ErrEmptyTemplate) {
		t.Errorf("Marshal(nil) err = %v, want ErrEmptyTemplate", err)
	}
}

// A nil option must be skipped, not invoked (the variadic is a reserved
// extension point; Build(c, nil) must not panic) (REQ-106).
func TestBuildNilOption(t *testing.T) {
	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c, nil)
	if err != nil {
		t.Fatalf("Build(c, nil): %v", err)
	}
	if wt.Tree == nil {
		t.Fatal("Build(c, nil) returned no tree")
	}
}

// An OPT without a resolvable default language must error, never emit
// "defaultLanguage": "" (spec § REQ-106 Surface MUST).
func TestBuildNoDefaultLanguage(t *testing.T) {
	raw, err := os.ReadFile(referenceDir + "/" + referenceStem + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	// Strip the OPT-level <language> block; the reader accepts the
	// document, leaving Compiled.Language() empty.
	before, rest, foundOpen := bytes.Cut(raw, []byte("<language>"))
	_, after, foundClose := bytes.Cut(rest, []byte("</language>"))
	if !foundOpen || !foundClose {
		t.Fatal("fixture has no <language> block to strip")
	}
	stripped := slices.Concat(before, after)

	opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(stripped))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate (language stripped): %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if _, err := webtemplate.Build(c); !errors.Is(err, webtemplate.ErrNoDefaultLanguage) {
		t.Errorf("Build err = %v, want ErrNoDefaultLanguage", err)
	}
}

// Datatypes outside the core subset must emit their node without inputs
// and without error (spec § REQ-106 inputs MUST NOT error).
func TestExoticDatatypesEmitNoInputs(t *testing.T) {
	cases := map[string]string{ // fixture → expected input-less leaf rmType
		"Test_dv_multimedia_open_constraint.v0.opt":        "DV_MULTIMEDIA",
		"Test_dv_parsable_open_constraint.v0.opt":          "DV_PARSABLE",
		"Test_dv_uri_open_constraint.v0.opt":               "DV_URI",
		"Test_dv_ehr_uri_open_constraint.v0.opt":           "DV_EHR_URI",
		"Test_dv_interval_dv_count_open_constraint.v0.opt": "DV_INTERVAL",
	}
	for fixture, rmType := range cases {
		t.Run(rmType, func(t *testing.T) {
			c := compileFixture(t, "../../../testkit/corpus/templates/"+fixture)
			wt, err := webtemplate.Build(c)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			found := false
			walkOurTree(wt.Tree, func(n *webtemplate.Node) {
				if strings.HasPrefix(n.RMType, rmType) {
					found = true
					if len(n.Inputs) != 0 {
						t.Errorf("%s node carries inputs %+v, want none", n.RMType, n.Inputs)
					}
				}
			})
			if !found {
				t.Errorf("no %s node emitted", rmType)
			}
		})
	}
}

func TestBuildRootShape(t *testing.T) {
	c := compileFixture(t, referenceDir+"/"+referenceStem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if wt.Version != "2.3" {
		t.Errorf("version = %q, want 2.3", wt.Version)
	}
	if wt.Tree == nil || wt.Tree.RMType != "COMPOSITION" {
		t.Fatalf("root = %+v, want COMPOSITION", wt.Tree)
	}
	if wt.Tree.AQLPath != "" {
		t.Errorf("root aqlPath = %q, want empty", wt.Tree.AQLPath)
	}
	if len(wt.Tree.Children) == 0 {
		t.Error("root has no children")
	}
}

// nodeFacts is the structural signature compared for parity.
type nodeFacts struct {
	rmType, nodeID, id string
	min, max           int
}

func (f nodeFacts) String() string {
	return fmt.Sprintf("rmType=%s nodeId=%s id=%s min=%d max=%d", f.rmType, f.nodeID, f.id, f.min, f.max)
}

// walkOurTree visits every node of our tree depth-first.
func walkOurTree(n *webtemplate.Node, visit func(*webtemplate.Node)) {
	visit(n)
	for _, ch := range n.Children {
		walkOurTree(ch, visit)
	}
}

// walkRefTree visits every node of the reference JSON tree depth-first.
func walkRefTree(m map[string]any, visit func(map[string]any)) {
	visit(m)
	if ch, ok := m["children"].([]any); ok {
		for _, c := range ch {
			if cm, ok := c.(map[string]any); ok {
				walkRefTree(cm, visit)
			}
		}
	}
}

func refStr(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// refTree fails the test when the reference document carries no tree.
func refTree(t *testing.T, ref map[string]any) map[string]any {
	t.Helper()
	tree, ok := ref["tree"].(map[string]any)
	if !ok {
		t.Fatal("reference has no object tree")
	}
	return tree
}

// Both parity tests index nodes by aqlPath; siblings sharing a path (one
// at-code cloned under a Multiple attribute) would silently collapse into
// one entry. Since REQ-111 Phase 3 the name predicate separates exactly
// that class — the archetype-reuse-under-slot templates — so indexing by
// path is now sound for the reuse oracles too, not just constrain_test.

// parityFixtures are the vendored references PROBE-075 asserts against.
// constrain_test pins no node name (104 nodes, zero name predicates);
// Corona_Anamnese and GECCO_Diagnose are the REQ-111 oracles, added in
// Phase 4 once name-derived ids and predicated paths made them buildable.
var parityFixtures = []string{referenceStem, "Corona_Anamnese", "GECCO_Diagnose"}

// The categories of documented deviation assertStructuralParity tolerates.
const (
	inContextMinCategory = "in-context min"
	textChoiceCategory   = "text choice"
)

// deviationCounts is the exact number of documented-deviation mismatches
// tolerated per fixture and category (see deviations.md). Pinning the count
// keeps the allowance from widening into cover for a regression.
var deviationCounts = map[string]map[string]int{
	referenceStem:     {},
	"Corona_Anamnese": {},
	"GECCO_Diagnose":  {inContextMinCategory: 14, textChoiceCategory: 1},
}

// textChoiceOutliers are the vendored goldens that give an ELEMENT whose
// value admits DV_TEXT then DV_CODED_TEXT as a DV_TEXT leaf.
var textChoiceOutliers = map[string]bool{"GECCO_Diagnose": true}

// textChoiceDeviation reports whether a mismatch is the documented golden
// outlier: a textChoiceOutliers golden gives an ELEMENT whose value admits a
// DV_TEXT and a DV_CODED_TEXT as a DV_TEXT leaf, where the pinned reference
// code, as this builder, merges the pair into the DV_CODED_TEXT leaf, and the
// reference's own test reports the golden's node as different. Everything else
// about the node must still match exactly.
func textChoiceDeviation(stem string, ref, ours nodeFacts) bool {
	if !textChoiceOutliers[stem] || ref.rmType != "DV_TEXT" || ours.rmType != "DV_CODED_TEXT" {
		return false
	}
	return ref.nodeID == ours.nodeID && ref.id == ours.id && ref.min == ours.min && ref.max == ours.max
}

// minOutliers are the vendored goldens that mark in-context RM-attribute
// leaves min=1: GECCO_Diagnose, and the test_all_types golden whose ACTION
// subtree actionParity compares.
var minOutliers = map[string]bool{"GECCO_Diagnose": true, actionParityStem: true}

// inContextMinDeviation reports whether a mismatch is the documented
// golden outlier: the minOutliers goldens mark in-context RM-attribute leaves
// (language, encoding, subject, composer, territory, setting, start_time,
// time) min=1 where this builder emits min=0. Their OPTs constrain no
// `existence` on them, and constrain_test and Corona_Anamnese agree with
// min=0 — so the golden is the outlier, not the builder. Everything else
// about the node must still match exactly.
func inContextMinDeviation(stem string, ref, ours nodeFacts) bool {
	if !minOutliers[stem] || ref.nodeID != "" {
		return false
	}
	if ref.min != 1 || ours.min != 0 {
		return false
	}
	return ref.rmType == ours.rmType && ref.id == ours.id && ref.max == ours.max
}

func TestStructuralParity(t *testing.T) {
	for _, stem := range parityFixtures {
		t.Run(stem, func(t *testing.T) { assertStructuralParity(t, stem) })
	}
}

func assertStructuralParity(t *testing.T, stem string) {
	t.Helper()
	refByPath := map[string]nodeFacts{}
	walkRefTree(refTree(t, loadReferenceStem(t, stem)), func(m map[string]any) {
		num := func(k string) int {
			if v, ok := m[k].(float64); ok {
				return int(v)
			}
			return 0
		}
		refByPath[refStr(m, "aqlPath")] = nodeFacts{refStr(m, "rmType"), refStr(m, "nodeId"), refStr(m, "id"), num("min"), num("max")}
	})

	c := compileFixture(t, referenceDir+"/"+stem+".opt")
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ourByPath := map[string]nodeFacts{}
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		ourByPath[n.AQLPath] = nodeFacts{n.RMType, n.NodeID, n.ID, n.Min, n.Max}
	})

	var missing, extra, mismatch []string
	allowed := map[string]int{}
	for p, rf := range refByPath {
		of, ok := ourByPath[p]
		if !ok {
			missing = append(missing, p)
			continue
		}
		switch {
		case of == rf:
		case inContextMinDeviation(stem, rf, of):
			allowed[inContextMinCategory]++
		case textChoiceDeviation(stem, rf, of):
			allowed[textChoiceCategory]++
		default:
			mismatch = append(mismatch, fmt.Sprintf("%s\n    ref:  %s\n    ours: %s", p, rf, of))
		}
	}
	// The allowance is pinned to its measured size so it cannot quietly widen
	// into cover for a real regression.
	if want := deviationCounts[stem]; !maps.Equal(allowed, want) {
		t.Errorf("documented deviations = %v, want %v (see deviations.md)", allowed, want)
	}
	for p := range ourByPath {
		if _, ok := refByPath[p]; !ok {
			extra = append(extra, p)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	slices.Sort(mismatch)

	t.Logf("PARITY: ref=%d ours=%d | matched=%d missing=%d extra=%d mismatch=%d",
		len(refByPath), len(ourByPath), len(refByPath)-len(missing)-len(mismatch), len(missing), len(extra), len(mismatch))
	show := func(label string, xs []string, n int) {
		if len(xs) == 0 {
			return
		}
		if len(xs) > n {
			t.Logf("--- %s (%d, first %d):", label, len(xs), n)
			xs = xs[:n]
		} else {
			t.Logf("--- %s (%d):", label, len(xs))
		}
		for _, x := range xs {
			t.Logf("  %s", x)
		}
	}
	show("MISSING (in ref, not ours)", missing, 15)
	show("EXTRA (ours, not ref)", extra, 15)
	show("MISMATCH", mismatch, 15)

	if len(missing) > 0 || len(extra) > 0 || len(mismatch) > 0 {
		t.Errorf("structural parity not yet reached (missing=%d extra=%d mismatch=%d)", len(missing), len(extra), len(mismatch))
	}
}

// actionParityStem is the reference golden whose template holds an ACTION
// and an ELEMENT whose value admits two types: the openEHR_SDK Web Template of
// the cross-format set's test_all_types OPT.
const actionParityStem = "test_all_types.en.v1"

// actionDeltas are the permitted differences between the reference's ACTION
// subtree and this builder's, by category, each named by its aqlPath relative
// to the ACTION (see deviations.md). The lists are pinned whole, so a
// category can neither widen nor shift to another node unnoticed.
var actionDeltas = map[string][]string{
	// The golden marks these in-context leaves min=1, where the builder
	// emits 0 as the other goldens do (inContextMinDeviation).
	"in-context min": {"/encoding", "/language", "/subject", "/time"},
	// The per-careflow-step nodes the builder keeps beside the merged
	// in-context node; the reference drops them.
	"per-step node": {
		"/ism_transition[at0003]", "/ism_transition[at0003]/careflow_step", "/ism_transition[at0003]/current_state",
		"/ism_transition[at0004]", "/ism_transition[at0004]/careflow_step", "/ism_transition[at0004]/current_state",
		"/ism_transition[at0005]", "/ism_transition[at0005]/careflow_step", "/ism_transition[at0005]/current_state",
	},
	// The reference's instruction_details leaves, which the builder does
	// not model.
	"instruction_details": {"/instruction_details/activity_id", "/instruction_details/instruction_id"},
	// The careflow step's local terminology, which the builder leaves out
	// as it does on every coded input.
	"local terminology": {"/ism_transition/careflow_step"},
}

// TestREQ106ActionParity — REQ-106, PROBE-075. The ACTION subtree of the
// test_all_types Web Template matches the reference's golden on structure
// (id, rmType, nodeId, aqlPath, min, max) and inputs, except for the deltas
// actionDeltas names. TestREQ106ChoiceParity compares that golden's `choice`
// ELEMENT; the rest of it is outside both tests, its other entries differing
// for reasons neither touches.
func TestREQ106ActionParity(t *testing.T) {
	var refAction map[string]any
	walkRefTree(refTree(t, loadReferenceStem(t, actionParityStem)), func(m map[string]any) {
		if refStr(m, "rmType") != "ACTION" {
			return
		}
		if refAction != nil {
			t.Fatalf("the %s golden holds two ACTION nodes", actionParityStem)
		}
		refAction = m
	})
	if refAction == nil {
		t.Fatalf("the %s golden holds no ACTION node", actionParityStem)
	}
	base := refStr(refAction, "aqlPath")
	refFacts, refInputs := map[string]nodeFacts{}, map[string]string{}
	walkRefTree(refAction, func(m map[string]any) {
		num := func(k string) int {
			v, _ := m[k].(float64)
			return int(v)
		}
		rel := strings.TrimPrefix(refStr(m, "aqlPath"), base)
		refFacts[rel] = nodeFacts{refStr(m, "rmType"), refStr(m, "nodeId"), refStr(m, "id"), num("min"), num("max")}
		refInputs[rel] = refInputSig(m)
	})

	action := onlyAction(t, buildOPT(t, readFile(t, testAllTypesOPT)))
	if action.AQLPath != base {
		t.Fatalf("ACTION aqlPath = %s, want the golden's %s", action.AQLPath, base)
	}
	ourFacts, ourInputs := map[string]nodeFacts{}, map[string]string{}
	walkOurTree(action, func(n *webtemplate.Node) {
		rel := strings.TrimPrefix(n.AQLPath, base)
		ourFacts[rel] = nodeFacts{n.RMType, n.NodeID, n.ID, n.Min, n.Max}
		ourInputs[rel] = inputSig(n.Inputs)
	})

	got := map[string][]string{}
	var unexplained []string
	for rel, rf := range refFacts {
		of, ok := ourFacts[rel]
		switch {
		case !ok && strings.HasPrefix(rel, "/instruction_details/"):
			got["instruction_details"] = append(got["instruction_details"], rel)
			continue
		case !ok:
			unexplained = append(unexplained, "missing "+rel+": "+rf.String())
			continue
		case of == rf:
		case inContextMinDeviation(actionParityStem, rf, of):
			got["in-context min"] = append(got["in-context min"], rel)
		default:
			unexplained = append(unexplained, fmt.Sprintf("%s\n    ref:  %s\n    ours: %s", rel, rf, of))
		}
		switch rs, ours := refInputs[rel], ourInputs[rel]; {
		case rs == ours:
		case strings.ReplaceAll(rs, "%local", "") == ours:
			got["local terminology"] = append(got["local terminology"], rel)
		default:
			unexplained = append(unexplained, fmt.Sprintf("%s inputs\n    ref:  [%s]\n    ours: [%s]", rel, rs, ours))
		}
	}
	for rel, of := range ourFacts {
		if _, ok := refFacts[rel]; ok {
			continue
		}
		if strings.HasPrefix(rel, "/ism_transition[") {
			got["per-step node"] = append(got["per-step node"], rel)
			continue
		}
		unexplained = append(unexplained, "extra "+rel+": "+of.String())
	}

	t.Logf("ACTION PARITY: ref=%d ours=%d", len(refFacts), len(ourFacts))
	slices.Sort(unexplained)
	for _, u := range unexplained {
		t.Errorf("undocumented delta: %s", u)
	}
	for _, category := range slices.Sorted(maps.Keys(actionDeltas)) {
		g := got[category]
		slices.Sort(g)
		if want := slices.Sorted(slices.Values(actionDeltas[category])); !slices.Equal(g, want) {
			t.Errorf("%s deltas = %d %v, want %d %v (see deviations.md)", category, len(g), g, len(want), want)
		}
	}
}

// choiceParityPath is the aqlPath of the test_all_types EVALUATION's `choice`
// ELEMENT, whose value admits a DV_QUANTITY or a DV_COUNT.
const choiceParityPath = "/content[openEHR-EHR-EVALUATION.test_all_types.v1]/data[at0001]/items[at0009]"

// TestREQ106ChoiceParity — REQ-106, PROBE-075. The test_all_types EVALUATION's
// `choice` ELEMENT, whose value admits two types, matches the reference's
// golden exactly: the ELEMENT node and one child per value type, on structure
// (id, rmType, nodeId, aqlPath, min, max), names and inputs. Its children share
// one aqlPath, so the nodes are compared by their chain of ids.
func TestREQ106ChoiceParity(t *testing.T) {
	var ref map[string]any
	walkRefTree(refTree(t, loadReferenceStem(t, actionParityStem)), func(m map[string]any) {
		if refStr(m, "aqlPath") == choiceParityPath {
			ref = m
		}
	})
	if ref == nil {
		t.Fatalf("the %s golden holds no node at %s", actionParityStem, choiceParityPath)
	}
	refSigs := map[string]string{}
	var walkRef func(m map[string]any, key string)
	walkRef = func(m map[string]any, key string) {
		key += "/" + refStr(m, "id")
		num := func(k string) int {
			v, _ := m[k].(float64)
			return int(v)
		}
		names, _ := m["localizedNames"].(map[string]any)
		descriptions, _ := m["localizedDescriptions"].(map[string]any)
		refSigs[key] = fmt.Sprintf("%s aqlPath=%s name=%s names=%v descriptions=%v inputs=[%s]",
			nodeFacts{refStr(m, "rmType"), refStr(m, "nodeId"), refStr(m, "id"), num("min"), num("max")},
			refStr(m, "aqlPath"), refStr(m, "name"), names, descriptions, refInputSig(m))
		children, _ := m["children"].([]any)
		for _, c := range children {
			if cm, ok := c.(map[string]any); ok {
				walkRef(cm, key)
			}
		}
	}
	walkRef(ref, "")

	var ours *webtemplate.Node
	walkOurTree(buildOPT(t, readFile(t, testAllTypesOPT)).Tree, func(n *webtemplate.Node) {
		if n.AQLPath == choiceParityPath {
			ours = n
		}
	})
	if ours == nil {
		t.Fatalf("Build gave no node at %s", choiceParityPath)
	}
	ourSigs := map[string]string{}
	var walkOurs func(n *webtemplate.Node, key string)
	walkOurs = func(n *webtemplate.Node, key string) {
		key += "/" + n.ID
		ourSigs[key] = fmt.Sprintf("%s aqlPath=%s name=%s names=%v descriptions=%v inputs=[%s]",
			nodeFacts{n.RMType, n.NodeID, n.ID, n.Min, n.Max}, n.AQLPath, n.Name, n.LocalizedNames, n.LocalizedDescriptions, inputSig(n.Inputs))
		for _, ch := range n.Children {
			walkOurs(ch, key)
		}
	}
	walkOurs(ours, "")

	for _, key := range slices.Sorted(maps.Keys(refSigs)) {
		if got, want := ourSigs[key], refSigs[key]; got != want {
			t.Errorf("%s\n    ref:  %s\n    ours: %s", key, want, got)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(ourSigs)) {
		if _, ok := refSigs[key]; !ok {
			t.Errorf("extra %s: %s", key, ourSigs[key])
		}
	}
}

// dropRootTerm returns opt with the term definition for code removed from the
// archetype root archetypeID: the first after that root's archetype_id, which
// an OPT writes after every root nested in it and before its own terms. It
// fails the test unless the OPT names that root once and the term found is the
// root's own.
func dropRootTerm(t *testing.T, opt []byte, archetypeID, code string) []byte {
	t.Helper()
	id := []byte("<value>" + archetypeID + "</value>")
	if n := bytes.Count(opt, id); n != 1 {
		t.Fatalf("the OPT names %s %d times, want once", archetypeID, n)
	}
	at := bytes.Index(opt, id)
	start := bytes.Index(opt[at:], []byte(`<term_definitions code="`+code+`">`))
	if start < 0 || bytes.Contains(opt[at:at+start], []byte("<archetype_id>")) {
		t.Fatalf("%s defines no term %s of its own", archetypeID, code)
	}
	start += at
	end := bytes.Index(opt[start:], []byte("</term_definitions>"))
	if end < 0 {
		t.Fatalf("the term %s of %s is not closed", code, archetypeID)
	}
	return slices.Concat(opt[:start], opt[start+end+len("</term_definitions>"):])
}

// TestNamesComeFromTheNearestArchetypeRoot — REQ-106. A node's name, id and
// description, and a coded entry's label, come from the terms of the node's
// nearest enclosing archetype root alone, as the reference reads them. Where
// that archetype does not define the code, the term an enclosing archetype
// defines under the same code names another concept: the node falls back as a
// node with no term does, and a local code is left out of the coded list, as
// the reference leaves it out.
func TestNamesComeFromTheNearestArchetypeRoot(t *testing.T) {
	raw, err := os.ReadFile(referenceDir + "/Corona_Anamnese.opt")
	if err != nil {
		t.Fatal(err)
	}
	// The COMPOSITION defines at0002 as "Bericht ID", and an archetype around
	// travel_event.v0 defines at0005 as "Status".
	raw = dropRootTerm(t, raw, "openEHR-EHR-OBSERVATION.story.v1", "at0002")
	raw = dropRootTerm(t, raw, "openEHR-EHR-OBSERVATION.travel_event.v0", "at0005")
	opt, err := aom14.ReadOperationalTemplate(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	const (
		eventPath = "/content[openEHR-EHR-OBSERVATION.story.v1]/data[at0001]/events[at0002]"
		codedPath = "/content[openEHR-EHR-SECTION.adhoc.v1,'Risikogebiet']/items[openEHR-EHR-OBSERVATION.travel_event.v0]" +
			"/data[at0001]/events[at0002]/data[at0003]/items[at0004,'Letzte Reise?']/value"
	)
	var event, coded *webtemplate.Node
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		switch n.AQLPath {
		case eventPath:
			event = n
		case codedPath:
			coded = n
		}
	})
	if event == nil || coded == nil {
		t.Fatalf("Build gave no node at %s (%v) or %s (%v)", eventPath, event != nil, codedPath, coded != nil)
	}
	if event.ID != "events" || event.Name != "" || len(event.LocalizedNames) != 0 || len(event.LocalizedDescriptions) != 0 {
		t.Errorf("event %s: id %q, name %q, names %v, descriptions %v; want id \"events\" and no name or description",
			eventPath, event.ID, event.Name, event.LocalizedNames, event.LocalizedDescriptions)
	}
	if len(coded.Inputs) != 1 {
		t.Fatalf("coded %s inputs = %+v, want one", codedPath, coded.Inputs)
	}
	list := coded.Inputs[0].List
	if slices.ContainsFunc(list, func(item webtemplate.InputListItem) bool { return item.Value == "at0005" }) {
		t.Errorf("coded %s list %+v holds at0005, which its archetype no longer defines", codedPath, list)
	}
	if !slices.ContainsFunc(list, func(item webtemplate.InputListItem) bool {
		return item.Value == "at0006" && item.Label == "Ja - national"
	}) {
		t.Errorf("coded %s list %+v lacks at0006 'Ja - national', which its archetype defines", codedPath, list)
	}
}
