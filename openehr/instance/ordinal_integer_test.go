package instance_test

import "testing"

// TestREQ107_REQ056_IntegerOnTheOrdinalItselfPastTheIntegerRangeIsRefusedAtRead is
// the REQ-107 and REQ-056 check that a C_INTEGER an OPT puts on a
// DV_ORDINAL itself, a primitive leaf in place of a C_DV_ORDINAL, never
// reaches the generator with a list member, a range end or an assumed value
// past the 32-bit range of an RM Integer: reading the template is refused
// with an error wrapping canxml.ErrInvalidShape, since the model types each
// of them as an Integer. The cases one past either end of the range are
// refused and the ends themselves are read, so the refusal is the range
// and not the shape of the template.
func TestREQ107_REQ056_IntegerOnTheOrdinalItselfPastTheIntegerRangeIsRefusedAtRead(t *testing.T) {
	cases := []struct {
		name string
		body string // the C_INTEGER body
		// refused is set where reading the template is to fail.
		refused bool
	}{
		{name: "at most 5000000000", body: intRangeBody("", "5000000000", true), refused: true},
		{name: "at least -5000000000", body: intRangeBody("-5000000000", "", true), refused: true},
		{name: "listing 5000000000 alone", body: "<list>5000000000</list>", refused: true},
		{name: "assuming 5000000000", body: "<list>1</list><assumed_value>5000000000</assumed_value>", refused: true},
		{name: "at most one past the greatest Integer", body: intRangeBody("", "2147483648", true), refused: true},
		{name: "at least one below the least Integer", body: intRangeBody("-2147483649", "", true), refused: true},
		{name: "listing one past the greatest Integer", body: "<list>2147483648</list>", refused: true},
		{name: "at most the greatest Integer", body: intRangeBody("", "2147483647", true)},
		{name: "at least the least Integer", body: intRangeBody("-2147483648", "", true)},
		{name: "listing the least Integer", body: "<list>-2147483648</list>"},
		{name: "assuming the greatest Integer", body: "<list>1</list><assumed_value>2147483647</assumed_value>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xml := optTemplate("ELEMENT", optSingle("value", optPrimitive("DV_ORDINAL", "C_INTEGER", tc.body)))
			if tc.refused {
				requireReadRefused(t, xml)
				return
			}
			compileOPTText(t, xml, true)
		})
	}
}
