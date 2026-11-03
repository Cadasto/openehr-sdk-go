package xmlschema

// NS is the XML Schema namespace, which binds the prefix of an XML Schema
// built-in datatype such as `xs:string`.
const NS = "http://www.w3.org/2001/XMLSchema"

// LiteralPrefix is the prefix an `xsi:type` value may carry with no namespace
// declaration in scope that binds it and still name an XML Schema built-in
// datatype, as producers write `xsd:string` without declaring `xsd`.
const LiteralPrefix = "xsd"

// builtins maps every XML Schema 1.0 built-in datatype, the ur-types
// `anyType` and `anySimpleType` included, to the BMM primitive it reads as.
// An integer built-in reads as an Integer where a 32-bit Integer holds its
// whole range, `int` and the narrower ones, and as an Integer64 otherwise, so
// a value the built-in admits is not refused for its range where a 64-bit
// Integer64 holds it. A built-in no other primitive reads, such as `gYear`,
// `hexBinary` or `QName`, reads as a String, so an item typed with it keeps
// its text and the `xsi:type` it was written with.
var builtins = map[string]string{
	"string":             "String",
	"normalizedString":   "String",
	"token":              "String",
	"language":           "String",
	"Name":               "String",
	"NCName":             "String",
	"NMTOKEN":            "String",
	"ID":                 "String",
	"IDREF":              "String",
	"anyURI":             "Uri",
	"boolean":            "Boolean",
	"int":                "Integer",
	"short":              "Integer",
	"byte":               "Integer",
	"unsignedShort":      "Integer",
	"unsignedByte":       "Integer",
	"integer":            "Integer64",
	"nonNegativeInteger": "Integer64",
	"positiveInteger":    "Integer64",
	"nonPositiveInteger": "Integer64",
	"negativeInteger":    "Integer64",
	"unsignedInt":        "Integer64",
	"long":               "Integer64",
	"unsignedLong":       "Integer64",
	"float":              "Real",
	"decimal":            "Real",
	"double":             "Double",
	"date":               "Iso8601_date",
	"time":               "Iso8601_time",
	"dateTime":           "Iso8601_date_time",
	"duration":           "Iso8601_duration",
	"gYearMonth":         "String",
	"gYear":              "String",
	"gMonthDay":          "String",
	"gDay":               "String",
	"gMonth":             "String",
	"hexBinary":          "String",
	"base64Binary":       "String",
	"QName":              "String",
	"NOTATION":           "String",
	"NMTOKENS":           "String",
	"IDREFS":             "String",
	"ENTITY":             "String",
	"ENTITIES":           "String",
	"anySimpleType":      "String",
	"anyType":            "String",
}

// Builtin returns the BMM primitive the XML Schema built-in datatype named
// local reads as, such as `String` for `string`, and false when local names
// no XML Schema 1.0 built-in datatype.
func Builtin(local string) (string, bool) {
	name, ok := builtins[local]
	return name, ok
}

// canonicalBuiltins maps each BMM primitive an XML Schema built-in datatype
// reads as ([Builtin]) to the one built-in a typed item decoded from canonical
// JSON is given, since canonical JSON keeps the primitive and not the
// built-in.
var canonicalBuiltins = map[string]string{
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
