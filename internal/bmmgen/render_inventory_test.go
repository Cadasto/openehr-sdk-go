package bmmgen

// REQ-171 Where the RM rules come from: the invariant inventory, emitted from
// the invariants of every class of the pinned RM and BASE BMMs, BASE's
// primitive types included, each classified in a checked-in file; generation
// fails on an invariant without a classification and on a classification of
// no invariant.

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// pinnedClassificationPath is the checked-in classification, from this
// package's directory.
var pinnedClassificationPath = filepath.Join("..", "..", inventoryDir, classificationFile)

// pinnedClassification reads and parses the checked-in classification.
func pinnedClassification(t *testing.T) (raw []byte, classes map[string]invariantClassification) {
	t.Helper()
	raw, err := os.ReadFile(pinnedClassificationPath)
	if err != nil {
		t.Fatalf("read %s: %v", pinnedClassificationPath, err)
	}
	classes, err = parseClassification(raw)
	if err != nil {
		t.Fatalf("parseClassification(%s): %v", pinnedClassificationPath, err)
	}
	return raw, classes
}

// pinnedDeclared returns the plan of the pinned RM target and the invariants
// its schemas declare.
func pinnedDeclared(t *testing.T) (*Plan, []string) {
	t.Helper()
	resolver := bmm.FSResolver{Root: testResources}
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, resolver)
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	schemas, err := loadSchemaChain(context.Background(), TargetRM.RootID, resolver)
	if err != nil {
		t.Fatalf("loadSchemaChain(%s): %v", TargetRM.RootID, err)
	}
	declared, err := declaredInvariants(schemas)
	if err != nil {
		t.Fatalf("declaredInvariants: %v", err)
	}
	return plan, declared
}

// invariantsInFile reads the invariants one pinned BMM file declares under
// section (class_definitions or primitive_types) straight from its JSON, so
// the expectation does not pass through the loader the generator uses. It
// returns the CLASS.Invariant keys and the number of classes declaring one.
func invariantsInFile(t *testing.T, file, section string) (keys []string, classes int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testResources, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var doc map[string]jsontext.Value
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", file, err)
	}
	var defs map[string]struct {
		Invariants map[string]any `json:"invariants"`
	}
	if sec, ok := doc[section]; ok {
		if err := json.Unmarshal(sec, &defs); err != nil {
			t.Fatalf("decode %s %s: %v", file, section, err)
		}
	}
	for class, def := range defs {
		if len(def.Invariants) > 0 {
			classes++
		}
		for name := range def.Invariants {
			keys = append(keys, class+"."+name)
		}
	}
	return keys, classes
}

// REQ-171 — the inventory holds every invariant the pinned RM and BASE BMMs
// declare: the RM's 155 in 68 classes, BASE's 22 in 10 classes and 38 in 7
// primitive types, Interval's among them. Five keys are declared by both
// files (CODE_PHRASE's, and four of AUTHORED_RESOURCE's, which the RM
// redefines), so the inventory holds 210.
func TestInventory_REQ171_ListsEveryPinnedInvariant(t *testing.T) {
	t.Parallel()
	_, declared := pinnedDeclared(t)
	want := map[string]bool{}
	for _, tc := range []struct {
		file, section       string
		invariants, classes int
	}{
		{file: "openehr_rm_1.2.0.bmm.json", section: "class_definitions", invariants: 155, classes: 68},
		{file: "openehr_rm_1.2.0.bmm.json", section: "primitive_types"},
		{file: "openehr_base_1.3.0.bmm.json", section: "class_definitions", invariants: 22, classes: 10},
		{file: "openehr_base_1.3.0.bmm.json", section: "primitive_types", invariants: 38, classes: 7},
	} {
		keys, classes := invariantsInFile(t, tc.file, tc.section)
		if len(keys) != tc.invariants || classes != tc.classes {
			t.Errorf("%s %s declares %d invariants in %d classes, want %d in %d; a BMM bump changed the counts, update them here", tc.file, tc.section, len(keys), classes, tc.invariants, tc.classes)
		}
		for _, k := range keys {
			want[k] = true
		}
	}
	if got := slices.Sorted(maps.Keys(want)); !slices.Equal(declared, got) {
		t.Errorf("declaredInvariants(pinned) = %d keys, want the %d the files declare\ngot:  %q\nwant: %q", len(declared), len(got), declared, got)
	}
	if len(declared) != 210 {
		t.Errorf("declaredInvariants(pinned) holds %d keys, want 210", len(declared))
	}
	for _, key := range []string{"Interval.Lower_included_valid", "Iso8601_date.Year_valid", "CODE_PHRASE.Code_string_valid", "VERSION.Lifecycle_state_ valid", "RESOURCE_DESCRIPTION_ITEM.misuse_valid"} {
		if !slices.Contains(declared, key) {
			t.Errorf("declaredInvariants(pinned) lacks %q", key)
		}
	}
}

// REQ-171 — an invariant of an included schema whose class the including
// schema redefines without it is still in the inventory: the schemas are read
// one by one, not through the merge, in which the redefinition would hide it.
func TestInventory_REQ171_ReadsAnInvariantTheIncludingSchemaRedefines(t *testing.T) {
	t.Parallel()
	resolver := bmm.MapResolver{
		"child_1.0.0": []byte(`{"schema_name": "child", "rm_release": "1.0.0",
			"includes": {"base_1.0.0": {"id": "base_1.0.0"}},
			"class_definitions": {"X": {"name": "X", "invariants": {"A": "a"}}}}`),
		"base_1.0.0": []byte(`{"schema_name": "base", "rm_release": "1.0.0",
			"class_definitions": {"X": {"name": "X", "invariants": {"A": "a", "B": "b"}}},
			"primitive_types": {"P": {"name": "P", "invariants": {"C": "c"}}}}`),
	}
	schemas, err := loadSchemaChain(context.Background(), "child_1.0.0", resolver)
	if err != nil {
		t.Fatalf("loadSchemaChain: %v", err)
	}
	got, err := declaredInvariants(schemas)
	if err != nil {
		t.Fatalf("declaredInvariants: %v", err)
	}
	if want := []string{"P.C", "X.A", "X.B"}; !slices.Equal(got, want) {
		t.Errorf("declaredInvariants(child including base) = %q, want %q", got, want)
	}
	merged, err := bmm.LoadAll("child_1.0.0", resolver)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if fromMerge, _ := declaredInvariants([]*bmm.Schema{merged}); slices.Contains(fromMerge, "X.B") {
		t.Errorf("the merged schema keeps X.B (%q); the case no longer shows why the schemas are read one by one", fromMerge)
	}
}

// REQ-171 Acceptance — generation fails when an invariant of the pinned BMMs
// has no classification, naming it, and when the classification names an
// invariant no pinned BMM declares, naming that.
func TestRenderInventory_REQ171_FailsOnAGapInTheClassification(t *testing.T) {
	t.Parallel()
	plan, declared := pinnedDeclared(t)
	_, classes := pinnedClassification(t)
	if _, err := RenderInventoryFile(plan, declared, classes); err != nil {
		t.Fatalf("RenderInventoryFile(pinned classification) = %v, want none", err)
	}
	for _, tc := range []struct {
		name   string
		doctor func(map[string]invariantClassification)
		want   []string
	}{
		{
			name:   "an invariant left unclassified",
			doctor: func(c map[string]invariantClassification) { delete(c, "DV_AMOUNT.Accuracy_validity") },
			want:   []string{"no classification", "DV_AMOUNT.Accuracy_validity"},
		},
		{
			name:   "an invariant whose name holds a space left unclassified",
			doctor: func(c map[string]invariantClassification) { delete(c, "VERSION.Lifecycle_state_ valid") },
			want:   []string{"no classification", "VERSION.Lifecycle_state_ valid"},
		},
		{
			name: "a classification of no pinned invariant",
			doctor: func(c map[string]invariantClassification) {
				c["DV_QUANTITY.No_such_rule"] = invariantClassification{Kind: kindOutOfScope, Reason: "r"}
			},
			want: []string{"no pinned BMM declares", "DV_QUANTITY.No_such_rule"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doctored := maps.Clone(classes)
			tc.doctor(doctored)
			_, err := RenderInventoryFile(plan, declared, doctored)
			if err == nil {
				t.Fatalf("RenderInventoryFile(%s) succeeded, want an error naming %q", tc.name, tc.want)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("RenderInventoryFile(%s) error = %q, want it to name %q", tc.name, err, w)
				}
			}
		})
	}
}

// REQ-171 — the classification file is refused when an entry is malformed:
// an unknown kind, an evaluated entry without a Go function or with a
// reason, any other without a reason or with a function, an unknown member,
// a duplicate key.
func TestParseClassification_REQ171_RefusesMalformedEntries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src, want string
	}{
		{name: "well formed", src: `{"A.x": {"kind": "evaluated", "function": "checkX"}, "A.y": {"kind": "not_mechanical", "reason": "r"}, "A.z": {"kind": "out_of_scope", "reason": "r"}}`},
		{name: "unknown kind", src: `{"A.x": {"kind": "skipped", "reason": "r"}}`, want: `kind "skipped"`},
		{name: "evaluated without a function", src: `{"A.x": {"kind": "evaluated"}}`, want: "names no Go function"},
		{name: "evaluated with a function that is no identifier", src: `{"A.x": {"kind": "evaluated", "function": "check X"}}`, want: "names no Go function"},
		{name: "evaluated with a reason", src: `{"A.x": {"kind": "evaluated", "function": "checkX", "reason": "r"}}`, want: "gives a reason"},
		{name: "out of scope without a reason", src: `{"A.x": {"kind": "out_of_scope", "reason": " "}}`, want: "gives no reason"},
		{name: "not mechanical with a function", src: `{"A.x": {"kind": "not_mechanical", "reason": "r", "function": "checkX"}}`, want: "names a function"},
		{name: "an unknown member", src: `{"A.x": {"kind": "out_of_scope", "reason": "r", "note": "n"}}`, want: "note"},
		{name: "a duplicate key", src: `{"A.x": {"kind": "out_of_scope", "reason": "r"}, "A.x": {"kind": "out_of_scope", "reason": "r"}}`, want: "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseClassification([]byte(tc.src))
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("parseClassification(%s) = %v, want no error", tc.src, err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("parseClassification(%s) = %v, want an error naming %q", tc.src, err, tc.want)
			}
		})
	}
}

// renderedInventory renders the inventory of the pinned schemas with the
// pinned classification and returns, from the parsed output, each key of
// invariantInventory with its kind identifier and function, and each class
// of classInvariants with its keys.
func renderedInventory(t *testing.T) (plan *Plan, entries map[string][2]string, classes map[string][]string) {
	t.Helper()
	plan, declared := pinnedDeclared(t)
	_, classification := pinnedClassification(t)
	out, err := RenderInventoryFile(plan, declared, classification)
	if err != nil {
		t.Fatalf("RenderInventoryFile: %v", err)
	}
	entries, classes = parseInventory(t, out)
	return plan, entries, classes
}

// parseInventory parses a rendered inventory and returns each key of
// invariantInventory with its kind identifier and function, and each class
// of classInvariants with its keys.
func parseInventory(t *testing.T, out []byte) (entries map[string][2]string, classes map[string][]string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), inventoryFile, out, 0)
	if err != nil {
		t.Fatalf("parse rendered %s: %v", inventoryFile, err)
	}
	entries, classes = map[string][2]string{}, map[string][]string{}
	unquote := func(e ast.Expr) string {
		lit, ok := e.(*ast.BasicLit)
		if !ok {
			t.Fatalf("rendered %s holds %T where a string literal was expected", inventoryFile, e)
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("unquote %s: %v", lit.Value, err)
		}
		return s
	}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, elt := range lit.Elts {
				kv := elt.(*ast.KeyValueExpr)
				key := unquote(kv.Key)
				switch vs.Names[0].Name {
				case "invariantInventory":
					var e [2]string
					for _, f := range kv.Value.(*ast.CompositeLit).Elts {
						fkv := f.(*ast.KeyValueExpr)
						switch fkv.Key.(*ast.Ident).Name {
						case "kind":
							e[0] = fkv.Value.(*ast.Ident).Name
						case "function":
							e[1] = unquote(fkv.Value)
						}
					}
					entries[key] = e
				case "classInvariants":
					classes[key] = nil
					if keys, ok := kv.Value.(*ast.CompositeLit); ok {
						for _, k := range keys.Elts {
							classes[key] = append(classes[key], unquote(k))
						}
					}
				}
			}
		}
	}
	return entries, classes
}

// REQ-171 — the rendered inventory carries every invariant with the kind and
// the function the classification gives it, and, for every concrete class
// the registry decodes and only those, the invariants the class and its
// ancestors declare, sorted: a BASE primitive type's (DV_DATE from
// Iso8601_date, DV_INTERVAL from Interval) and those of an abstract ancestor
// (PERSON from PARTY and LOCATABLE) included.
func TestRenderInventory_REQ171_CarriesKindsAndClassSets(t *testing.T) {
	t.Parallel()
	plan, entries, classes := renderedInventory(t)
	_, classification := pinnedClassification(t)
	if len(entries) != len(classification) {
		t.Errorf("rendered invariantInventory holds %d entries, want the %d the classification gives", len(entries), len(classification))
	}
	for key, c := range classification {
		want := [2]string{kindIdents[c.Kind], c.Function}
		if entries[key] != want {
			t.Errorf("rendered invariantInventory[%q] = %q, want %q", key, entries[key], want)
		}
	}
	var concrete []string
	for _, pc := range plan.ConcreteClasses {
		concrete = append(concrete, pc.BMMName)
	}
	if got := slices.Sorted(maps.Keys(classes)); !slices.Equal(got, slices.Sorted(slices.Values(concrete))) {
		t.Errorf("rendered classInvariants covers %q, want every concrete class %q", got, concrete)
	}
	for class, keys := range classes {
		if !slices.IsSorted(keys) {
			t.Errorf("rendered classInvariants[%s] = %q, want it sorted", class, keys)
		}
		for _, k := range keys {
			if _, ok := entries[k]; !ok {
				t.Errorf("rendered classInvariants[%s] names %q, which the inventory does not hold", class, k)
			}
		}
	}
	for _, tc := range []struct {
		class string
		want  []string
	}{
		{class: "DV_DATE", want: []string{"DV_DATE.Value_valid", "Iso8601_date.Year_valid", "DV_ORDERED.Normal_status_validity", "DV_QUANTIFIED.Magnitude_status_valid"}},
		{class: "DV_INTERVAL", want: []string{"DV_INTERVAL.Limits_consistent", "Interval.Lower_included_valid", "Interval.Limits_comparable"}},
		{class: "PERSON", want: []string{"PARTY.Is_archetype_root", "ACTOR.Roles_valid", "LOCATABLE.Links_valid"}},
		{class: "DV_CODED_TEXT", want: []string{"DV_TEXT.Mappings_valid", "DV_TEXT.Language_valid"}},
	} {
		for _, w := range tc.want {
			if !slices.Contains(classes[tc.class], w) {
				t.Errorf("rendered classInvariants[%s] = %q, want it to hold %q", tc.class, classes[tc.class], w)
			}
		}
	}
	if keys, ok := classes["OBJECT_REF"]; !ok || keys != nil {
		t.Errorf("rendered classInvariants[OBJECT_REF] = %q (listed %t), want nil: OBJECT_REF and its ancestors declare no invariant", keys, ok)
	}
}

// writeValidationTree writes a scratch output tree holding the validation
// package's classification, from raw, and returns its root.
func writeValidationTree(t *testing.T, raw []byte) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, inventoryDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, classificationFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// REQ-171 Acceptance — `make codegen` and `make codegen-check` fail when an
// invariant of the pinned BMMs has no classification: [Run], in both modes,
// over a tree whose classification lacks one, fails naming it. Over a tree
// with the full classification it writes the inventory and then verifies it
// clean. Over a tree with no validation package it fails, in both modes,
// naming the classification file, so a run cannot leave the inventory out
// by accident; only with SkipInventory set does it pass and write none.
func TestRun_REQ171_FailsOnAnUnclassifiedInvariant(t *testing.T) {
	raw, classes := pinnedClassification(t)
	delete(classes, "ELEMENT.Inv_null_reason_valid")
	doctored, err := json.Marshal(classes)
	if err != nil {
		t.Fatal(err)
	}
	gap := writeValidationTree(t, doctored)
	for _, verify := range []bool{false, true} {
		_, err := Run(Options{ResourcesDir: testResources, OutDir: gap, Targets: []Target{TargetRM}, Verify: verify})
		if err == nil || !strings.Contains(err.Error(), "ELEMENT.Inv_null_reason_valid") {
			t.Errorf("Run(verify %t, classification without ELEMENT.Inv_null_reason_valid) = %v, want an error naming it", verify, err)
		}
	}

	full := writeValidationTree(t, raw)
	if _, err := Run(Options{ResourcesDir: testResources, OutDir: full, Targets: []Target{TargetRM}}); err != nil {
		t.Fatalf("Run(full classification) = %v", err)
	}
	inventory := filepath.Join(full, inventoryDir, inventoryFile)
	if _, err := os.Stat(inventory); err != nil {
		t.Fatalf("Run(full classification) wrote no %s: %v", inventory, err)
	}
	result, err := Run(Options{ResourcesDir: testResources, OutDir: full, Targets: []Target{TargetRM}, Verify: true})
	if err != nil {
		t.Fatalf("Run(verify, full classification) = %v", err)
	}
	if len(result.Drifts) != 0 {
		t.Errorf("Run(verify) right after Run = drifts %v, want none", result.Drifts)
	}

	bare := t.TempDir()
	for _, verify := range []bool{false, true} {
		_, err := Run(Options{ResourcesDir: testResources, OutDir: bare, Targets: []Target{TargetRM}, Verify: verify})
		if err == nil || !strings.Contains(err.Error(), classificationFile) {
			t.Errorf("Run(verify %t, tree with no validation package) = %v, want an error naming %s", verify, err, classificationFile)
		}
	}
	skipped := t.TempDir()
	if _, err := Run(Options{ResourcesDir: testResources, OutDir: skipped, Targets: []Target{TargetRM}, SkipInventory: true}); err != nil {
		t.Fatalf("Run(SkipInventory, tree with no validation package) = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skipped, inventoryDir, inventoryFile)); err == nil {
		t.Errorf("Run(SkipInventory, tree with no validation package) wrote an inventory, want none")
	}
}

// REQ-171 Where the RM rules come from — the inventory is a dispatch table,
// not a rules engine: the generator reads the names of the invariants, never
// their expressions. Two schemas that differ only in their expressions give
// the same inventory, and no expression of the pinned BMMs reaches the
// rendered file.
func TestRenderInventory_REQ171_ReadsNoExpression(t *testing.T) {
	t.Parallel()
	declared := func(expr string) []string {
		t.Helper()
		resolver := bmm.MapResolver{"s_1.0.0": []byte(`{"schema_name": "s", "rm_release": "1.0.0",
			"class_definitions": {"X": {"name": "X", "invariants": {"A": ` + strconv.Quote(expr) + `}}}}`)}
		schemas, err := loadSchemaChain(context.Background(), "s_1.0.0", resolver)
		if err != nil {
			t.Fatalf("loadSchemaChain: %v", err)
		}
		keys, err := declaredInvariants(schemas)
		if err != nil {
			t.Fatalf("declaredInvariants: %v", err)
		}
		return keys
	}
	if a, b := declared("not a.is_empty"), declared("a > 0 implies b"); !slices.Equal(a, b) {
		t.Errorf("declaredInvariants differs with the expression alone: %q and %q", a, b)
	}

	plan, all := pinnedDeclared(t)
	_, classification := pinnedClassification(t)
	out, err := RenderInventoryFile(plan, all, classification)
	if err != nil {
		t.Fatalf("RenderInventoryFile: %v", err)
	}
	checked := 0
	for _, file := range []string{"openehr_rm_1.2.0.bmm.json", "openehr_base_1.3.0.bmm.json"} {
		raw, err := os.ReadFile(filepath.Join(testResources, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		type defs map[string]struct {
			Invariants map[string]string `json:"invariants"`
		}
		var doc struct {
			ClassDefinitions defs `json:"class_definitions"`
			PrimitiveTypes   defs `json:"primitive_types"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		for _, section := range []defs{doc.ClassDefinitions, doc.PrimitiveTypes} {
			for _, def := range section {
				for name, expr := range def.Invariants {
					if len(expr) < 20 {
						continue // short enough to occur by chance
					}
					checked++
					if strings.Contains(string(out), expr) {
						t.Errorf("the rendered inventory holds the expression of %s: %q", name, expr)
					}
				}
			}
		}
	}
	if checked < 100 {
		t.Errorf("checked %d expressions of the pinned BMMs, want at least 100; the check has gone blind", checked)
	}
}
