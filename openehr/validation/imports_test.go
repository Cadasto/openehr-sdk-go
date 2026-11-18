package validation_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// aqlParser lists what the closure of openehr/validation must not reach, so
// that a caller that validates instances links no AQL parser (REQ-013,
// docs/specifications/module-layout.md § REQ-013, the openehr/validation row):
// openehr/aql and every package under it, the ANTLR runtime the generated AQL
// parser imports, and golang.org/x/exp, which that runtime imports. Scan checks
// an import from another module where a package of this module makes it, so
// golang.org/x/exp is caught when this module imports it; today it would arrive
// only through the ANTLR runtime.
func aqlParser() []string {
	return []string{
		"github.com/cadasto/openehr-sdk-go/openehr/aql",
		"github.com/antlr4-go/antlr/v4",
		"golang.org/x/exp",
	}
}

// TestValidationForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/validation.
// The validator works on in-memory RM graphs: decoding wire bytes, network
// calls and authentication belong to its callers. These rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client. The generated attribute access in
//     internal/rmaccess is one of those packages, so the closure scan covers
//     it too.
//   - Nothing in that closure is openehr/aql or a package under it, and
//     nothing imports the ANTLR runtime or golang.org/x/exp, so a caller that
//     validates instances links no AQL parser. The AQL lint reaches the
//     validation model through openehr/validation/aqlvalidation instead.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in, since openehr/rm's generated
//     marshal files import openehr/serialize/canxml.
//   - The package's own non-test files import nothing that reads a file,
//     embeds one, loads a plugin or reads a BMM: the RM rules are compiled in
//     and the invariant inventory is generated, so no rule is loaded or
//     interpreted at run time (§ REQ-171 Where the RM rules come from).
//
// Test files may import anything, for fixture decoding for instance.
func TestValidationForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{rule: "REQ-013: a caller that validates instances links no AQL parser", forbidden: aqlParser()},
	}
	for _, r := range closure {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/validation MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}
	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/validation MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
		if p, ok := importguard.Matches(imp, importguard.RuleLoaders()); ok {
			t.Errorf("openehr/validation MUST NOT import %q in its own files (forbidden entry %q; REQ-171: the validators load no rule from outside the binary and interpret no BMM expression at run time)", imp, p)
		}
	}
}

// TestValidationAQLParserRuleCatchesTheBridge is the can-fail control for the
// REQ-013 AQL-parser closure rule above. The bridge package
// openehr/validation/aqlvalidation imports openehr/aql/lint, the import that
// used to sit in openehr/validation itself, so the same forbidden list scanned
// from there must report it. If the list or the scan stopped catching that
// import, the rule above would pass over nothing.
func TestValidationAQLParserRuleCatchesTheBridge(t *testing.T) {
	t.Parallel()
	const lint = "github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	violations, err := importguard.Scan("./aqlvalidation", aqlParser())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(violations, func(v importguard.Violation) bool { return v.Import == lint }) {
		t.Errorf("importguard.Scan(./aqlvalidation, aqlParser()) = %+v, want a violation for %q (REQ-013)", violations, lint)
	}
}
