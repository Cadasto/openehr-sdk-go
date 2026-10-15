package validation

// inventory_dispatch_internal_test.go: REQ-171 Where the RM rules come from,
// REQ-112. The floor dispatches its rules through the invariant inventory:
// every invariant invariants.json calls evaluated is bound to the Go function
// it names, and to no other. An invariant another evaluated check decides is
// bound to that check's function. That each function runs on a node of every
// class the inventory says is subject to its invariant, and tells a value
// that breaks the invariant from one that keeps it, is PROBE-077's
// (rmfloor_invariant_matrix_internal_test.go).

import (
	"fmt"
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

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// dispatchFile is the hand-written dispatch the source checks read.
const dispatchFile = "inventory_dispatch.go"

// dispatchSource is what [dispatchBindings] finds in a package's source: for
// each key of the invariantChecks literal, the name of the function the
// entry binds; the keys bound through coveredCheck; one line per entry whose
// recorded name is not the function it binds, or that names no method of
// findings; and the methods of findings the source declares.
type dispatchSource struct {
	bound    map[string]string
	covered  map[string]bool
	problems []string
	methods  map[string]bool
}

// codedEvaluator is the method a codedCheck entry binds: every coded row is
// evaluated by it.
const codedEvaluator = "checkCodedInvariants"

// dispatchBindings parses the non-test Go files in dir and reads the
// invariantChecks literal of file. Each entry must be a call of nodeCheck,
// typeCheck, ruleCheck or coveredCheck whose first argument, a string
// literal, is the name of the method expression (*findings).name its second
// argument is, or a call of codedCheck, which binds checkCodedInvariants.
// Every function an entry binds must be a method of findings the source
// declares. It returns an error when the literal is missing, so the check
// cannot pass over nothing.
func dispatchBindings(dir, file string) (dispatchSource, error) {
	out := dispatchSource{bound: map[string]string{}, covered: map[string]bool{}, methods: map[string]bool{}}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	var lit *ast.CompositeLit
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return out, err
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && len(d.Recv.List) == 1 && isFindingsPointer(d.Recv.List[0].Type) {
					out.methods[d.Name.Name] = true
				}
			case *ast.GenDecl:
				if name != file || d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "invariantChecks" || len(vs.Values) != 1 {
						continue
					}
					if cl, ok := vs.Values[0].(*ast.CompositeLit); ok {
						lit = cl
					}
				}
			}
		}
	}
	if lit == nil {
		return out, fmt.Errorf("found no invariantChecks literal in %s, so the check has gone blind", file)
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			out.problems = append(out.problems, "an invariantChecks entry without a key")
			continue
		}
		key, ok := stringLit(kv.Key)
		if !ok {
			out.problems = append(out.problems, "an invariantChecks entry whose key is not a string literal")
			continue
		}
		fn, covered, problem := boundFunction(kv.Value)
		if problem != "" {
			out.problems = append(out.problems, key+": "+problem)
			continue
		}
		out.bound[key] = fn
		if covered {
			out.covered[key] = true
		}
	}
	for key, fn := range out.bound {
		if !out.methods[fn] {
			out.problems = append(out.problems, fmt.Sprintf("%s: binds %s, which is no method of findings", key, fn))
		}
	}
	slices.Sort(out.problems)
	return out, nil
}

// boundFunction returns the name of the function an invariantChecks entry
// value binds and whether it binds it through coveredCheck, or what is wrong
// with it.
func boundFunction(e ast.Expr) (fn string, covered bool, problem string) {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return "", false, "not a call of a binder"
	}
	binder, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", false, "not a call of a binder"
	}
	switch binder.Name {
	case "codedCheck":
		return codedEvaluator, false, ""
	case "nodeCheck", "typeCheck", "ruleCheck":
	case "coveredCheck":
		covered = true
	default:
		return "", false, "binds through " + binder.Name + ", which is no binder"
	}
	if len(call.Args) != 2 {
		return "", false, binder.Name + " with other than two arguments"
	}
	name, ok := stringLit(call.Args[0])
	if !ok {
		return "", false, "records its function's name as other than a string literal"
	}
	sel, ok := call.Args[1].(*ast.SelectorExpr)
	if !ok {
		return "", false, "binds no method expression of *findings"
	}
	paren, ok := sel.X.(*ast.ParenExpr)
	if !ok || !isFindingsPointer(paren.X) {
		return "", false, "binds no method expression of *findings"
	}
	if sel.Sel.Name != name {
		return "", false, fmt.Sprintf("records the name %q beside the method %s", name, sel.Sel.Name)
	}
	return name, covered, ""
}

// isFindingsPointer reports whether e is the type *findings.
func isFindingsPointer(e ast.Expr) bool {
	star, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "findings"
}

// stringLit returns the value of e when e is a string literal.
func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// REQ-171 Where the RM rules come from — the dispatch binds every invariant
// invariants.json calls evaluated, and only those, each to the function the
// classification names: the name recorded beside each function is the method
// it binds, every such method is declared, and a coded entry binds the row
// whose class and invariant are its key. An invariant another evaluated check
// decides is bound through coveredCheck, and only such a one, and names the
// mandatory-presence check or a function the dispatch runs for an invariant
// of its own, since a covering function nothing runs would decide nothing.
func TestInventory_REQ171_DispatchMatchesTheClassification(t *testing.T) {
	t.Parallel()
	src, err := dispatchBindings(".", dispatchFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range src.problems {
		t.Errorf("%s: %s", dispatchFile, p)
	}
	if !src.methods[codedEvaluator] {
		t.Errorf("findings declares no %s, which every codedCheck entry binds", codedEvaluator)
	}
	if got := codedCheck(&ruleEntryLanguage).function; got != codedEvaluator {
		t.Errorf("codedCheck records the function %q, want %q", got, codedEvaluator)
	}
	if !src.methods[mandatoryCheck] {
		t.Errorf("findings declares no %s, the mandatory-presence check build counts as run on every class", mandatoryCheck)
	}
	dispatched := map[string]bool{}
	for _, c := range invariantChecks {
		if c.run != nil {
			dispatched[c.function] = true
		}
	}
	for _, key := range slices.Sorted(maps.Keys(invariantInventory)) {
		e := invariantInventory[key]
		fn, bound := src.bound[key]
		switch {
		case e.kind == invariantEvaluated && !bound:
			t.Errorf("%s is evaluated in invariants.json (by %s) but invariantChecks binds no function to it", key, e.function)
		case e.kind != invariantEvaluated && bound:
			t.Errorf("invariantChecks binds %s to %s, but invariants.json does not call it evaluated", key, fn)
		case bound && fn != e.function:
			t.Errorf("invariantChecks binds %s to %s, but invariants.json names %s", key, fn, e.function)
		}
	}
	for key := range src.bound {
		if _, ok := invariantInventory[key]; !ok {
			t.Errorf("invariantChecks binds %s, which is not in the inventory", key)
		}
	}
	if len(src.bound) != len(invariantChecks) {
		t.Errorf("the source scan read %d entries of invariantChecks, which holds %d; the scan has gone blind", len(src.bound), len(invariantChecks))
	}
	for key, c := range invariantChecks {
		if fn := src.bound[key]; c.function != fn {
			t.Errorf("invariantChecks[%s].function = %q, want %q, the function the source binds", key, c.function, fn)
		}
		if c.coded != nil && c.coded.name() != key {
			t.Errorf("invariantChecks[%s] binds the coded row %s", key, c.coded.name())
		}
		if kinds := btoi(c.run != nil) + btoi(c.coded != nil) + btoi(c.covered); kinds != 1 {
			t.Errorf("invariantChecks[%s] binds %d of a function, a coded row and a covering check, want one", key, kinds)
		}
		if c.covered != src.covered[key] {
			t.Errorf("invariantChecks[%s] covered = %t, but the source binds it through coveredCheck: %t", key, c.covered, src.covered[key])
		}
		if c.covered && c.function != mandatoryCheck && !dispatched[c.function] {
			t.Errorf("invariantChecks[%s] is covered by %s, which the dispatch runs for no invariant of its own, so nothing decides it", key, c.function)
		}
	}
	if len(src.covered) == 0 {
		t.Error("the source binds no invariant through coveredCheck; the covered checks have gone blind")
	}
}

// btoi returns 1 for true and 0 for false.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestInventory_REQ171_DispatchCheckFailsOnEachGap runs the source check over
// one-file packages, so the test above is shown to fail on each gap it exists
// to catch, and to pass an entry that binds the function it names.
func TestInventory_REQ171_DispatchCheckFailsOnEachGap(t *testing.T) {
	t.Parallel()
	const header = "package p\n\ntype findings struct{}\n\nfunc (f *findings) checkA(any, string) {}\nfunc (f *findings) checkCodedInvariants() {}\n\n"
	for _, tc := range []struct {
		name, body, want string
	}{
		{name: "a function bound under its own name", body: `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck("checkA", (*findings).checkA), "A.y": codedCheck(&ruleY), "A.z": coveredCheck("checkA", (*findings).checkA)}`},
		{name: "a covering function under another name", body: `var invariantChecks = map[string]invariantCheck{"A.z": coveredCheck("checkB", (*findings).checkA)}`, want: `records the name "checkB" beside the method checkA`},
		{name: "a covering function findings does not declare", body: `var invariantChecks = map[string]invariantCheck{"A.z": coveredCheck("checkB", (*findings).checkB)}`, want: "checkB, which is no method"},
		{name: "a name beside another method", body: `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck("checkB", (*findings).checkA)}`, want: `records the name "checkB" beside the method checkA`},
		{name: "a method findings does not declare", body: `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck("checkB", (*findings).checkB)}`, want: "checkB, which is no method"},
		{name: "a name that is no literal", body: `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck(name, (*findings).checkA)}`, want: "other than a string literal"},
		{name: "a function that is no method expression", body: `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck("checkA", checkA)}`, want: "no method expression"},
		{name: "an unknown binder", body: `var invariantChecks = map[string]invariantCheck{"A.x": bind("checkA", (*findings).checkA)}`, want: "bind, which is no binder"},
		{name: "a value that is no call", body: `var invariantChecks = map[string]invariantCheck{"A.x": check}`, want: "not a call"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, dispatchFile), []byte(header+tc.body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := dispatchBindings(dir, dispatchFile)
			if err != nil {
				t.Fatalf("dispatchBindings(%s) error = %v", tc.name, err)
			}
			switch {
			case tc.want == "" && len(got.problems) != 0:
				t.Errorf("dispatchBindings(%s) = %q, want no problem", tc.name, got.problems)
			case tc.want != "" && (len(got.problems) != 1 || !strings.Contains(got.problems[0], tc.want)):
				t.Errorf("dispatchBindings(%s) = %q, want one problem naming %q", tc.name, got.problems, tc.want)
			}
		})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dispatchFile), []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatchBindings(dir, dispatchFile); err == nil {
		t.Errorf("dispatchBindings(no invariantChecks) succeeded, want an error: the check would pass over nothing")
	}
	both := t.TempDir()
	body := `var invariantChecks = map[string]invariantCheck{"A.x": nodeCheck("checkA", (*findings).checkA), "A.z": coveredCheck("checkA", (*findings).checkA)}`
	if err := os.WriteFile(filepath.Join(both, dispatchFile), []byte(header+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := dispatchBindings(both, dispatchFile)
	if err != nil {
		t.Fatalf("dispatchBindings(an evaluator and a covered binding) error = %v", err)
	}
	if !got.covered["A.z"] || got.covered["A.x"] || got.bound["A.z"] != "checkA" {
		t.Errorf("dispatchBindings(an evaluator and a covered binding) covered %v bound %v, want A.z alone covered, by checkA", got.covered, got.bound)
	}
}

// REQ-171 Acceptance — the inventory covers every class the registry
// decodes, and the floor holds rules for each: a class the registry knows
// that the inventory does not would be dispatched nothing.
func TestInventory_REQ171_CoversEveryRegisteredClass(t *testing.T) {
	t.Parallel()
	names := typereg.Default.Names()
	if len(names) == 0 {
		t.Fatal("the registry knows no class; registrations missing?")
	}
	if got := slices.Sorted(maps.Keys(classInvariants)); !slices.Equal(got, slices.Sorted(slices.Values(names))) {
		t.Errorf("classInvariants covers %q, want every class the registry decodes %q", got, names)
	}
	for _, name := range names {
		if floorRules.byClass[name] == nil {
			t.Errorf("the floor holds no rules for %s, which the registry decodes", name)
		}
	}
}

// REQ-171 Coverage — a root of every class the registry decodes lists, in
// Coverage.Unevaluated, every invariant of its class and its ancestors the
// floor does not evaluate, sorted and once each, and no invariant it
// evaluates.
func TestCoverage_REQ171_EveryRootListsItsUnevaluatedInvariants(t *testing.T) {
	t.Parallel()
	listed := 0
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		got := ValidateRM(ctor()).Coverage.Unevaluated
		for i, key := range got {
			if i > 0 && got[i-1] >= key {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it sorted with no repeat", name, got)
				break
			}
			if pinnedSource.evaluated(key) {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated lists %s, which the floor evaluates", name, key)
			}
		}
		for _, key := range classInvariants[name] {
			if !pinnedSource.evaluated(key) && !slices.Contains(got, key) {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it to list %s", name, got, key)
			}
		}
		if len(got) > 0 {
			listed++
		}
	}
	if listed == 0 {
		t.Error("no registered root lists an unevaluated invariant; the check has gone blind")
	}
}

// REQ-171 Where the RM rules come from — the dispatch runs, on a node of a
// class, the attribute checks that apply to it, then the evaluator of each
// evaluated invariant the class is subject to, once for a node however many
// invariants it serves and once per invariant for one that reports the
// invariant that dispatched it, and hands the class's evaluated coded rows to
// the coded pass. An invariant that is not evaluated, or that is evaluated
// and bound to nothing, runs nothing and is listed as unevaluated, sorted. An
// invariant another check decides runs nothing either: it is evaluated on a
// class where that check runs, the mandatory-presence check running on every
// class, and listed as unevaluated on one where it does not.
func TestInventory_REQ171_BuildDispatchesTheEvaluatedInvariants(t *testing.T) {
	t.Parallel()
	var ran []string
	record := func(name string) func(*findings, floorNode, string) {
		return func(_ *findings, _ floorNode, rule string) { ran = append(ran, name+" "+rule) }
	}
	rowA, rowB := codedRule{owner: "P", invariant: "c1"}, codedRule{owner: "P", invariant: "c2"}
	src := inventorySource{
		classes: map[string][]string{
			"A": {"A.ca", "A.cm", "A.cs", "A.e1", "A.e2", "A.n", "P.c1", "P.c2", "P.r1", "P.r2"},
			"B": {"A.ca", "A.cm", "A.cs", "B.unbound"},
			"C": nil,
		},
		inventory: map[string]inventoryEntry{
			"A.e1":      {kind: invariantEvaluated, function: "shared"},
			"A.e2":      {kind: invariantEvaluated, function: "shared"},
			"A.n":       {kind: invariantNotMechanical},
			"P.c1":      {kind: invariantEvaluated, function: "checkCodedInvariants"},
			"P.c2":      {kind: invariantEvaluated, function: "checkCodedInvariants"},
			"P.r1":      {kind: invariantEvaluated, function: "perRule"},
			"P.r2":      {kind: invariantEvaluated, function: "perRule"},
			"B.unbound": {kind: invariantEvaluated, function: "missing"},
			"A.ca":      {kind: invariantEvaluated, function: "attribute"},
			"A.cm":      {kind: invariantEvaluated, function: mandatoryCheck},
			"A.cs":      {kind: invariantEvaluated, function: "shared"},
		},
		checks: map[string]invariantCheck{
			"A.e1": {function: "shared", run: record("shared")},
			"A.e2": {function: "shared", run: record("shared")},
			"P.c1": {function: "checkCodedInvariants", coded: &rowA},
			"P.c2": {function: "checkCodedInvariants", coded: &rowB},
			"P.r1": {function: "perRule", perRule: true, run: record("perRule")},
			"P.r2": {function: "perRule", perRule: true, run: record("perRule")},
			"A.ca": {function: "attribute", covered: true},
			"A.cm": {function: mandatoryCheck, covered: true},
			"A.cs": {function: "shared", covered: true},
		},
		attributes: []attributeCheck{
			{class: "A", check: invariantCheck{function: "attribute", run: record("attribute")}},
			{class: "B", check: invariantCheck{function: "attribute", run: record("attribute")}},
		},
	}
	rules := src.build()
	if want := []string{"A.cs", "A.n", "B.unbound"}; !slices.Equal(rules.unevaluated, want) {
		t.Errorf("build().unevaluated = %q, want %q", rules.unevaluated, want)
	}
	for _, tc := range []struct {
		class       string
		ran         []string
		coded       []*codedRule
		unevaluated []string
	}{
		{class: "A", ran: []string{"attribute ", "shared A.e1", "perRule P.r1", "perRule P.r2"}, coded: []*codedRule{&rowA, &rowB}, unevaluated: []string{"A.n"}},
		{class: "B", ran: []string{"attribute "}, unevaluated: []string{"A.cs", "B.unbound"}},
		{class: "C"},
	} {
		r := rules.byClass[tc.class]
		if r == nil || rules.byIndex[r.index] != r {
			t.Fatalf("build() holds no rules for %s at its index", tc.class)
		}
		ran = nil
		for _, c := range r.checks {
			c.run(nil, floorNode{}, c.rule)
		}
		if !slices.Equal(ran, tc.ran) {
			t.Errorf("build() runs %q on %s, want %q", ran, tc.class, tc.ran)
		}
		if !slices.Equal(r.coded, tc.coded) {
			t.Errorf("build() hands %d coded rows to the coded pass on %s, want %d", len(r.coded), tc.class, len(tc.coded))
		}
		var unevaluated []string
		r.unevaluated.each(func(i int) { unevaluated = append(unevaluated, rules.unevaluated[i]) })
		if !slices.Equal(unevaluated, tc.unevaluated) {
			t.Errorf("build() lists %q as unevaluated on %s, want %q", unevaluated, tc.class, tc.unevaluated)
		}
	}
}

// REQ-171 Where the RM rules come from, REQ-112 Coded invariants — the coded
// pass checks only the rows the inventory hands it for the node's class,
// whatever other coded attributes the value carries.
func TestFloor_REQ171_CodedPassChecksOnlyTheRowsItIsGiven(t *testing.T) {
	t.Parallel()
	bogus := rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "no-such-code"}
	obs := &rm.Observation{Language: bogus, Encoding: bogus}
	for _, tc := range []struct {
		name  string
		rules []*codedRule
		want  []string
	}{
		{name: "both rows", rules: []*codedRule{&ruleEntryLanguage, &ruleEntryEncoding}, want: []string{"ENTRY.Language_valid", "ENTRY.Encoding_valid"}},
		{name: "encoding only", rules: []*codedRule{&ruleEntryEncoding}, want: []string{"ENTRY.Encoding_valid"}},
		{name: "no row", rules: nil},
	} {
		var f findings
		f.checkCodedInvariants(obs, "OBSERVATION", "/", tc.rules)
		var got []string
		for _, i := range f.issues {
			got = append(got, i.Rule)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("checkCodedInvariants(OBSERVATION breaking both rows, %s) reports %q, want %q", tc.name, got, tc.want)
		}
	}
}

// REQ-112 Archetype roots, REQ-107 — the classes the floor checks as
// archetype roots, those subject to an evaluated Is_archetype_root
// invariant in the inventory, are the classes the instance generator treats
// as archetype roots (internal/rmroots), so the two cannot drift apart.
func TestInventory_REQ112_ArchetypeRootsMatchTheGenerator(t *testing.T) {
	t.Parallel()
	roots := 0
	for _, class := range slices.Sorted(maps.Keys(classInvariants)) {
		floor := slices.ContainsFunc(classInvariants[class], func(key string) bool {
			return strings.HasSuffix(key, ".Is_archetype_root") && pinnedSource.evaluated(key)
		})
		if floor != rmroots.IsArchetypeRoot(class) {
			t.Errorf("%s: the floor checks it as an archetype root: %t; rmroots.IsArchetypeRoot: %t", class, floor, rmroots.IsArchetypeRoot(class))
		}
		if floor {
			roots++
		}
	}
	if roots != 13 {
		t.Errorf("the floor checks %d classes as archetype roots, want the 13 the pinned RM makes roots", roots)
	}
}
