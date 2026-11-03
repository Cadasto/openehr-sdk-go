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
