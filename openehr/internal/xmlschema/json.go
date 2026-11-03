package xmlschema

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// typedStringJSON is the canonical-JSON form of a [TypedString].
type typedStringJSON struct {
	Value string `json:"value"`
	Type  string `json:"xsi_type"`
}

// MarshalJSONTo writes s as a JSON object holding its text and its
// `xsi:type`, `{"value":"archetype_id/value","xsi_type":"xs:string"}`, with no
// `_type`: a TypedString is a String, which canonical JSON writes with no type
// name, kept as an object so that the `xsi:type` it was read with survives.
func (s TypedString) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, typedStringJSON(s), typereg.MarshalOptions(enc))
}

// errTypedStringJSON is the refusal of a typed string whose `xsi_type` is no
// prefixed XML Schema built-in datatype. It names no value.
var errTypedStringJSON = errors.New("the xsi_type of a typed string is no prefixed XML Schema built-in datatype")

// UnmarshalJSONFrom reads s from the JSON object [TypedString.MarshalJSONTo]
// writes. An `xsi_type` that is no prefixed XML Schema built-in datatype, which
// canonical XML could not write, is refused, as is any other object, as a
// shape failure that wraps typereg.ErrInvalidShape.
func (s *TypedString) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var w typedStringJSON
	if err := json.UnmarshalDecode(dec, &w); err != nil {
		return typereg.ClassifyShape(err)
	}
	if !isTypedStringType(w.Type) {
		return typereg.ClassifyShape(errTypedStringJSON)
	}
	*s = TypedString(w)
	return nil
}

// isTypedStringType reports whether t is an XML Schema built-in datatype under
// a prefix, the `xsi:type` a [TypedString] carries.
func isTypedStringType(t string) bool {
	prefix, local, ok := strings.Cut(t, ":")
	_, builtin := Builtin(local)
	return ok && prefix != "" && builtin
}

// errAnyJSON is the refusal of a JSON value a BMM `Any` member does not hold.
var errAnyJSON = errors.New("an Any member holds a JSON object, a string or null")

// DecodeAnyJSON decodes the next JSON value of dec into *out, the value of a
// BMM `Any` member such as the AOM 1.4 `EXPR_LEAF.item`, as canonical JSON
// writes the values [DecodeAny] reads from canonical XML:
//
//   - an object with a `_type` as the class it names, through the type
//     registry, as at a polymorphic slot;
//   - an object with no `_type` as a *[TypedString];
//   - a string as a Go string;
//   - null as nil.
//
// Any other JSON value is refused.
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
		return typereg.DecodePolymorphic(dec, out, func() any { return new(TypedString) })
	case jsontext.KindFalse, jsontext.KindTrue, jsontext.KindNumber, jsontext.KindBeginArray,
		jsontext.KindEndObject, jsontext.KindEndArray, jsontext.KindInvalid:
	}
	return errAnyJSON
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
