package rm_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// intervalFlags are the four boundary flags BASE Interval declares, which the
// decoder gives a value when the document leaves them out (REQ-169), so no
// decode reports one missing.
var intervalFlags = []string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}

// TestREQ175_ClassDescriptorsMatchRMTables is the census that pins the
// generator to the RM tables (REQ-175, PROBE-110): for every RM class the
// type registry constructs, the description its canonical-XML decoder hands
// the runtime names, as its mandatory members, exactly what rminfo makes
// mandatory on the class minus the attributes no serialization carries and
// the four interval boundary flags, in rminfo's order. An empty root element
// decoded through a tracker that reports missing members must then report
// `/` + name for each, in that order, and nothing else. The two tables are
// generated from one BMM by two renderers; this holds them to one answer.
func TestREQ175_ClassDescriptorsMatchRMTables(t *testing.T) {
	checked := 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		ctor, ok := typereg.Default.Lookup(class)
		if !ok {
			continue
		}
		checked++
		want := slices.DeleteFunc(rminfo.Default.RequiredAttributes(class), func(attr string) bool {
			return rminfo.IsNonStorableAttr(class, attr) || slices.Contains(intervalFlags, attr)
		})
		for i, attr := range want {
			want[i] = "/" + attr
		}
		t.Run(class, func(t *testing.T) {
			var got []string
			tr := xmlunmatched.NewTracker(func(p string) { t.Errorf("unmatched %q reported for an empty element", p) }, false)
			tr.ReportMissing(func(p string) { got = append(got, p) })
			doc := "<" + class + ` xmlns="http://schemas.openehr.org/v1"/>`
			if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), ctor()); err != nil {
				t.Fatalf("Decode(%s) = %v; want nil", doc, err)
			}
			if !slices.Equal(got, want) {
				t.Errorf("the empty %s reports missing members %q; want rminfo's mandatory attributes %q", class, got, want)
			}
		})
	}
	if checked < 100 {
		t.Errorf("checked %d classes; want every class the type registry constructs, over 100", checked)
	}
}
