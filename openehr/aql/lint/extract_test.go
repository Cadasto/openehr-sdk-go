package lint_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

const query = "SELECT o/data[at0001]/events[at0006]/value/magnitude AS sys " +
	"FROM EHR e[ehr_id/value=$ehr] " +
	"CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1] " +
	"CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
	"WHERE o/data[at0001]/events[at0006]/value/magnitude > $threshold"

func mustParse(t *testing.T, q string) *parse.Document {
	t.Helper()
	doc, err := parse.Parse(q)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	return doc
}

func TestExtractMetadata(t *testing.T) {
	md := lint.Extract(mustParse(t, query))

	wantArch := []string{
		"openEHR-EHR-COMPOSITION.encounter.v1",
		"openEHR-EHR-OBSERVATION.blood_pressure.v1",
	}
	if len(md.Archetypes) != len(wantArch) {
		t.Fatalf("Archetypes = %v, want %v", md.Archetypes, wantArch)
	}
	for i, w := range wantArch {
		if md.Archetypes[i] != w {
			t.Errorf("Archetypes[%d] = %q, want %q", i, md.Archetypes[i], w)
		}
	}

	for _, alias := range []string{"e", "c", "o"} {
		if _, ok := md.Aliases[alias]; !ok {
			t.Errorf("alias %q not in alias map %v", alias, md.Aliases)
		}
	}
	if md.Aliases["o"].RMType != "OBSERVATION" {
		t.Errorf("alias o RMType = %q, want OBSERVATION", md.Aliases["o"].RMType)
	}

	if len(md.Params) != 2 || md.Params[0] != "ehr" || md.Params[1] != "threshold" {
		t.Errorf("Params = %v, want [ehr threshold]", md.Params)
	}
	if len(md.Paths) != 2 {
		t.Errorf("Paths = %d, want 2", len(md.Paths))
	}
}

// TestExtractDoesNotAliasDocumentSlices ensures mutating Metadata paths does
// not corrupt the parsed document.
func TestExtractDoesNotAliasDocumentSlices(t *testing.T) {
	doc := mustParse(t, query)
	md := lint.Extract(doc)
	md.Paths = append(md.Paths, parse.IdentifiedPath{IdentifiedPath: aql.IdentifiedPath{Alias: "z"}})
	if len(doc.Paths) == len(md.Paths) {
		t.Fatal("document paths aliased metadata paths slice")
	}
}

func TestNormalise(t *testing.T) {
	doc := mustParse(t, query)
	// First path is the SELECT projection rooted at alias o.
	p, err := lint.Normalise(doc.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if p.Alias != "o" {
		t.Errorf("Alias = %q, want o", p.Alias)
	}
	want := "/data[at0001]/events[at0006]/value/magnitude"
	if p.Suffix != want {
		t.Errorf("Suffix = %q, want %q", p.Suffix, want)
	}
	if len(p.Segments) != 4 {
		t.Errorf("Segments = %d, want 4", len(p.Segments))
	}
}

// TestNormaliseBareAlias covers a projection that is just an alias root
// (`SELECT o`) — no segments, empty suffix, no error.
func TestNormaliseBareAlias(t *testing.T) {
	doc := mustParse(t, "SELECT o FROM OBSERVATION o")
	p, err := lint.Normalise(doc.Paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if p.Alias != "o" || p.Suffix != "" || len(p.Segments) != 0 {
		t.Errorf("bare alias path = %+v", p)
	}
}

func TestNormaliseEmptyAlias(t *testing.T) {
	_, err := lint.Normalise(parse.IdentifiedPath{})
	if !errors.Is(err, lint.ErrEmptyPath) {
		t.Errorf("err = %v, want ErrEmptyPath", err)
	}
}
