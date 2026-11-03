package xmlschema

// NS is the XML Schema namespace, which binds the prefix of an XML Schema
// built-in datatype such as `xs:string`.
const NS = "http://www.w3.org/2001/XMLSchema"

// LiteralPrefix is the prefix an `xsi:type` value may carry with no namespace
// declaration in scope that binds it and still name an XML Schema built-in
// datatype, as producers write `xsd:string` without declaring `xsd`.
const LiteralPrefix = "xsd"

// builtins maps the XML Schema built-in datatypes the openEHR ITS-XML schemas
// and the producers of canonical XML write openEHR primitives as to the BMM
// primitive each reads as.
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
	"integer":            "Integer",
	"short":              "Integer",
	"byte":               "Integer",
	"nonNegativeInteger": "Integer",
	"positiveInteger":    "Integer",
	"nonPositiveInteger": "Integer",
	"negativeInteger":    "Integer",
	"unsignedInt":        "Integer",
	"unsignedShort":      "Integer",
	"unsignedByte":       "Integer",
	"long":               "Integer64",
	"unsignedLong":       "Integer64",
	"float":              "Real",
	"decimal":            "Real",
	"double":             "Double",
	"date":               "Iso8601_date",
	"time":               "Iso8601_time",
	"dateTime":           "Iso8601_date_time",
	"duration":           "Iso8601_duration",
}

// Builtin returns the BMM primitive the XML Schema built-in datatype named
// local reads as, such as `String` for `string`, and false when local names
// none of the built-in datatypes it reads.
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
