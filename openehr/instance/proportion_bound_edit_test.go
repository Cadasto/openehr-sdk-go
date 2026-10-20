package instance

import "fmt"

// ProportionKindBound is one DV_PROPORTION bound whose numerator carries
// the C_REAL body realBody and whose type carries the C_INTEGER body
// typeBody, followed by precisionAttr, a precision attribute from
// RequiredPrecision, OptionalPrecision or ProhibitedPrecision, or "" to
// leave the precision open. The denominator is left open.
func ProportionKindBound(realBody, typeBody, precisionAttr string) string {
	return ProportionKindBoundOver(realBody, "", typeBody, precisionAttr)
}

// ProportionKindBoundOver is ProportionKindBound with a denominator that
// carries the C_REAL body denominatorBody, or is left open where that is
// "".
func ProportionKindBoundOver(realBody, denominatorBody, typeBody, precisionAttr string) string {
	denominator := ""
	if denominatorBody != "" {
		denominator = boundPrimitive("denominator", "REAL", "C_REAL", denominatorBody)
	}
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_PROPORTION</rm_type_name>` +
		boundOccurrences + `<node_id></node_id>` +
		boundPrimitive("numerator", "REAL", "C_REAL", realBody) +
		denominator +
		boundPrimitive("type", "INTEGER", "C_INTEGER", typeBody) +
		precisionAttr +
		`</children>`
}

// RequiredPrecision is a precision attribute of existence 1..1 whose
// C_INTEGER carries body.
func RequiredPrecision(body string) string {
	return boundPrimitive("precision", "INTEGER", "C_INTEGER", body)
}

// OptionalPrecision is a precision attribute of existence 0..1 whose
// C_INTEGER carries body.
func OptionalPrecision(body string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>precision</rm_attribute_name>` +
		existence(0, 1) + `<match_negated>false</match_negated><children xsi:type="C_PRIMITIVE_OBJECT">` +
		`<rm_type_name>INTEGER</rm_type_name>` + boundOccurrences + `<node_id></node_id>` +
		`<item xsi:type="C_INTEGER">` + body + `</item></children></attributes>`
}

// ProhibitedPrecision is a precision attribute of existence 0..0.
func ProhibitedPrecision() string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>precision</rm_attribute_name>` +
		existence(0, 0) + `<match_negated>false</match_negated></attributes>`
}

func existence(lower, upper int) string {
	return fmt.Sprintf(`<existence><lower_included>true</lower_included><upper_included>true</upper_included>`+
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`+
		`<lower>%d</lower><upper>%d</upper></existence>`, lower, upper)
}

// boundPrimitive is a required single attribute called name whose one
// child is a C_PRIMITIVE_OBJECT of the AOM primitive rmType with an
// itemType item carrying body.
func boundPrimitive(name, rmType, itemType, body string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		boundExistence + `<match_negated>false</match_negated><children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>` +
		rmType + `</rm_type_name>` + boundOccurrences + `<node_id></node_id><item xsi:type="` + itemType + `">` +
		body + `</item></children></attributes>`
}
