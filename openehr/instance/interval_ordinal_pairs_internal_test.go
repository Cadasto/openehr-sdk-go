package instance

import (
	"fmt"
	"strings"
	"testing"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// ordinalPairBound is one DV_ORDINAL bound that admits exactly the given
// (value, local code) pairs.
func ordinalPairBound(pairs ...string) string {
	var b strings.Builder
	b.WriteString(`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name>`)
	b.WriteString(boundOccurrences)
	b.WriteString(`<node_id></node_id>`)
	for _, p := range pairs {
		value, code, _ := strings.Cut(p, "/")
		fmt.Fprintf(&b, `<list><value>%s</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id><code_string>%s</code_string></defining_code></symbol></list>`, value, code)
	}
	b.WriteString(`</children>`)
	return b.String()
}

// TestREQ107_OrdinalOrderKeepsEachSidesPairs is the REQ-107 check that
// ordering a DV_INTERVAL<DV_ORDINAL> never leaves a bound with a (value,
// symbol) pair its own side's C_DV_ORDINAL does not list. Swapping 2/C
// and 1/B would give the lower side 1/B, but the lower side pairs 1 with
// A, so the extremes 1/A and 2/D are used instead.
func TestREQ107_OrdinalOrderKeepsEachSidesPairs(t *testing.T) {
	opt := ReadVendoredOPT(t, CountIntervalOPT)
	node := intervalNode(t, RetargetInterval(t, opt, "DV_ORDINAL", ordinalPairBound("1/A", "2/C"), ordinalPairBound("1/B", "2/D")))
	ord := func(v int, code string) rm.DVOrdinal {
		return rm.DVOrdinal{
			Value: rm.Integer(v),
			Symbol: rm.DVCodedText{
				Value:        code,
				DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}},
			},
		}
	}
	iv := &rm.DVInterval[rm.DVOrdinal]{}
	iv.Lower, iv.Upper = ord(2, "C"), ord(1, "B")
	orderIntervalBounds(node, iv)

	lowerNode, upperNode := boundNodes(node)
	for side, b := range map[string]struct {
		node  *tcimpl.CompiledNode
		value rm.DVOrdinal
	}{"lower": {lowerNode, iv.Lower}, "upper": {upperNode, iv.Upper}} {
		if !ordinalPairListed(b.node, b.value) {
			t.Errorf("%s bound %s is not a pair its side lists", side, ordinalParts(b.value))
		}
	}
	if iv.Lower.Value > iv.Upper.Value {
		t.Errorf("bounds %s and %s are out of order", ordinalParts(iv.Lower), ordinalParts(iv.Upper))
	}
	if got, want := [2][2]string{ordinalParts(iv.Lower), ordinalParts(iv.Upper)}, [2][2]string{{"1", "A"}, {"2", "D"}}; got != want {
		t.Errorf("orderIntervalBounds = %q, want %q", got, want)
	}
}

// ordinalPairListed reports whether node's C_DV_ORDINAL lists o's value
// with o's symbol code.
func ordinalPairListed(node *tcimpl.CompiledNode, o rm.DVOrdinal) bool {
	c, ok := node.PrimitiveConstraint().(constraints.CDvOrdinal)
	if !ok {
		return false
	}
	for _, pair := range c.Values {
		if pair.Value == int(o.Value) && pair.Symbol.CodeString == o.Symbol.DefiningCode.CodeString {
			return true
		}
	}
	return false
}
