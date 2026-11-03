package xmlschema

// REQ-176, REQ-052: canonical JSON names a typed item's BMM primitive, and
// its decode names one XML Schema built-in datatype per primitive, which must
// read back as that primitive; the forward and reverse tables cannot drift.

import "testing"

// TestREQ176_REQ052_CanonicalBuiltinsReadBack pins the reverse table over every
// canonical built-in: each BMM primitive an XML Schema built-in reads as has
// the one built-in below, which Builtin reads as that primitive again, and
// every primitive Builtin returns has one.
func TestREQ176_REQ052_CanonicalBuiltinsReadBack(t *testing.T) {
	want := map[string]string{
		"String":            "string",
		"Uri":               "anyURI",
		"Boolean":           "boolean",
		"Integer":           "int",
		"Integer64":         "long",
		"Real":              "float",
		"Double":            "double",
		"Iso8601_date":      "date",
		"Iso8601_time":      "time",
		"Iso8601_date_time": "dateTime",
		"Iso8601_duration":  "duration",
	}
	for primitive, builtin := range want {
		got, ok := canonicalBuiltins[primitive]
		if !ok || got != builtin {
			t.Errorf("canonicalBuiltins[%q] = %q, %t; want %q", primitive, got, ok, builtin)
			continue
		}
		if back, ok := Builtin(got); !ok || back != primitive {
			t.Errorf("Builtin(%q) = %q, %t; want %q, the primitive it is canonical for", got, back, ok, primitive)
		}
	}
	for primitive := range canonicalBuiltins {
		if _, ok := want[primitive]; !ok {
			t.Errorf("canonicalBuiltins holds %q, which no XML Schema built-in reads as here", primitive)
		}
	}
	for local, primitive := range builtins {
		if _, ok := canonicalBuiltins[primitive]; !ok {
			t.Errorf("Builtin(%q) reads as %q, which has no canonical built-in", local, primitive)
		}
	}
}

// TestREQ176_REQ056_EveryBuiltinReadsAsAPrimitive pins that every XML Schema
// 1.0 built-in datatype, the two ur-types included, reads as a BMM
// primitive, so that an item typed with any of them decodes as a typed
// string rather than fail as an unknown type; and that the built-ins no
// other primitive reads, such as `gYear`, `hexBinary` and `QName`, read as a
// String.
func TestREQ176_REQ056_EveryBuiltinReadsAsAPrimitive(t *testing.T) {
	asString := []string{
		"gYearMonth", "gYear", "gMonthDay", "gDay", "gMonth", "hexBinary", "base64Binary",
		"QName", "NOTATION", "NMTOKENS", "IDREFS", "ENTITY", "ENTITIES", "anySimpleType", "anyType",
	}
	for _, local := range asString {
		if got, ok := Builtin(local); !ok || got != "String" {
			t.Errorf("Builtin(%q) = %q, %t; want \"String\"", local, got, ok)
		}
	}
	every := append([]string{
		"string", "boolean", "decimal", "float", "double", "duration", "dateTime", "time", "date", "anyURI",
		"normalizedString", "token", "language", "NMTOKEN", "Name", "NCName", "ID", "IDREF",
		"integer", "nonPositiveInteger", "negativeInteger", "long", "int", "short", "byte",
		"nonNegativeInteger", "unsignedLong", "unsignedInt", "unsignedShort", "unsignedByte", "positiveInteger",
	}, asString...)
	for _, local := range every {
		if _, ok := Builtin(local); !ok {
			t.Errorf("Builtin(%q) reads as no primitive; want every XML Schema 1.0 built-in to read as one", local)
		}
	}
	if len(builtins) != len(every) {
		t.Errorf("the table holds %d built-ins; want the %d of XML Schema 1.0", len(builtins), len(every))
	}
}

// TestREQ176_REQ052_IntegerBuiltinsByRange pins the primitive each integer
// built-in reads as: an Integer where a 32-bit Integer holds the built-in's
// whole range, and an Integer64 where it does not, so that a value the
// built-in admits beyond 32 bits, such as an `unsignedInt` of 3000000000, is
// written in canonical JSON.
func TestREQ176_REQ052_IntegerBuiltinsByRange(t *testing.T) {
	for local, want := range map[string]string{
		"int": "Integer", "short": "Integer", "byte": "Integer", "unsignedShort": "Integer", "unsignedByte": "Integer",
		"integer": "Integer64", "nonNegativeInteger": "Integer64", "positiveInteger": "Integer64",
		"nonPositiveInteger": "Integer64", "negativeInteger": "Integer64", "unsignedInt": "Integer64",
		"long": "Integer64", "unsignedLong": "Integer64",
	} {
		if got, ok := Builtin(local); !ok || got != want {
			t.Errorf("Builtin(%q) = %q, %t; want %q", local, got, ok, want)
		}
	}
}
