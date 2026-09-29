package instance

// interval_opt_edit_test.go edits the bound constraints of the two vendored
// interval OPTs whose DV_INTERVAL constrains both bounds. The helpers are
// exported so the external interval tests (package instance_test) build
// the same OPTs as the in-package ones; a _test.go file never reaches the
// shipped package.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The vendored OPTs whose one ELEMENT value is a DV_INTERVAL with both
// bounds constrained: to 0..100, and to 0.0..100.0 Cel.
const (
	CountIntervalOPT    = "Test_dv_interval_dv_count_lower_upper_constraint.v0"
	QuantityIntervalOPT = "Test_dv_interval_dv_quantity_lower_upper_constraint.v0"
)

// ReadVendoredOPT returns the text of the vendored OPT called name.
func ReadVendoredOPT(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", name, err)
	}
	return string(raw)
}

// End is one end of a numeric range in an edited constraint.
type End struct {
	Value     float64
	Included  bool
	Unbounded bool
}

// Closed returns an end that includes v.
func Closed(v float64) End { return End{Value: v, Included: true} }

// Open returns an end that excludes v.
func Open(v float64) End { return End{Value: v} }

// NoEnd is the end of a range unbounded in that direction.
var NoEnd = End{Unbounded: true}

// IntRange returns a C_INTEGER body that admits the range from lo to hi.
func IntRange(lo, hi End) string { return "<range>" + rangeBody(lo, hi) + "</range>" }

// IntList returns a C_INTEGER body that admits exactly values.
func IntList(values ...int) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "<list>%d</list>", v)
	}
	return b.String()
}

// QuantityEntry returns a C_DV_QUANTITY list entry that admits units with
// a magnitude from lo to hi.
func QuantityEntry(units string, lo, hi End) string {
	return "<list><magnitude>" + rangeBody(lo, hi) + "</magnitude><units>" + units + "</units></list>"
}

func rangeBody(lo, hi End) string {
	body := fmt.Sprintf("<lower_included>%t</lower_included><upper_included>%t</upper_included>"+
		"<lower_unbounded>%t</lower_unbounded><upper_unbounded>%t</upper_unbounded>",
		lo.Included, hi.Included, lo.Unbounded, hi.Unbounded)
	if !lo.Unbounded {
		body += fmt.Sprintf("<lower>%v</lower>", lo.Value)
	}
	if !hi.Unbounded {
		body += fmt.Sprintf("<upper>%v</upper>", hi.Value)
	}
	return body
}

var (
	intervalTypeRE = regexp.MustCompile(`<rm_type_name>DV_INTERVAL&lt;[A-Z_]+&gt;</rm_type_name>`)
	countItemRE    = regexp.MustCompile(`(?s)<item xsi:type="C_INTEGER">.*?</item>`)
	quantityListRE = regexp.MustCompile(`(?s)<list>\s*<magnitude>.*?</list>`)
)

// EditCountBounds replaces the C_INTEGER body on each bound's magnitude in
// the CountIntervalOPT text, lower first.
func EditCountBounds(t *testing.T, opt, lowerBody, upperBody string) string {
	t.Helper()
	item := func(body string) string { return `<item xsi:type="C_INTEGER">` + body + `</item>` }
	return replaceUnderInterval(t, opt, countItemRE, item(lowerBody), item(upperBody))
}

// EditQuantityBounds replaces the list entries of each bound's
// C_DV_QUANTITY in the QuantityIntervalOPT text, lower first.
func EditQuantityBounds(t *testing.T, opt, lowerEntries, upperEntries string) string {
	t.Helper()
	return replaceUnderInterval(t, opt, quantityListRE, lowerEntries, upperEntries)
}

// replaceUnderInterval replaces the two matches of re that follow the OPT's
// DV_INTERVAL node, the lower bound's and then the upper bound's.
func replaceUnderInterval(t *testing.T, opt string, re *regexp.Regexp, lower, upper string) string {
	t.Helper()
	node := intervalTypeRE.FindStringIndex(opt)
	if node == nil {
		t.Fatal("OPT has no DV_INTERVAL node")
	}
	head, tail := opt[:node[1]], opt[node[1]:]
	m := re.FindAllStringIndex(tail, -1)
	if len(m) != 2 {
		t.Fatalf("found %d matches of %s under the interval, want 2", len(m), re)
	}
	return head + tail[:m[0][0]] + lower + tail[m[0][1]:m[1][0]] + upper + tail[m[1][1]:]
}
