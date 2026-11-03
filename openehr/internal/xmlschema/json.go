package xmlschema

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// MarshalJSONTo writes s as a JSON object whose `_type` is the BMM primitive
// its Type reads as and whose `value` is its text as that primitive's JSON
// value: `{"_type":"String","value":"archetype_id/value"}` for an `xs:string`
// item. A String, a Uri and the ISO 8601 primitives write the text as a JSON
// string. A Boolean writes `true` or `1` as true and `false` or `0` as false,
// XML white space around the text removed as XML Schema reads a boolean. An
// Integer or an Integer64 writes a JSON integer, a whole number of 32 or 64
// bits. A Real or a Double writes the text itself as a JSON number, a literal
// of the XML Schema built-in Type names in the float64 range, a decimal with
// no exponent for `decimal` and a floating-point literal for `float` and
// `double`, so the number written has the text's value exactly, digits a
// float64 would not keep included: XML white space around the text is
// removed, and the text is
// rewritten only where JSON's number grammar differs from XML Schema's, a
// leading `+` and leading zeros dropped, `0` written before a leading decimal
// point and a trailing one dropped, so `+.5` is written `0.5` and `1.E3`
// `1E3`. The JSON keeps neither Type's prefix nor which of the primitive's
// built-ins Type names.
//
// A Type that is no prefixed XML Schema built-in datatype is refused, as the
// XML encoder refuses it ([TypedString.MarshalXML]), and so is text the
// built-in does not read, such as a Boolean written `yes`, an Integer out of
// range, an `xs:decimal` written with an exponent, such as `1e3`, or a Double
// written `INF`, which a JSON number cannot hold. Neither refusal quotes the
// text.
func (s TypedString) MarshalJSONTo(enc *jsontext.Encoder) error {
	primitive, ok := typedPrimitive(s.Type)
	if !ok {
		return errTypedType
	}
	_, builtin, _ := strings.Cut(s.Type, ":")
	value, number, err := jsonValue(primitive, builtin, s.Value)
	if err != nil {
		return err
	}
	for _, tok := range [...]jsontext.Token{
		jsontext.BeginObject,
		jsontext.String("_type"), jsontext.String(primitive),
		jsontext.String("value"),
	} {
		if err := enc.WriteToken(tok); err != nil {
			return err
		}
	}
	if number != nil {
		err = enc.WriteValue(number)
	} else {
		err = enc.WriteToken(value)
	}
	if err != nil {
		return err
	}
	return enc.WriteToken(jsontext.EndObject)
}

// errTypedType is the refusal of a typed item whose Type is no prefixed XML
// Schema built-in datatype.
var errTypedType = errors.New("the xsi:type of a typed item is no prefixed XML Schema built-in datatype")

// typedPrimitive returns the BMM primitive t reads as when t is an XML Schema
// built-in datatype under a prefix, the `xsi:type` a [TypedString] carries,
// and false otherwise. The prefix is one the XML encoder can declare bound to
// the XML Schema namespace and read back: an XML NCName other than `xml` and
// `xmlns`, which XML reserves, and `xsi`, which the encoder binds to the XML
// Schema instance namespace for the `xsi:type` attribute itself.
func typedPrimitive(t string) (string, bool) {
	prefix, local, ok := strings.Cut(t, ":")
	if !ok || !isNCName(prefix) {
		return "", false
	}
	switch prefix {
	case "xml", "xmlns", "xsi":
		return "", false
	}
	return Builtin(local)
}

// isNCName reports whether s is an XML NCName (Namespaces in XML 1.0: an XML
// name with no colon) that encoding/xml reads back as one. In ASCII that is a
// letter or `_` followed by letters, digits, `_`, `-` and `.`. Beyond ASCII,
// encoding/xml reads names by the character tables of the first edition of
// XML 1.0, which admit fewer characters than the NameStartChar and NameChar of
// the fifth edition, so s is held to what it reads.
func isNCName(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= utf8.RuneSelf:
			return readsAsName(s)
		case c == '_', 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case i > 0 && (c == '-' || c == '.' || '0' <= c && c <= '9'):
		default:
			return false
		}
	}
	return true
}

// readsAsName reports whether encoding/xml reads s, which holds no colon, as
// the whole name of an element.
func readsAsName(s string) bool {
	tok, err := xml.NewDecoder(strings.NewReader("<" + s + "/>")).RawToken()
	start, ok := tok.(xml.StartElement)
	return err == nil && ok && start.Name.Space == "" && start.Name.Local == s
}

// xmlSpace is the white space XML Schema removes around the text of a boolean
// or a number: space, tab, carriage return and line feed.
const xmlSpace = " \t\r\n"

// jsonValue returns text, the text of a typed item whose type, the XML
// Schema built-in named builtin, reads as primitive, as the JSON value
// [TypedString.MarshalJSONTo] writes for it: the token value, or for a Real or
// a Double the JSON number in number, or the refusal of text the built-in does
// not read.
func jsonValue(primitive, builtin, text string) (value jsontext.Token, number jsontext.Value, err error) {
	switch primitive {
	case "Boolean":
		switch strings.Trim(text, xmlSpace) {
		case "true", "1":
			return jsontext.True, nil, nil
		case "false", "0":
			return jsontext.False, nil, nil
		}
		return jsontext.Token{}, nil, textRefusal(primitive, strconv.ErrSyntax)
	case "Integer", "Integer64":
		bits := 32
		if primitive == "Integer64" {
			bits = 64
		}
		n, err := strconv.ParseInt(strings.Trim(text, xmlSpace), 10, bits)
		if err != nil {
			return jsontext.Token{}, nil, textRefusal(primitive, numCause(err))
		}
		return jsontext.Int(n), nil, nil
	case "Real", "Double":
		number, err := jsonNumber(primitive, builtin, text)
		return jsontext.Token{}, number, err
	}
	return jsontext.String(text), nil, nil
}

// jsonNumber returns text, the text of a typed item whose type, the XML
// Schema built-in named builtin, reads as primitive, a Real or a Double, as
// the JSON number [TypedString.MarshalJSONTo] writes for it, or the refusal of
// text the built-in does not read.
func jsonNumber(primitive, builtin, text string) (jsontext.Value, error) {
	s := strings.Trim(text, xmlSpace)
	switch {
	case s == "INF" || s == "+INF" || s == "-INF" || s == "NaN":
		return nil, fmt.Errorf("the text of a typed item is no finite %s, which canonical JSON cannot write", primitive)
	case !isDecimal(builtin, s):
		return nil, textRefusal(primitive, strconv.ErrSyntax)
	}
	// The float64 refuses a literal beyond its range, which canonical JSON
	// does not read back as the primitive; the number written is s's own.
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return nil, textRefusal(primitive, numCause(err))
	}
	return appendJSONNumber(make(jsontext.Value, 0, len(s)+1), s), nil
}

// appendJSONNumber appends s, a literal [isDecimal] accepts, to dst as a JSON
// number of the same value, rewritten only where JSON's number grammar
// differs from XML Schema's: a leading plus sign and leading zeros dropped, a
// zero written before a leading decimal point and a trailing decimal point
// dropped. The exponent is written as it is.
func appendJSONNumber(dst []byte, s string) []byte {
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		dst = append(dst, '-')
		s = s[1:]
	}
	mant, exp := s, ""
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		mant, exp = s[:k], s[k:]
	}
	intPart, frac, _ := strings.Cut(mant, ".")
	if intPart = strings.TrimLeft(intPart, "0"); intPart == "" {
		intPart = "0"
	}
	dst = append(dst, intPart...)
	if frac != "" {
		dst = append(dst, '.')
		dst = append(dst, frac...)
	}
	return append(dst, exp...)
}

// textRefusal is the refusal of the text of a typed item that its primitive
// does not read, for the reason cause gives, a strconv sentinel. It does not
// quote the text.
func textRefusal(primitive string, cause error) error {
	return fmt.Errorf("the text of a typed item does not read as %s: %w", primitive, cause)
}

// numCause returns the sentinel a *strconv.NumError carries, which names the
// class of the failure and none of the text, or err itself for any other
// error.
func numCause(err error) error {
	if ne, ok := errors.AsType[*strconv.NumError](err); ok && ne != nil {
		return ne.Err
	}
	return err
}

// isDecimal reports whether s is a decimal number as XML Schema writes one
// of the built-in named builtin, `decimal`, `float` or `double`, the special
// values of the last two aside: an optional sign, digits with at most one
// decimal point, and, for a `float` or a `double`, an optional exponent of an
// optional sign and digits. A `decimal` has no exponent.
func isDecimal(builtin, s string) bool {
	mant, exp, hasExp := s, "", false
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		if builtin == "decimal" {
			return false
		}
		mant, exp, hasExp = s[:k], s[k+1:], true
	}
	intPart, frac, _ := strings.Cut(trimSign(mant), ".")
	if intPart == "" && frac == "" {
		return false
	}
	if hasExp && trimSign(exp) == "" {
		return false
	}
	return isDigits(intPart) && isDigits(frac) && isDigits(trimSign(exp))
}

// trimSign drops one leading sign from s.
func trimSign(s string) string {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// isDigits reports whether every byte of s is an ASCII decimal digit; an
// empty s has none that is not.
func isDigits(s string) bool {
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// typedJSON is the canonical-JSON form of a [TypedString] as
// [TypedString.UnmarshalJSONFrom] reads it: its `value` kept as written until
// the primitive its `_type` names says how to read it.
type typedJSON struct {
	Type  string         `json:"_type"`
	Value jsontext.Value `json:"value"`
}

var (
	// errTypedPrimitive is the refusal of a typed item whose `_type` names no
	// BMM primitive an XML Schema built-in datatype reads as.
	errTypedPrimitive = errors.New("the _type of a typed item names no primitive an XML Schema built-in datatype reads as")
	// errTypedNoValue is the refusal of a typed item whose `value` is missing
	// or null.
	errTypedNoValue = errors.New("a typed item holds no value")
)

// UnmarshalJSONFrom reads s from the JSON object [TypedString.MarshalJSONTo]
// writes. Type is `xsd:` and the XML Schema built-in canonical for the
// primitive the `_type` names: `string` for a String, `anyURI` for a Uri,
// `boolean`, `int` for an Integer, `long` for an Integer64, `float` for a
// Real, `double`, and `date`, `time`, `dateTime` and `duration` for the ISO
// 8601 primitives. Value is the `value` as text: a JSON string's own text,
// `true` or `false`, or a JSON number as written.
//
// An object whose `_type` names none of those primitives, whose `value` is
// missing or null or of another JSON kind than its primitive writes, or whose
// `value` is a number the primitive cannot hold, such as an Integer written
// 2.5 or out of the 32-bit range, is refused as a shape failure that wraps
// typereg.ErrInvalidShape.
func (s *TypedString) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var w typedJSON
	if err := json.UnmarshalDecode(dec, &w); err != nil {
		return typereg.ClassifyShape(err)
	}
	builtin, ok := canonicalBuiltins[w.Type]
	if !ok {
		return typereg.ClassifyShape(errTypedPrimitive)
	}
	text, err := typedText(w.Type, w.Value)
	if err != nil {
		return typereg.ClassifyShape(err)
	}
	*s = TypedString{Value: text, Type: LiteralPrefix + ":" + builtin}
	return nil
}

// typedText returns v, the `value` of a typed item whose `_type` names
// primitive, as the item's text, or the refusal of a value of another JSON
// kind than primitive writes or one it cannot hold.
func typedText(primitive string, v jsontext.Value) (string, error) {
	if len(v) == 0 || v.Kind() == 'n' {
		return "", errTypedNoValue
	}
	var err error
	switch primitive {
	case "Boolean":
		var b bool
		if err = json.Unmarshal(v, &b); err == nil {
			return strconv.FormatBool(b), nil
		}
	case "Integer":
		var n int32
		err = json.Unmarshal(v, &n)
	case "Integer64":
		var n int64
		err = json.Unmarshal(v, &n)
	case "Real", "Double":
		var f float64
		err = json.Unmarshal(v, &f)
	default:
		var text string
		if err = json.Unmarshal(v, &text); err == nil {
			return text, nil
		}
	}
	if err != nil {
		return "", err
	}
	return string(v), nil
}

// errAnyJSON is the refusal of a JSON value a BMM `Any` member does not hold.
var errAnyJSON = errors.New("an Any member holds a JSON object, a string or null")

// DecodeAnyJSON decodes the next JSON value of dec into *out, the value of a
// BMM `Any` member such as the AOM 1.4 `EXPR_LEAF.item`, as canonical JSON
// writes the values [DecodeAny] reads from canonical XML:
//
//   - an object whose `_type` names a BMM primitive an XML Schema built-in
//     datatype reads as, such as `String`, as a *[TypedString], before the
//     type registry is asked;
//   - an object with any other `_type` as the class it names, through the
//     type registry, as at a polymorphic slot;
//   - a string as a Go string;
//   - null as nil.
//
// An object with no `_type` fails as at a polymorphic slot, with
// typereg.ErrMissingType, and any other JSON value is refused.
func DecodeAnyJSON(dec *jsontext.Decoder, out *any) error {
	switch dec.PeekKind() {
	case jsontext.KindNull:
		*out = nil
		_, err := dec.ReadToken()
		return err
	case jsontext.KindString:
		var s string
		if err := json.UnmarshalDecode(dec, &s); err != nil {
			return err
		}
		*out = s
		return nil
	case jsontext.KindBeginObject:
		return decodeAnyObject(dec, out)
	case jsontext.KindFalse, jsontext.KindTrue, jsontext.KindNumber, jsontext.KindBeginArray,
		jsontext.KindEndObject, jsontext.KindEndArray, jsontext.KindInvalid:
	}
	return errAnyJSON
}

// decodeAnyObject decodes the JSON object dec reads next into *out, as
// [DecodeAnyJSON] says. Where the object opens with its `_type`, as canonical
// JSON writes it, it decodes straight from dec; any other object it reads
// whole first, to find its `_type`.
func decodeAnyObject(dec *jsontext.Decoder, out *any) error {
	primitive, known := leadingPrimitive(dec)
	if !known {
		raw, err := dec.ReadValue()
		if err != nil {
			return typereg.ClassifyDuplicate(err)
		}
		opts := dec.Options()
		primitive = namesPrimitive(raw, opts)
		dec = jsontext.NewDecoder(bytes.NewReader(raw), opts)
	}
	if !primitive {
		return typereg.DecodePolymorphic(dec, out, nil)
	}
	s := new(TypedString)
	if err := s.UnmarshalJSONFrom(dec); err != nil {
		return err
	}
	*out = s
	return nil
}

// leadingPrimitive reads the `_type` of the JSON object dec reads next from
// the bytes dec holds unread, without reading the object, and reports whether
// it names a BMM primitive an XML Schema built-in datatype reads as. known is
// false where those bytes do not settle it: the object does not open with
// `_type`, spelled so and holding a JSON string of plain ASCII with no escape,
// or dec has not read that far into its input; and where dec's options could
// read another member as `_type`, allowing duplicate names or matching names
// without regard to case.
func leadingPrimitive(dec *jsontext.Decoder) (primitive, known bool) {
	opts := dec.Options()
	if dup, _ := json.GetOption(opts, jsontext.AllowDuplicateNames); dup {
		return false, false
	}
	if fold, _ := json.GetOption(opts, json.MatchCaseInsensitiveNames); fold {
		return false, false
	}
	// The unread bytes hold the object next, maybe after the colon or comma
	// that separates it from what came before.
	b := dec.UnreadBuffer()
	i := skipJSONSpace(b, 0)
	if i < len(b) && (b[i] == ':' || b[i] == ',') {
		i = skipJSONSpace(b, i+1)
	}
	if i >= len(b) || b[i] != '{' {
		return false, false
	}
	const key = `"_type"`
	i = skipJSONSpace(b, i+1)
	if !bytes.HasPrefix(b[i:], []byte(key)) {
		return false, false
	}
	i = skipJSONSpace(b, i+len(key))
	if i >= len(b) || b[i] != ':' {
		return false, false
	}
	i = skipJSONSpace(b, i+1)
	if i >= len(b) || b[i] != '"' {
		return false, false
	}
	for j := i + 1; j < len(b); j++ {
		switch c := b[j]; {
		case c == '"':
			_, primitive = canonicalBuiltins[string(b[i+1:j])]
			return primitive, true
		case c == '\\' || c < 0x20 || c >= 0x80:
			return false, false
		}
	}
	return false, false
}

// skipJSONSpace returns the index of the first byte of b at or after i that is
// not JSON white space (space, tab, line feed or carriage return), or len(b).
func skipJSONSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// namesPrimitive reports whether raw, a JSON object, has a `_type` that names
// a BMM primitive an XML Schema built-in datatype reads as, read under opts,
// the options of the decode the object sits in.
func namesPrimitive(raw jsontext.Value, opts json.Options) bool {
	var head struct {
		Type string `json:"_type"`
	}
	if json.Unmarshal(raw, &head, opts, json.RejectUnknownMembers(false)) != nil {
		return false
	}
	_, ok := canonicalBuiltins[head.Type]
	return ok
}

// AnyJSON decodes a mandatory BMM `Any` member, held as a Go any at To, from
// canonical JSON ([DecodeAnyJSON]). A generated decoder sets it in the place
// of the member, so that the member's JSON value decodes through it.
type AnyJSON struct {
	To *any
}

// UnmarshalJSONFrom decodes the member's value into *a.To.
func (a AnyJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	return DecodeAnyJSON(dec, a.To)
}

// OptionalAnyJSON decodes an optional BMM `Any` member, held as a Go *any at
// To, from canonical JSON ([DecodeAnyJSON]): null leaves the member nil.
type OptionalAnyJSON struct {
	To **any
}

// UnmarshalJSONFrom decodes the member's value into a new any *a.To points to,
// or sets *a.To to nil for null.
func (a OptionalAnyJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var v any
	if err := DecodeAnyJSON(dec, &v); err != nil {
		return err
	}
	if v == nil {
		*a.To = nil
		return nil
	}
	*a.To = &v
	return nil
}

// errChoiceJSON is the refusal of a JSON value a member whose canonical XML
// takes a choice of shapes does not hold.
var errChoiceJSON = errors.New("a member of a choice of shapes holds a JSON object or null")

// DecodeChoiceJSON decodes the next JSON value of dec into *out, the value of a
// BMM `Any` member whose canonical XML takes one of several shapes, such as the
// AOM 1.4 `ORDINAL.symbol`: an object whose `_type` names one of types, the BMM
// names of the shapes, as that class, through the type registry, as at a
// polymorphic slot; and null as nil. A `_type` the registry holds that names
// none of the shapes is refused with typereg.ErrTypeMismatch, and any other
// JSON value as a shape failure.
func DecodeChoiceJSON(dec *jsontext.Decoder, out *any, types []string) error {
	switch dec.PeekKind() {
	case jsontext.KindNull:
		*out = nil
		_, err := dec.ReadToken()
		return err
	case jsontext.KindBeginObject:
		var v any
		if err := typereg.DecodePolymorphic(dec, &v, nil); err != nil {
			return err
		}
		if n, ok := v.(interface{ BMMName() string }); !ok || !slices.Contains(types, n.BMMName()) {
			return &typereg.DecodeError{Inner: fmt.Errorf("canjson: decoded %T, none of the member's shapes: %w", v, typereg.ErrTypeMismatch)}
		}
		*out = v
		return nil
	case jsontext.KindString, jsontext.KindFalse, jsontext.KindTrue, jsontext.KindNumber, jsontext.KindBeginArray,
		jsontext.KindEndObject, jsontext.KindEndArray, jsontext.KindInvalid:
	}
	return errChoiceJSON
}

// ChoiceJSON decodes a mandatory member whose canonical XML takes a choice of
// shapes, held as a Go any at To, from canonical JSON ([DecodeChoiceJSON]):
// Types names the BMM classes of its shapes. A generated decoder sets it in the
// place of the member, so that the member's JSON value decodes through it.
type ChoiceJSON struct {
	To    *any
	Types []string
}

// UnmarshalJSONFrom decodes the member's value into *c.To.
func (c ChoiceJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	return DecodeChoiceJSON(dec, c.To, c.Types)
}

// OptionalChoiceJSON decodes an optional member whose canonical XML takes a
// choice of shapes, held as a Go *any at To, from canonical JSON
// ([DecodeChoiceJSON]): null leaves the member nil.
type OptionalChoiceJSON struct {
	To    **any
	Types []string
}

// UnmarshalJSONFrom decodes the member's value into a new any *c.To points to,
// or sets *c.To to nil for null.
func (c OptionalChoiceJSON) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var v any
	if err := DecodeChoiceJSON(dec, &v, c.Types); err != nil {
		return err
	}
	if v == nil {
		*c.To = nil
		return nil
	}
	*c.To = &v
	return nil
}
