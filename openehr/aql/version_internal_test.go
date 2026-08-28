package aql

// version_internal_test.go reaches the unexported containment carrier to pin
// the one REQ-163 refusal the public API cannot put a query into: a version
// predicate on a class node that is not spelled VERSION. [Version] fixes the RM
// type to the SDK's own spelling, so the state is unreachable from outside —
// the guard exists so a future constructor, or a widened field, cannot regress
// it silently. The public surface is covered from aql_test (version_test.go).
// REQ-163 · PROBE-088

import (
	"errors"
	goast "go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestVersionPredicateRefusedOffVersionNode pins the REQ-163 rule that
// `versionPredicate` is reachable from classExprOperand's VERSION alternative
// alone: the bracket has no position on any other class expression, so a node
// carrying one MUST be refused rather than emitted as text the parser rejects.
//
// The refusal names the constructor that DOES carry the shape, so the caller is
// pointed at a route rather than left with a grammar citation.
func TestVersionPredicateRefusedOffVersionNode(t *testing.T) {
	for _, rmType := range []string{"COMPOSITION", "OBSERVATION", "VERSIONED_COMPOSITION"} {
		t.Run(rmType, func(t *testing.T) {
			c := Containment{rmType: rmType, alias: "x", versionPred: LatestVersion()}
			err := c.validateTree(map[string]bool{})
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("validateTree = %v, want ErrInvalidQuery", err)
			}
			if !strings.Contains(err.Error(), "aql.Version") {
				t.Errorf("refusal does not name the constructor that carries the shape: %v", err)
			}
		})
	}
}

// TestVersionPredicateSpellingFoldIsASCII pins the FOLD the VERSION-spelling
// test uses, which is not interchangeable with the archetype refusal's beside
// it because the two have opposite polarity.
//
// This refusal fires when the node is NOT VERSION, so a WIDER fold accepts
// more: under [strings.EqualFold], `VERſION` (U+017F, which folds to `s`) would
// count as the keyword and carry a bracket to the wire — a spelling the lexer,
// whose keyword fragments are ASCII, cannot read. [asciiKeyword] is what keeps
// the accept set to the spellings the lexer actually tokenises, in both
// directions: every ASCII casing is accepted, the Unicode fold-equal is not.
func TestVersionPredicateSpellingFoldIsASCII(t *testing.T) {
	accepted := []string{"VERSION", "version", "Version"}
	for _, rmType := range accepted {
		t.Run("accepts "+rmType, func(t *testing.T) {
			c := Containment{rmType: rmType, alias: "v", versionPred: LatestVersion()}
			if err := c.validateVersionPredicate(); err != nil {
				t.Fatalf("validateVersionPredicate(%q) = %v, want nil", rmType, err)
			}
		})
	}
	// The Unicode fold-equal spelling, refused here as well as by
	// [validateRMTypeToken] one check earlier — the redundancy is the point.
	t.Run("refuses the Unicode fold-equal spelling", func(t *testing.T) {
		c := Containment{rmType: "VERſION", alias: "v", versionPred: LatestVersion()}
		if err := c.validateVersionPredicate(); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("validateVersionPredicate = %v, want ErrInvalidQuery", err)
		}
	})
}

// TestVersionPredicateVocabularyIsThreeShapes pins the sealed sum's size: the
// grammar's `versionPredicate` is a fixed three-way choice that does not
// recurse into its own position, so a fourth shape would be a grammar change.
//
// Two claims, and the SECOND is what an emission table cannot make:
//
//   - the three constructors return three DISTINCT shapes rather than aliases
//     of one, which no golden could tell apart from a single correctly
//     rendering shape;
//   - the package declares NO OTHER shape. That half is derived from the
//     sources, not written down twice: a fourth type declaring versionBracket
//     fails here by name. The dispatch tripwire
//     (parse/dispatch_tripwire_test.go) derives the same set but holds it to a
//     FLOOR — deliberately, so a widened vocabulary does not fail an unrelated
//     sweep — so the exact-count claim has to live here, in the file that owns
//     the vocabulary.
func TestVersionPredicateVocabularyIsThreeShapes(t *testing.T) {
	shapes := map[string]VersionPredicate{
		"latest":     LatestVersion(),
		"all":        AllVersions(),
		"comparison": VersionCompare("a/b", OpEq, Int(1)),
	}
	seen := map[string]string{}
	for name, p := range shapes {
		key := typeKey(p)
		if key == "unknown" {
			t.Errorf("the %s constructor returns a shape [typeKey] does not know", name)
			continue
		}
		if other, dup := seen[key]; dup {
			t.Errorf("%s and %s are the same shape (%s); the three alternatives must stay distinct",
				name, other, key)
		}
		seen[key] = name
	}

	// The derived half. Every receiver of the versionBracket marker method in
	// this package's non-test sources IS a vocabulary shape, so the set below
	// is the whole vocabulary — including one a new file added without
	// touching this test.
	declared := declaredVersionPredicateShapes(t)
	want := []string{"allVersions", "latestVersion", "versionComparison"}
	if !slices.Equal(declared, want) {
		t.Fatalf("the versionPredicate vocabulary is %v, want exactly %v — `versionPredicate` is a "+
			"fixed three-way choice, so a fourth shape is a grammar change and needs the grammar "+
			"profile, [derefVersionPredicate] and [typeKey] extended with it", declared, want)
	}
	// …and the constructors reach all of it: a shape nothing constructs is
	// dead vocabulary, and one constructed twice is the aliasing above.
	if got := slices.Sorted(maps.Keys(seen)); !slices.Equal(got, want) {
		t.Fatalf("the constructors cover %v of the declared vocabulary %v", got, want)
	}
}

// declaredVersionPredicateShapes returns, sorted, the names of every type in
// this package's non-test sources that declares the `versionBracket` marker
// method — i.e. the whole [VersionPredicate] vocabulary, derived rather than
// restated.
//
// It parses the sources instead of using reflection, which the idiom spec bans,
// and it is the same derivation the dispatch tripwire runs — repeated here
// because the two ask different questions of it (that one a floor, this one an
// exact set).
func declaredVersionPredicateShapes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var shapes []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*goast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 || fd.Name.Name != "versionBracket" {
				continue
			}
			rt := fd.Recv.List[0].Type
			if star, ok := rt.(*goast.StarExpr); ok {
				rt = star.X
			}
			if id, ok := rt.(*goast.Ident); ok {
				shapes = append(shapes, id.Name)
			}
		}
	}
	slices.Sort(shapes)
	return slices.Compact(shapes)
}

// unlearnedVersionPredicate is a shape OUTSIDE the sealed catalogue, standing in
// for the value a caller forms by embedding the exported interface
// (`struct{ aql.VersionPredicate }`). Its methods panic rather than returning a
// zero value, so a dispatch site that reaches them is caught here loudly instead
// of quietly rendering "".
type unlearnedVersionPredicate struct{}

func (unlearnedVersionPredicate) versionBracket() string {
	panic("versionBracket called on an out-of-catalogue version predicate")
}

func (unlearnedVersionPredicate) versionValidate() error {
	panic("versionValidate called on an out-of-catalogue version predicate")
}

// TestOutOfCatalogueVersionPredicateIsRefused pins the catalogue gate at the
// VALIDATION dispatch site. [VersionPredicate] is exported and sealed by
// unexported methods, which blocks a foreign type IMPLEMENTING it and not
// EMBEDDING it, so an out-of-catalogue value is caller-constructible and MUST
// be refused rather than dereferenced (REQ-025 § No panics).
//
// Delete the [derefVersionPredicate] call in [Containment.validateVersionPredicate]
// and this test fails by panic.
func TestOutOfCatalogueVersionPredicateIsRefused(t *testing.T) {
	c := Containment{rmType: "VERSION", alias: "v", versionPred: unlearnedVersionPredicate{}}
	err := c.validateTree(map[string]bool{})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("validateTree = %v, want ErrInvalidQuery", err)
	}
	if !strings.Contains(err.Error(), "aql.LatestVersion") {
		t.Errorf("refusal does not name the constructors that carry the shape: %v", err)
	}
}

// TestOutOfCatalogueVersionPredicateDoesNotPanicOnRender pins the SECOND
// dispatch site, [Containment.classToken] — the one a guard on the validation
// path alone leaves open. The two sit in different functions, so guarding one
// MOVES the panic rather than removing it.
//
// Asked of classToken directly, because [Containment.validateTree] refuses the
// node before [Builder.Build] ever renders it: through the public surface this
// site is unreachable, which is exactly why it needs its own row rather than
// being left to the Build-level test.
//
// Delete the [derefVersionPredicate] call in classToken and this test fails by
// panic.
func TestOutOfCatalogueVersionPredicateDoesNotPanicOnRender(t *testing.T) {
	c := Containment{rmType: "VERSION", alias: "v", versionPred: unlearnedVersionPredicate{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("classToken panicked on an out-of-catalogue predicate: %v", r)
		}
	}()
	// No bracket: the shape has no canonical text, and validateTree has already
	// refused the node, so there is nothing to render and nothing reaches the
	// wire.
	if got, want := c.classToken(), "VERSION v"; got != want {
		t.Fatalf("classToken() = %q, want %q", got, want)
	}
}

// typeKey renders a predicate's concrete type without reflection (the idiom
// spec bans it): each shape's bracket text is grammar-distinct, and the two
// keyword shapes are singletons, so the rendered text identifies the shape.
func typeKey(p VersionPredicate) string {
	switch p.(type) {
	case latestVersion:
		return "latestVersion"
	case allVersions:
		return "allVersions"
	case versionComparison:
		return "versionComparison"
	default:
		return "unknown"
	}
}
