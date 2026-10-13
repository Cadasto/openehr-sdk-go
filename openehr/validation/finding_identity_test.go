package validation_test

// finding_identity_test.go: REQ-171 § Finding identity. Every issue an
// instance validator reports names its Layer and carries a non-empty Rule,
// except the guards and the walk conditions, which carry neither; so does
// every issue from ValidateAQL. A rule the pinned BMM names is spelt
// CLASS.Invariant as that BMM spells it. The checks here run three ways: over
// every vendored corpus document (the census), over the source of every emit
// site (so a site the corpus never reaches still fails when it stops setting
// either field), and over the BMMs (so a rule name cannot drift from the
// model).

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// noIdentityCodes are the codes of the issues that judge the call or the
// walk rather than the instance: the guards and the walk conditions. They
// carry no layer and an empty rule (REQ-171 § Finding identity).
var noIdentityCodes = map[string]bool{
	"nil_root":        true,
	"nil_template":    true,
	"nil_composition": true,
	"nil_party":       true,
	"nil_folder":      true,
	"nil_ehr_status":  true,
	"nil_ehr_access":  true,
	"rm_type_unknown": true,
	"invalid_shape":   true,
	"max_depth":       true,
	"max_nodes":       true,
	"cyclic_value":    true,
}

// fixedRuleCodes are the codes that are specific to one RM rule, with that
// rule: an issue with one of them carries both the code and the rule's name.
var fixedRuleCodes = map[string]string{
	"lower_included_valid": "Interval.Lower_included_valid",
	"upper_included_valid": "Interval.Upper_included_valid",
	"mappings_valid":       "DV_TEXT.Mappings_valid",
	"term_mapping_match":   "TERM_MAPPING.Match_valid",
	"rm_version_valid":     "ARCHETYPED.Rm_version_valid",
}

// classInvariant is the shape of a rule the BMM names: CLASS.Invariant.
var classInvariant = regexp.MustCompile(`^[A-Z][A-Za-z_]*\.[A-Z][a-z][A-Za-z_]*$`)

// bmmInvariants holds every invariant the pinned RM and BASE BMMs declare.
var bmmInvariants = sync.OnceValues(loadBMMInvariants)

// loadBMMInvariants reads every invariant the pinned RM and BASE BMMs
// declare, as CLASS.Invariant: from the class definitions of both and from
// the primitive types of BASE, where Interval declares its own.
func loadBMMInvariants() (map[string]bool, error) {
	type classes map[string]struct {
		Invariants map[string]any `json:"invariants"`
	}
	names := map[string]bool{}
	for _, file := range []string{"openehr_rm_1.2.0.bmm.json", "openehr_base_1.3.0.bmm.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", file))
		if err != nil {
			return nil, err
		}
		var schema struct {
			ClassDefinitions classes `json:"class_definitions"`
			PrimitiveTypes   classes `json:"primitive_types"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			return nil, fmt.Errorf("decode %s: %w", file, err)
		}
		for _, defs := range []classes{schema.ClassDefinitions, schema.PrimitiveTypes} {
			for class, def := range defs {
				for inv := range def.Invariants {
					names[class+"."+inv] = true
				}
			}
		}
	}
	if len(names) == 0 {
		return nil, errors.New("the pinned BMMs declare no invariant, so the check has gone blind")
	}
	return names, nil
}

// mustBMMInvariants returns the invariants of the pinned BMMs, or stops the
// test.
func mustBMMInvariants(t *testing.T) map[string]bool {
	t.Helper()
	names, err := bmmInvariants()
	if err != nil {
		t.Fatalf("read the pinned BMMs: %v", err)
	}
	return names
}

// identityProblem returns what is wrong with the Layer and the Rule of i, or
// "" when they are what REQ-171 § Finding identity gives an issue of its
// code: none on a guard or a walk condition; otherwise a layer, and a rule of
// that layer's vocabulary that fits the code, a CLASS.Invariant rule spelt as
// the pinned BMMs spell it.
func identityProblem(i validation.Issue, bmm map[string]bool) string {
	if noIdentityCodes[i.Code] {
		if i.Layer != validation.LayerNone || i.Rule != "" {
			return fmt.Sprintf("a guard or walk condition carries layer %q and rule %q, want neither", i.Layer, i.Rule)
		}
		return ""
	}
	if i.Layer == validation.LayerNone || i.Rule == "" {
		return fmt.Sprintf("carries layer %q and rule %q, want both", i.Layer, i.Rule)
	}
	if classInvariant.MatchString(i.Rule) && !bmm[i.Rule] {
		return fmt.Sprintf("rule %q is not an invariant of the pinned BMMs", i.Rule)
	}
	tmpl, rmLayer := i.Layer == validation.LayerTemplate, i.Layer == validation.LayerRM
	var ok bool
	switch code := i.Code; {
	case code == "required":
		ok = tmpl && i.Rule == "existence" || rmLayer && i.Rule == "mandatory"
	case code == "prohibited":
		ok = tmpl && i.Rule == "existence"
	case code == "cardinality":
		ok = tmpl && (i.Rule == "cardinality" || i.Rule == "occurrences") || rmLayer && i.Rule == "cardinality"
	case code == "rm_type_mismatch", code == "alternative_mismatch":
		ok = tmpl && i.Rule == "type"
	case code == "archetype_id_mismatch", code == "node_id_mismatch":
		ok = tmpl && i.Rule == "identity"
	case code == "slot_fill":
		ok = tmpl && i.Rule == "slot"
	case strings.HasPrefix(code, "primitive_"):
		ok = tmpl && i.Rule == strings.TrimPrefix(code, "primitive_")
	case code == "rm_invariant":
		ok = rmLayer && (i.Rule == "mandatory" || i.Rule == "precision_range" || classInvariant.MatchString(i.Rule))
	case code == "is_archetype_root":
		ok = rmLayer && classInvariant.MatchString(i.Rule) && strings.HasSuffix(i.Rule, ".Is_archetype_root")
	case code == "code_not_in_value_set":
		ok = rmLayer && classInvariant.MatchString(i.Rule)
	case fixedRuleCodes[code] != "":
		ok = rmLayer && i.Rule == fixedRuleCodes[code]
	default:
		return fmt.Sprintf("code %q is not one this check knows; add what layer and rule it carries", code)
	}
	if !ok {
		return fmt.Sprintf("layer %q and rule %q do not fit code %q", i.Layer, i.Rule, i.Code)
	}
	return ""
}

// assertIdentities fails the test for every issue of r whose Layer and Rule
// identityProblem rejects, and returns how many issues it checked.
func assertIdentities(t *testing.T, call string, r validation.Result, bmm map[string]bool) int {
	t.Helper()
	for _, i := range r.Issues {
		if p := identityProblem(i, bmm); p != "" {
			t.Errorf("%s: %s at %q: %s (REQ-171 § Finding identity)", call, i.Code, i.Path, p)
		}
	}
	return len(r.Issues)
}

// identityKeys renders each issue as "code path layer rule", sorted, so a test
// can compare where every issue comes from as well as where it sits.
func identityKeys(issues []validation.Issue) []string {
	keys := make([]string, 0, len(issues))
	for _, i := range issues {
		keys = append(keys, i.Code+" "+i.Path+" "+i.Layer.String()+" "+i.Rule)
	}
	slices.Sort(keys)
	return keys
}

// REQ-171 — Layer.String names each layer, and an out-of-range value shows
// its number.
func TestLayer_REQ171_String(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		layer validation.Layer
		want  string
	}{
		{layer: validation.LayerNone, want: ""},
		{layer: validation.LayerTemplate, want: "template"},
		{layer: validation.LayerRM, want: "rm"},
		{layer: validation.Layer(3), want: "layer(3)"},
		{layer: validation.Layer(255), want: "layer(255)"},
	} {
		if got := tc.layer.String(); got != tc.want {
			t.Errorf("Layer(%d).String() = %q, want %q", uint8(tc.layer), got, tc.want)
		}
	}
	var zero validation.Issue
	if zero.Layer != validation.LayerNone {
		t.Errorf("the zero Issue has Layer %q, want LayerNone", zero.Layer)
	}
}

// REQ-171 Acceptance — every issue of a run over the vendored corpus carries
// a Layer and a non-empty Rule that fit its code, and every guard or walk
// condition among them carries neither. Each document is validated by the
// floor alone and, where the corpus holds its OPT, against that template;
// each EHR_STATUS sample also goes through the presence-aware Bytes entry,
// the invalid ones included.
func TestREQ171_CorpusCensus(t *testing.T) {
	bmm := mustBMMInvariants(t)
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("ListCompositionJSON: %v", err)
	}
	checked, templated := 0, 0
	codes := map[string]bool{}
	tally := func(r validation.Result) {
		for _, i := range r.Issues {
			codes[i.Code] = true
		}
	}
	for _, rel := range rels {
		factory, ok := fixtures.FactoryForJSONRel(rel)
		if !ok {
			continue
		}
		body, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel.Rel, err)
		}
		root := factory()
		if err := canjson.Unmarshal(body, root); err != nil {
			continue // a decode failure is the codec's tests' concern
		}
		r := validation.ValidateRM(root)
		checked += assertIdentities(t, "ValidateRM("+rel.Rel+")", r, bmm)
		tally(r)
		if rel.Kind != "compositions" {
			continue
		}
		c, ok := compileCorpusTemplate(t, rel.Template)
		if !ok {
			continue
		}
		r = validation.Validate(root, c)
		checked += assertIdentities(t, "Validate("+rel.Rel+")", r, bmm)
		tally(r)
		templated++
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
		r := validation.ValidateRMEHRStatusBytes(body)
		checked += assertIdentities(t, "ValidateRMEHRStatusBytes("+filepath.Base(path)+")", r, bmm)
		tally(r)
	}
	if templated == 0 || len(statuses) == 0 || checked == 0 {
		t.Fatalf("the census validated %d documents against a template, %d EHR_STATUS samples and %d issues; the corpus listing has gone blind", templated, len(statuses), checked)
	}
	t.Logf("checked %d issues over %d codes: %v", checked, len(codes), slices.Sorted(maps.Keys(codes)))
}

// compileCorpusTemplate compiles the corpus OPT of templateID; ok is false
// when the corpus holds none or it does not compile.
func compileCorpusTemplate(t *testing.T, templateID string) (*templatecompile.Compiled, bool) {
	t.Helper()
	path := fixtures.TemplateOpt(templateID)
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	opt, err := template.ParseFile(path)
	if err != nil {
		return nil, false
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		return nil, false
	}
	return c, true
}

// REQ-171 — the guards, the walk conditions and every issue from ValidateAQL
// carry no layer and an empty rule: each entry point's nil guard, a root
// outside the closed RM set, input that does not decode, a value that holds
// itself, a graph past the depth limit, one past the visit limit, and the
// lint issues of a query.
func TestREQ171_GuardsAndWalkConditionsCarryNoIdentity(t *testing.T) {
	bmm := mustBMMInvariants(t)
	c := mustCompile(t, "vital_signs")
	deep := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "bottom"}}
	for range 300 {
		deep = &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "level"}, Items: []rm.ContentItem{deep}}
	}
	cyclic := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "s"}}
	cyclic.Items = []rm.ContentItem{cyclic}
	tests := []struct {
		name string
		code string // a code the call must report
		call func() validation.Result
	}{
		{name: "ValidateComposition(nil)", code: "nil_composition", call: func() validation.Result { return validation.ValidateComposition(nil, c) }},
		{name: "Validate(nil root)", code: "nil_root", call: func() validation.Result { return validation.Validate(nil, c) }},
		{name: "Validate(nil template)", code: "nil_template", call: func() validation.Result { return validation.Validate(validVitalSignsComposition(), nil) }},
		{name: "Validate(template without a root)", code: "nil_template", call: func() validation.Result {
			return validation.Validate(validVitalSignsComposition(), &templatecompile.Compiled{})
		}},
		{name: "ValidateDemographic(nil)", code: "nil_party", call: func() validation.Result { return validation.ValidateDemographic(nil, c) }},
		{name: "ValidateFolder(nil)", code: "nil_folder", call: func() validation.Result { return validation.ValidateFolder(nil, c) }},
		{name: "ValidateEHRStatus(nil)", code: "nil_ehr_status", call: func() validation.Result { return validation.ValidateEHRStatus(nil, c) }},
		{name: "Validate(root outside the RM set)", code: "rm_type_unknown", call: func() validation.Result { return validation.Validate(struct{}{}, c) }},
		{name: "ValidateRM(nil)", code: "nil_root", call: func() validation.Result { return validation.ValidateRM(nil) }},
		{name: "ValidateRM(root outside the RM set)", code: "rm_type_unknown", call: func() validation.Result { return validation.ValidateRM(struct{}{}) }},
		{name: "ValidateRMFolder(nil)", code: "nil_folder", call: func() validation.Result { return validation.ValidateRMFolder(nil) }},
		{name: "ValidateRMEHRStatus(nil)", code: "nil_ehr_status", call: func() validation.Result { return validation.ValidateRMEHRStatus(nil) }},
		{name: "ValidateRMEHRAccess(nil)", code: "nil_ehr_access", call: func() validation.Result { return validation.ValidateRMEHRAccess(nil) }},
		{name: "ValidateRMDemographic(nil)", code: "nil_party", call: func() validation.Result { return validation.ValidateRMDemographic(nil) }},
		{name: "ValidateRMEHRStatusBytes(array)", code: "invalid_shape", call: func() validation.Result { return validation.ValidateRMEHRStatusBytes([]byte(`[]`)) }},
		{name: "ValidateRMEHRStatusBytes(undecodable)", code: "invalid_shape", call: func() validation.Result {
			return validation.ValidateRMEHRStatusBytes([]byte(`{"_type": "EHR_STATUS", "is_queryable": "yes"}`))
		}},
		{name: "ValidateRM(SECTION holding itself)", code: "cyclic_value", call: func() validation.Result { return validation.ValidateRM(cyclic) }},
		{name: "ValidateRM(300 nested SECTIONs)", code: "max_depth", call: func() validation.Result { return validation.ValidateRM(deep) }},
		{name: "ValidateRM(SECTION holding itself by value)", code: "max_nodes", call: func() validation.Result {
			return validateWithin(t, "ValidateRM", visitLimitDeadline, func() validation.Result { return validation.ValidateRM(selfSharingSection()) })
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.call()
			if !containsCode(r.Issues, tc.code) {
				t.Fatalf("%s issues = %q, want one with code %s", tc.name, issueKeys(r.Issues), tc.code)
			}
			assertIdentities(t, tc.name, r, bmm)
		})
	}

	t.Run("ValidateAQL", func(t *testing.T) {
		for _, q := range []string{"", "SELEC x", "SELECT * FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1]"} {
			r := validation.ValidateAQL(aql.Query{Q: q}, c)
			if len(r.Issues) == 0 {
				t.Fatalf("ValidateAQL(%q) reported nothing; want lint issues to check", q)
			}
			for _, i := range r.Issues {
				if i.Layer != validation.LayerNone || i.Rule != "" {
					t.Errorf("ValidateAQL(%q): %s carries layer %q and rule %q, want neither (REQ-171: not an instance finding)", q, i.Code, i.Layer, i.Rule)
				}
			}
		}
	})
}

// sourceIssueLiterals is what issueLiteralProblems found in a package: one
// line per literal that does not state its identity, and the number of Issue
// literals in each file.
type sourceIssueLiterals struct {
	problems []string
	perFile  map[string]int
}

// identityExemptFuncs are the functions whose Issue literals carry neither a
// layer nor a rule whatever their code: the bridge from the AQL lint, whose
// issues are no instance findings.
var identityExemptFuncs = map[string]bool{"ValidateAQLWithTypeRelation": true}

// issueLiteralProblems parses the non-test Go files in dir and checks every
// composite literal of Issue, the elided ones in an []Issue literal included.
// A literal must name its fields and set Code. One whose Code is a guard or a
// walk condition, written as a string literal, or that sits in an exempt
// function, must set neither Layer nor Rule; every other must set Layer to
// something other than LayerNone and Rule to something other than "". It
// returns an error when dir holds no Issue literal, so the check cannot pass
// over nothing.
func issueLiteralProblems(dir string) (sourceIssueLiterals, error) {
	out := sourceIssueLiterals{perFile: map[string]int{}}
	files, err := parseNonTestFiles(dir)
	if err != nil {
		return out, err
	}
	for name, file := range files {
		for _, decl := range file.Decls {
			fn := ""
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				switch {
				case isIdent(lit.Type, "Issue"):
					out.perFile[name]++
					out.problems = append(out.problems, issueLiteralProblem(name, fn, lit)...)
				case isIssueSlice(lit.Type):
					for _, elt := range lit.Elts {
						if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
							out.perFile[name]++
							out.problems = append(out.problems, issueLiteralProblem(name, fn, inner)...)
						}
					}
				}
				return true
			})
		}
	}
	if len(out.perFile) == 0 {
		return out, fmt.Errorf("found no Issue literal in the non-test Go files of %s, so the check has gone blind", dir)
	}
	return out, nil
}

// issueLiteralProblem returns what is wrong with one Issue literal in file,
// inside function fn ("" outside any).
func issueLiteralProblem(file, fn string, lit *ast.CompositeLit) []string {
	where := file
	if fn != "" {
		where += " " + fn
	}
	fields := map[string]ast.Expr{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return []string{where + ": an Issue literal that does not name its fields"}
		}
		if key, ok := kv.Key.(*ast.Ident); ok {
			fields[key.Name] = kv.Value
		}
	}
	codeExpr, hasCode := fields["Code"]
	if !hasCode {
		return []string{where + ": an Issue literal that sets no Code, so the check cannot tell whether it needs a layer and a rule; set Code in the literal"}
	}
	code := "a computed code"
	guard := false
	if s, ok := stringLiteral(codeExpr); ok {
		code, guard = strconv.Quote(s), noIdentityCodes[s]
	}
	layer, hasLayer := fields["Layer"]
	rule, hasRule := fields["Rule"]
	if guard || identityExemptFuncs[fn] {
		if hasLayer || hasRule {
			return []string{fmt.Sprintf("%s: the Issue literal with %s sets a layer or a rule; a guard, a walk condition or an AQL issue carries neither", where, code)}
		}
		return nil
	}
	var problems []string
	if !hasLayer || isIdent(layer, "LayerNone") {
		problems = append(problems, fmt.Sprintf("%s: the Issue literal with %s sets no Layer", where, code))
	}
	if s, ok := stringLiteral(rule); !hasRule || ok && s == "" {
		problems = append(problems, fmt.Sprintf("%s: the Issue literal with %s sets no Rule", where, code))
	}
	return problems
}

// parseNonTestFiles parses the non-test Go files in dir, keyed by file name.
func parseNonTestFiles(dir string) (map[string]*ast.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files[name] = f
	}
	return files, nil
}

// isIdent reports whether e is the identifier name.
func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// isIssueSlice reports whether e is the type []Issue.
func isIssueSlice(e ast.Expr) bool {
	at, ok := e.(*ast.ArrayType)
	return ok && isIdent(at.Elt, "Issue")
}

// stringLiteral returns the value of e when e is a string literal.
func stringLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// REQ-171 Acceptance — every Issue literal in the package's source states its
// identity: a guard or a walk condition sets neither Layer nor Rule, and
// every other sets both. It is the can-fail control for the emit sites the
// corpus does not reach: removing the Layer or the Rule of any one literal
// fails here. Each file that emits instance findings is seen.
func TestREQ171_EveryIssueLiteralStatesItsIdentity(t *testing.T) {
	t.Parallel()
	got, err := issueLiteralProblems(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.problems {
		t.Errorf("%s (REQ-171 § Finding identity)", p)
	}
	for _, file := range []string{"walk_composition.go", "rmfloor.go", "rmfloor_coded.go", "rmfloor_bytes.go", "validate.go", "composition.go", "aql.go"} {
		if got.perFile[file] == 0 {
			t.Errorf("found no Issue literal in %s; the check has gone blind to it", file)
		}
	}
}

// TestREQ171_IssueLiteralCheckFailsOnEachGap runs the check over one-file
// packages, so the test above is shown to fail on each gap it exists to catch
// and to pass literals that state their identity.
func TestREQ171_IssueLiteralCheckFailsOnEachGap(t *testing.T) {
	t.Parallel()
	const header = "package p\n\n"
	tests := []struct {
		name        string
		src         string
		wantProblem string // a substring of the one problem expected, or "" for none
		wantErr     string // a substring of the error expected, or "" for none
	}{
		{
			name: "literal with a layer and a rule",
			src:  header + `func f() Issue { return Issue{Code: "required", Layer: LayerRM, Rule: ruleMandatory} }`,
		},
		{
			name:        "literal without a rule",
			src:         header + `func f() Issue { return Issue{Code: "required", Layer: LayerRM} }`,
			wantProblem: `f: the Issue literal with "required" sets no Rule`,
		},
		{
			name:        "literal without a layer",
			src:         header + `func f() Issue { return Issue{Code: "required", Rule: "mandatory"} }`,
			wantProblem: `f: the Issue literal with "required" sets no Layer`,
		},
		{
			name:        "literal whose layer is LayerNone",
			src:         header + `func f() Issue { return Issue{Code: "slot_fill", Layer: LayerNone, Rule: "slot"} }`,
			wantProblem: `sets no Layer`,
		},
		{
			name:        "literal whose rule is empty",
			src:         header + `func f() Issue { return Issue{Code: "slot_fill", Layer: LayerTemplate, Rule: ""} }`,
			wantProblem: `sets no Rule`,
		},
		{
			name:        "computed code without a rule",
			src:         header + `func f(c string) Issue { return Issue{Code: "primitive_" + c, Layer: LayerTemplate} }`,
			wantProblem: `the Issue literal with a computed code sets no Rule`,
		},
		{
			name:        "elided literal in an []Issue without a rule",
			src:         header + `func f() []Issue { return []Issue{{Code: "required", Layer: LayerRM}} }`,
			wantProblem: `sets no Rule`,
		},
		{
			name:        "literal behind a pointer without a layer",
			src:         header + `func f() *Issue { return &Issue{Code: "required", Rule: "mandatory"} }`,
			wantProblem: `sets no Layer`,
		},
		{
			name: "guard without a layer or a rule",
			src:  header + `func f() []Issue { return []Issue{{Path: "", Code: "nil_root"}} }`,
		},
		{
			name:        "guard with a layer",
			src:         header + `func f() Issue { return Issue{Code: "max_depth", Layer: LayerRM} }`,
			wantProblem: `the Issue literal with "max_depth" sets a layer or a rule`,
		},
		{
			name:        "literal whose code is set later",
			src:         header + `func f() Issue { i := Issue{Path: "/"}; i.Code = "required"; return i }`,
			wantProblem: `sets no Code`,
		},
		{
			name:        "positional literal",
			src:         header + `func f() Issue { return Issue{"/", "required"} }`,
			wantProblem: `does not name its fields`,
		},
		{
			name: "AQL bridge without a layer or a rule",
			src:  header + `func ValidateAQLWithTypeRelation(c string) Issue { return Issue{Code: c} }`,
		},
		{
			name:        "AQL bridge with a rule",
			src:         header + `func ValidateAQLWithTypeRelation(c string) Issue { return Issue{Code: c, Rule: "x"} }`,
			wantProblem: `sets a layer or a rule`,
		},
		{
			name:    "package with no Issue literal",
			src:     header + `func f() int { return 1 }`,
			wantErr: "found no Issue literal",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := issueLiteralProblems(dir)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("issueLiteralProblems(%q) error = %v, want none", tc.src, err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("issueLiteralProblems(%q) error = %v, want one naming %q", tc.src, err, tc.wantErr)
			}
			switch {
			case tc.wantProblem == "" && len(got.problems) != 0:
				t.Errorf("issueLiteralProblems(%q) = %q, want no problem", tc.src, got.problems)
			case tc.wantProblem != "" && (len(got.problems) != 1 || !strings.Contains(got.problems[0], tc.wantProblem)):
				t.Errorf("issueLiteralProblems(%q) = %q, want one problem naming %q", tc.src, got.problems, tc.wantProblem)
			}
		})
	}
}

// namedRulesInSource returns every rule the package's non-test source names
// as CLASS.Invariant: each string literal of that shape, and each row of the
// coded invariants table, whose owner and invariant make one.
func namedRulesInSource(dir string) ([]string, error) {
	files, err := parseNonTestFiles(dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if s, ok := stringLiteral(x); ok && classInvariant.MatchString(s) {
					seen[s] = true
				}
			case *ast.CompositeLit:
				if !isIdent(x.Type, "codedRule") {
					return true
				}
				var owner, invariant string
				for _, elt := range x.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					s, _ := stringLiteral(kv.Value)
					switch {
					case isIdent(kv.Key, "owner"):
						owner = s
					case isIdent(kv.Key, "invariant"):
						invariant = s
					}
				}
				seen[owner+"."+invariant] = true
			}
			return true
		})
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

// REQ-171 — every CLASS.Invariant rule the source can set, the coded
// invariants included, is an invariant of the pinned RM or BASE BMM, spelt as
// that BMM spells it, so a rule name cannot drift from the model. The rules
// the requirement names as examples are among them, so the source scan is
// not blind.
func TestREQ171_NamedRulesAreInvariantsOfThePinnedBMMs(t *testing.T) {
	t.Parallel()
	bmm := mustBMMInvariants(t)
	rules, err := namedRulesInSource(".")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the source names %d rules: %q", len(rules), rules)
	for _, r := range rules {
		if !bmm[r] {
			t.Errorf("rule %q is not an invariant of the pinned RM or BASE BMM; spell it CLASS.Invariant as the BMM does (REQ-171 § Finding identity)", r)
		}
	}
	for _, want := range []string{
		"DV_INTERVAL.Limits_consistent", "ELEMENT.Inv_null_flavour_indicated", "ENTRY.Language_valid",
		"Interval.Lower_included_valid", "TERM_MAPPING.Match_valid", "PARTY.Is_archetype_root",
		"DV_DURATION.Value_valid",
	} {
		if !slices.Contains(rules, want) {
			t.Errorf("the source names no rule %q; the scan has gone blind, or the rule was renamed (found %q)", want, rules)
		}
	}
}
