package lint

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// TestBindingsAgreeWithExtract holds the alias lookup Lint uses to the binding
// Extract publishes in Metadata.Aliases: the later of two class expressions
// carrying one alias binds it, an anonymous one binds nothing, and the answer
// is the same below and above the class count at which the lookup indexes.
func TestBindingsAgreeWithExtract(t *testing.T) {
	t.Parallel()
	long := make([]string, bindingIndexMin+2)
	for i := range long {
		long[i] = fmt.Sprintf("CLUSTER c%d", i)
	}
	cases := []struct {
		name  string
		query string
		ask   []string
	}{
		{
			name:  "short list, an alias bound twice",
			query: "SELECT o FROM EHR e CONTAINS COMPOSITION CONTAINS (OBSERVATION o AND EVALUATION o)",
			ask:   []string{"e", "o", "x", ""},
		},
		{
			name:  "indexed list, an alias bound twice",
			query: "SELECT c1 FROM COMPOSITION c1 CONTAINS (" + strings.Join(long, " AND ") + " AND SECTION c3)",
			ask:   []string{"c1", "c3", fmt.Sprintf("c%d", bindingIndexMin+1), "x", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parse.Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			if long := len(doc.Classes) >= bindingIndexMin; long != strings.HasPrefix(tc.name, "indexed") {
				t.Fatalf("premise gone: %d class expressions, indexed = %v", len(doc.Classes), long)
			}
			want := Extract(doc).Aliases
			b := newBindings(doc.Classes)
			for _, alias := range tc.ask {
				got, ok := b.lookup(alias)
				wantCE, wantOK := want[alias]
				if ok != wantOK || got.RMType != wantCE.RMType || got.Pos != wantCE.Pos {
					t.Errorf("lookup(%q) = %s at %v, %v; Extract binds %s at %v, %v",
						alias, got.RMType, got.Pos, ok, wantCE.RMType, wantCE.Pos, wantOK)
				}
			}
		})
	}
}
