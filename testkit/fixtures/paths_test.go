package fixtures_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The deprecated name must keep resolving the same tree, so a caller that
// has not moved to CorpusRoot yet still finds the fixtures.
func TestCassettesRootMatchesCorpusRoot(t *testing.T) {
	if got, want := fixtures.CassettesRoot(), fixtures.CorpusRoot(); got != want {
		t.Errorf("CassettesRoot() = %q, want CorpusRoot() = %q", got, want)
	}
}

// REQ-178: the generic templates live in their package, not in the corpus, and
// still reach every whole-corpus test: ListAllOPTs lists exactly the
// operational templates of GenericTemplatesDir, each named by its stem under
// the generic label, so a dropped directory entry fails here.
func TestREQ178_ListAllOPTsListsTheGenericTemplates(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(fixtures.GenericTemplatesDir())
	if err != nil {
		t.Fatalf("ReadDir(GenericTemplatesDir()) = %v", err)
	}
	var want []fixtures.OPTRef
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".opt" {
			continue
		}
		want = append(want, fixtures.OPTRef{
			Name: "generic/" + strings.TrimSuffix(e.Name(), ".opt"),
			Path: filepath.Join(fixtures.GenericTemplatesDir(), e.Name()),
		})
	}
	slices.SortFunc(want, func(a, b fixtures.OPTRef) int { return strings.Compare(a.Name, b.Name) })
	for _, name := range []string{"generic/generic_ehr_status.v1", "generic/generic_folder.v1"} {
		if !slices.ContainsFunc(want, func(r fixtures.OPTRef) bool { return r.Name == name }) {
			t.Errorf("GenericTemplatesDir() = %s holds no %s.opt", fixtures.GenericTemplatesDir(), strings.TrimPrefix(name, "generic/"))
		}
	}

	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs() = %v", err)
	}
	var got []fixtures.OPTRef
	for _, r := range refs {
		if strings.HasPrefix(r.Name, "generic/") {
			got = append(got, r)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListAllOPTs() lists under generic/\n  %+v\nwant the operational templates of %s\n  %+v", got, fixtures.GenericTemplatesDir(), want)
	}
}
