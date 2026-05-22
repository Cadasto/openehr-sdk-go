package bmmgen

// primitiveGoType maps BMM primitive class names (as they appear in
// `primitive_types` in openehr_base_1.3.0.bmm.json — or as
// type-name references inside properties) to the Go type expression
// the generator should emit for them. Per
// docs/specifications/bmm-conformance.md § Primitive type mapping.
//
// An entry mapping to "" means "skip this primitive — do NOT emit a
// Go type for it, and a property typed by this name should be
// treated specially by the caller". The set of names mapped to "" is
// also accessible via [isSkippedPrimitive].
var primitiveGoType = map[string]string{
	// Basic scalars
	"Boolean":   "bool",
	"Integer":   "Integer",
	"Integer64": "int64",
	"Real":      "Real",
	"Double":    "float64",
	"Character": "rune",
	"String":    "string",
	"Octet":     "byte",
	"Uri":       "string",
	"Any":       "any",

	// ISO 8601 family — wire-fidelity matters; keep as strings.
	"Iso8601_date":      "string",
	"Iso8601_time":      "string",
	"Iso8601_date_time": "string",
	"Iso8601_duration":  "string",
	"Iso8601_type":      "string", // abstract — alias only
	"Iso8601_timezone":  "string",
	"Temporal":          "string", // abstract — alias only
}

// skippedPrimitives are abstract foundation types we deliberately do
// not emit as Go types. They never appear as concrete object types
// in the BMM property graph; consumers that hit one of these via a
// property type get a fallback (typically `any`) in render.
var skippedPrimitives = map[string]bool{
	"Numeric":         true,
	"Ordered":         true,
	"Ordered_Numeric": true,
	"Comparable":      true,
	"Container":       true,

	// Functional foundation — not used by the RM.
	"TUPLE":     true,
	"TUPLE1":    true,
	"TUPLE2":    true,
	"FUNCTION":  true,
	"ROUTINE":   true,
	"PROCEDURE": true,

	// Built-in environment classes — abstract, no value as Go types.
	"Env":                   true,
	"Math":                  true,
	"Locale":                true,
	"Statistical_evaluator": true,
	"Quantity_converter":    true,

	// Container primitives are handled by the container-mapping logic;
	// the names below MUST NOT be emitted as their own Go types.
	"List":  true,
	"Set":   true,
	"Array": true,
	"Hash":  true,
}

// isPrimitive reports whether name maps to a Go primitive (e.g.
// String → string). Container kinds and skipped primitives return
// false.
func isPrimitive(name string) bool {
	_, ok := primitiveGoType[name]
	return ok
}

// isSkippedPrimitive reports whether the named class should be
// silently skipped — never emitted as a Go type. Properties typed by
// a skipped primitive emit `any` with a TODO comment (the generator
// reports them in its summary).
func isSkippedPrimitive(name string) bool {
	return skippedPrimitives[name]
}

// containerKinds are the BMM container_type strings handled by the
// container-mapping logic. List/Set/Array map to []T; Hash to
// map[K]V.
var containerKinds = map[string]bool{
	"List":  true,
	"Set":   true,
	"Array": true,
	"Hash":  true,
}

// skippedClasses are BMM class_definitions we never emit as Go
// types. The set parallels skippedPrimitives but for entries that
// live in class_definitions rather than primitive_types in the BMM.
var skippedClasses = map[string]bool{
	// Functional foundation_types.functional package.
	"TUPLE":     true,
	"TUPLE1":    true,
	"TUPLE2":    true,
	"FUNCTION":  true,
	"ROUTINE":   true,
	"PROCEDURE": true,

	// foundation_types.builtins package — abstract environment.
	"Env":                   true,
	"Math":                  true,
	"Locale":                true,
	"Statistical_evaluator": true,
	"Quantity_converter":    true,

	// Abstract type hierarchy from foundation_types.primitive_types
	// that lands in class_definitions in some BMM dialects (notably
	// openehr_base 1.3.0). These are typing constraints, not data.
	"Numeric":         true,
	"Ordered":         true,
	"Ordered_Numeric": true,
	"Comparable":      true,
	"Container":       true,

	// Time_Definitions has only constants (no instance state). Emit
	// as an empty marker struct so descendant emit can still embed it.
	// Keep it OFF the skip list — handled by render.
}

// isSkippedClass reports whether the named class should be skipped
// entirely by the generator (no Go type emitted, no typereg entry).
func isSkippedClass(name string) bool {
	return skippedClasses[name]
}

// skippedPackagePrefixes are the BMM package dotted-name prefixes
// the generator skips wholesale. Any class whose package path starts
// with one of these is omitted from emission.
//
// Per REQ-042, org.openehr.rm.ehr_extract is excluded. The
// foundation_types.functional and foundation_types.builtins packages
// are also excluded — see plan § "Skip foundation_types.functional".
var skippedPackagePrefixes = []string{
	"org.openehr.rm.ehr_extract",
	"org.openehr.base.foundation_types.functional",
	"org.openehr.base.foundation_types.builtins",
	"org.openehr.base.base_types.builtins",
}

// isSkippedPackage reports whether the BMM package's dotted name
// matches one of the skip prefixes.
func isSkippedPackage(pkgName string) bool {
	for _, p := range skippedPackagePrefixes {
		if pkgName == p {
			return true
		}
		if len(pkgName) > len(p) && pkgName[:len(p)] == p && pkgName[len(p)] == '.' {
			return true
		}
	}
	return false
}
