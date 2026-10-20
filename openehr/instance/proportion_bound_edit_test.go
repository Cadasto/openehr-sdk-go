package instance

// ProportionKindBound is one DV_PROPORTION bound whose numerator carries
// the C_REAL body realBody and whose type carries the C_INTEGER body
// typeBody; the denominator is left open.
func ProportionKindBound(realBody, typeBody string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_PROPORTION</rm_type_name>` +
		boundOccurrences + `<node_id></node_id>` +
		boundPrimitive("numerator", "REAL", "C_REAL", realBody) +
		boundPrimitive("type", "INTEGER", "C_INTEGER", typeBody) +
		`</children>`
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
