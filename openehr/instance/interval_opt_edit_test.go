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

const (
	boundOccurrences = `<occurrences><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>`
	boundExistence   = `<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>`
)

// RetargetInterval rewrites the CountIntervalOPT interval as
// DV_INTERVAL<param> and replaces each bound's child element, lower first.
func RetargetInterval(t *testing.T, opt, param, lowerChild, upperChild string) string {
	t.Helper()
	opt = replaceBoundChildren(t, opt, lowerChild, upperChild)
	if !intervalTypeRE.MatchString(opt) {
		t.Fatal("OPT has no DV_INTERVAL node")
	}
	return intervalTypeRE.ReplaceAllString(opt, `<rm_type_name>DV_INTERVAL&lt;`+param+`&gt;</rm_type_name>`)
}

// TemporalBound is one DV_DATE, DV_TIME or DV_DATE_TIME bound whose value
// carries the given C_DATE, C_TIME or C_DATE_TIME item body.
func TemporalBound(rmType, primitiveType, itemType, itemBody string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		boundOccurrences + `<node_id></node_id><attributes xsi:type="C_SINGLE_ATTRIBUTE">` +
		`<rm_attribute_name>value</rm_attribute_name>` + boundExistence +
		`<match_negated>false</match_negated><children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>` +
		primitiveType + `</rm_type_name>` + boundOccurrences + `<node_id></node_id><item xsi:type="` +
		itemType + `">` + itemBody + `</item></children></attributes></children>`
}

// OrdinalBound is one DV_ORDINAL bound that admits exactly values, each
// with a distinct local code.
func OrdinalBound(values ...int) string {
	var b strings.Builder
	b.WriteString(`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name>`)
	b.WriteString(boundOccurrences)
	b.WriteString(`<node_id></node_id>`)
	for i, v := range values {
		fmt.Fprintf(&b, `<list><value>%d</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id><code_string>at%04d</code_string></defining_code></symbol></list>`, v, i+1)
	}
	b.WriteString(`</children>`)
	return b.String()
}

// RealRange returns a C_REAL body that admits the range from lo to hi.
func RealRange(lo, hi End) string { return "<range>" + rangeBody(lo, hi) + "</range>" }

// RealList returns a C_REAL body that admits exactly values.
func RealList(values ...float64) string {
	var b strings.Builder
	for _, v := range values {
		fmt.Fprintf(&b, "<list>%v</list>", v)
	}
	return b.String()
}

// ProportionBound is one DV_PROPORTION bound. realBody is the C_REAL
// constraint on numerator; the denominator and type are left open.
func ProportionBound(realBody string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_PROPORTION</rm_type_name>` +
		boundOccurrences + `<node_id></node_id><attributes xsi:type="C_SINGLE_ATTRIBUTE">` +
		`<rm_attribute_name>numerator</rm_attribute_name>` + boundExistence +
		`<match_negated>false</match_negated><children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>REAL</rm_type_name>` +
		boundOccurrences + `<node_id></node_id><item xsi:type="C_REAL">` + realBody +
		`</item></children></attributes></children>`
}

// replaceBoundChildren replaces the child element of the interval's lower
// attribute, then of its upper attribute.
func replaceBoundChildren(t *testing.T, opt, lowerXML, upperXML string) string {
	t.Helper()
	node := intervalTypeRE.FindStringIndex(opt)
	if node == nil {
		t.Fatal("OPT has no DV_INTERVAL node")
	}
	lowerAttr := strings.Index(opt[node[1]:], "<rm_attribute_name>lower</rm_attribute_name>")
	upperAttr := strings.Index(opt[node[1]:], "<rm_attribute_name>upper</rm_attribute_name>")
	if lowerAttr < 0 || upperAttr < 0 || upperAttr < lowerAttr {
		t.Fatal("OPT interval has no lower attribute followed by an upper attribute")
	}
	lowerAt := node[1] + lowerAttr
	upperAt := node[1] + upperAttr
	lowerChild := strings.Index(opt[lowerAt:upperAt], "<children")
	upperChild := strings.Index(opt[upperAt:], "<children")
	if lowerChild < 0 || upperChild < 0 {
		t.Fatal("interval bound has no child element")
	}
	ls, le := tagSpan(opt, lowerAt+lowerChild, "<children", "</children>")
	us, ue := tagSpan(opt, upperAt+upperChild, "<children", "</children>")
	if ls < 0 || us < 0 {
		t.Fatal("interval bound child element is not closed")
	}
	opt = opt[:us] + upperXML + opt[ue:]
	return opt[:ls] + lowerXML + opt[le:]
}

// tagSpan returns the span of the element that starts at start, matching
// nested copies of the same open and close tags.
func tagSpan(s string, start int, open, close string) (int, int) {
	if start < 0 || start >= len(s) || !strings.HasPrefix(s[start:], open) {
		return -1, -1
	}
	depth := 0
	i := start
	for i < len(s) {
		relOpen := strings.Index(s[i:], open)
		relClose := strings.Index(s[i:], close)
		if relClose < 0 {
			return -1, -1
		}
		if relOpen >= 0 && relOpen < relClose {
			depth++
			i += relOpen + len(open)
			continue
		}
		depth--
		i += relClose + len(close)
		if depth == 0 {
			return start, i
		}
	}
	return -1, -1
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
