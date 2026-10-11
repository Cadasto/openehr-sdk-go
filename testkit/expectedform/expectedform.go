// Package expectedform builds the expected form of a canonical-JSON input: the
// document that a decode and re-encode of the input must be wire-equivalent to
// (testkit/wireequiv). The canonical-JSON round-trip tests and probes compare
// their first encode with it, so a member the decoder reads with the wrong
// meaning shows up as a difference against the input, not only between two
// encodes of the SDK's own.
//
// [Build] writes the expected form. It is the input with these changes, and no
// others:
//
//   - a member that names no attribute of its object's RM type is removed;
//   - a member whose value is null is removed, except a mandatory String
//     member, which becomes the empty string the encoder writes for it (a Go
//     string holds no null);
//   - an object that leaves out "_type" gains it: its RM type is the declared
//     type of the attribute that holds it, and the root's type is the decode
//     target's;
//   - an interval that leaves out a boundary flag gains it with its default:
//     "*_unbounded" is false, and "*_included" is true on a side that is not
//     unbounded and false on a side that is;
//   - an empty bound on a side marked unbounded is removed;
//   - an empty DV_TEXT "mappings", DV_MULTIMEDIA "data" or DV_MULTIMEDIA
//     "integrity_check" is removed;
//   - a number held by an Integer, Integer64, Real or Double attribute, written
//     as a JSON number or as a quoted string, is replaced by the literal the
//     encoder writes for its value.
//
// No change is made inside a generic payload, such as the "data" of an
// ORIGINAL_VERSION: the decoder holds it as plain JSON rather than as RM
// values, so none of the changes above applies to it.
//
// [Prune] applies the first change only. A caller counting what the input
// carries on the members the RM defines uses it.
//
// The members the first change removes are the ones canjson.WithUnmatched
// reports for the same input: both get them from the same walker, which reads
// each object's RM type and the RM's attribute lists from [rminfo.Default].
// Member order is not part of either result: compare with
// [wireequiv.Equivalent].
//
// The package reads its input with the v1 encoding/json package and keeps every
// number as its literal text, for the reason testkit/wireequiv gives: it is an
// independent reader of the wire bytes, not the codec under test. The walker
// that decides the removed members reads the bytes with encoding/json/jsontext,
// and it is not the decoder's code either.
package expectedform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Build returns the expected form of input, a canonical-JSON document whose
// root decodes into the RM type rootType. The package comment lists the changes
// it makes. An error means the input cannot be read against the RM: it is not
// a single JSON object, an object carries a "_type" no RM class has, an object
// leaves out "_type" where the declared type is abstract or a type parameter
// with no binding, or a member's value has the wrong JSON kind for its
// attribute. Build does not guess a type or a shape, so such an input fails
// loudly where the expected form is needed. The decoder refuses most of these
// inputs too, but not all: it reads an empty "_type" as an elided one, which
// Build refuses.
func Build(input []byte, rootType string) ([]byte, error) {
	return transform(input, rootType, true)
}

// Prune returns input with every member that names no attribute of its
// object's RM type removed, and nothing else changed: a null member, an elided
// "_type" and an omitted interval flag stay as they are. rootType and the
// errors are as for [Build].
func Prune(input []byte, rootType string) ([]byte, error) {
	return transform(input, rootType, false)
}

// RootType returns the RM type name of a decode target, such as "COMPOSITION"
// for a *rm.Composition, which is the rootType [Build] and [Prune] take. The
// second result is false when target does not name its BMM class.
func RootType(target any) (string, bool) {
	n, ok := target.(interface{ BMMName() string })
	if !ok {
		return "", false
	}
	name := n.BMMName()
	return name, name != ""
}

// intervalFlags are the four boundary flags BASE Interval declares. A class
// that carries all four is read as an interval.
var intervalFlags = [...]string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}

// transform decodes input, asks the shared walker which members are unmatched,
// walks the document from the root and encodes the result.
func transform(input []byte, rootType string, full bool) ([]byte, error) {
	if rootType == "" {
		return nil, errors.New("expectedform: the root type is empty")
	}
	w, err := newWalker(rminfo.Default, full)
	if err != nil {
		return nil, err
	}
	doc, err := decode(input)
	if err != nil {
		return nil, fmt.Errorf("expectedform: %w", err)
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, errors.New("expectedform: the document is not a JSON object")
	}
	w.unmatched = map[string]bool{}
	err = w.model.Unmatched(input, rootType, func(pointer string) error {
		w.unmatched[pointer] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("expectedform: %w", err)
	}
	out, err := w.object(root, rootType, "", "")
	if err != nil {
		return nil, fmt.Errorf("expectedform: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("expectedform: encode: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// decode parses one document into a generic JSON value, numbers kept as their
// literal text. Content after the first value is an error.
func decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("unexpected data after top-level JSON value")
		}
		return nil, err
	}
	return v, nil
}

// walker carries what every step of the walk needs: the RM answers, full,
// which selects Build over Prune, and the pointers of the members the shared
// walker reports as unmatched, which the walk removes.
type walker struct {
	model     *rmjsonwalk.Model
	lookup    rminfo.Lookup
	full      bool
	unmatched map[string]bool
}

// newWalker builds the shared RM model over l, which asserts the optional
// rminfo capabilities the walk needs, so a Lookup without them fails here
// rather than misreading a type.
func newWalker(l rminfo.Lookup, full bool) (walker, error) {
	m, err := rmjsonwalk.New(l)
	if err != nil {
		return walker{}, fmt.Errorf("expectedform: %w", err)
	}
	return walker{model: m, lookup: l, full: full}, nil
}

// object walks one JSON object whose declared RM type is declared. boundType
// binds the type parameter of the object's own attributes when the slot that
// holds it binds one (see rmjsonwalk.Attribute), and is empty otherwise.
//
// The walk does not decide on its own which members to remove: a member is
// removed when the shared walker reported its pointer. A member the walker did
// not report and the RM does not define is an error, since the two disagree.
func (w walker) object(obj map[string]any, declared, boundType, ptr string) (map[string]any, error) {
	rmType, err := w.resolve(obj, declared, ptr)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(obj)+len(intervalFlags)+1)
	for name, val := range obj {
		if name == "_type" {
			if val != nil || !w.full {
				out[name] = val
			}
			continue
		}
		child := ptr + "/" + escapeToken(name)
		if w.unmatched[child] {
			continue // names no attribute of rmType
		}
		attr, ok := w.model.Attribute(rmType, name, boundType)
		if !ok {
			return nil, fmt.Errorf("%s: names no attribute of %s, and the shared walker did not report it", child, rmType)
		}
		if val == nil {
			switch {
			case !w.full:
				out[name] = val
			case attr.Type == "String" && !attr.Container && slices.Contains(w.lookup.RequiredAttributes(rmType), name):
				out[name] = "" // a Go string holds no null: the encoder writes ""
			}
			continue
		}
		nv, keep, err := w.member(name, attr, val, child)
		if err != nil {
			return nil, err
		}
		if keep {
			out[name] = nv
		}
	}
	if w.full {
		out["_type"] = rmType
		if w.isInterval(rmType) {
			w.settleInterval(out, boundType)
		}
	}
	return out, nil
}

// resolve returns obj's RM type: its own "_type" when it carries one, else the
// declared type of the slot that holds it.
func (w walker) resolve(obj map[string]any, declared, ptr string) (string, error) {
	var (
		typ     string
		hasType bool
	)
	switch t := obj["_type"].(type) {
	case string:
		typ, hasType = t, true
	case nil:
		// Absent, or null: the declared type applies.
	default:
		return "", fmt.Errorf("%s: _type is not a string", pointerOrRoot(ptr))
	}
	rmType, err := w.model.Resolve(typ, hasType, declared)
	if err != nil {
		return "", fmt.Errorf("%s: %w", pointerOrRoot(ptr), err)
	}
	return rmType, nil
}

// member walks the value of the attribute name, which the RM declares as attr.
// The second result is false when the expected form drops the member.
func (w walker) member(name string, attr rmjsonwalk.Attribute, val any, ptr string) (any, bool, error) {
	if attr.Payload {
		return val, true, nil // the decoder holds a generic payload as plain JSON
	}
	if attr.Kind == rmjsonwalk.KindParameter {
		// No binding: the value must carry its own _type, which resolve
		// requires. A JSON primitive is kept as written.
		if _, isObj := val.(map[string]any); !isObj {
			return val, true, nil
		}
	}
	if !attr.Container {
		return w.single(attr.Type, attr.Kind, attr.Bound, val, ptr)
	}
	if attr.Type == "Octet" {
		// A byte list written as one base64 string. An empty DV_MULTIMEDIA
		// data or integrity_check is dropped on re-encode.
		if w.full && attr.DeclaredOn == "DV_MULTIMEDIA" && isEmptyBytes(val) {
			return nil, false, nil
		}
		return val, true, nil
	}
	list, ok := val.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s: want a JSON array for a container attribute", ptr)
	}
	if w.full && len(list) == 0 && name == "mappings" && attr.DeclaredOn == "DV_TEXT" {
		return nil, false, nil
	}
	out := make([]any, len(list))
	for i, e := range list {
		nv, _, err := w.single(attr.Type, attr.Kind, attr.Bound, e, ptr+"/"+strconv.Itoa(i))
		if err != nil {
			return nil, false, err
		}
		out[i] = nv
	}
	return out, true, nil
}

// single walks one value of declared type attrType, a member's value or one
// element of a container. attrType is empty for a type parameter with no
// binding, where the value's own "_type" decides.
func (w walker) single(attrType string, k rmjsonwalk.Kind, boundType string, val any, ptr string) (any, bool, error) {
	if k == rmjsonwalk.KindPrimitive {
		if w.full {
			return normaliseNumber(attrType, val), true, nil
		}
		return val, true, nil
	}
	obj, ok := val.(map[string]any)
	if !ok {
		if val == nil {
			return nil, true, nil // a null container element is kept as written
		}
		return nil, false, fmt.Errorf("%s: want a JSON object for an RM class", ptr)
	}
	nv, err := w.object(obj, attrType, boundType, ptr)
	if err != nil {
		return nil, false, err
	}
	return nv, true, nil
}

// isInterval reports whether rmType carries all four boundary flags.
func (w walker) isInterval(rmType string) bool {
	for _, f := range intervalFlags {
		if _, ok := w.lookup.AttributeRMType(rmType, f); !ok {
			return false
		}
	}
	return true
}

// settleInterval writes each omitted boundary flag with its default and drops
// an empty bound on a side marked unbounded. A flag present in the input is
// kept as written, an included open side among them. boundType is the
// concrete class the slot binds the bound to, or empty (see isEmptyBound).
func (w walker) settleInterval(obj map[string]any, boundType string) {
	for _, side := range [...]string{"lower", "upper"} {
		open := false
		if v, ok := obj[side+"_unbounded"]; ok {
			open = v == true
		} else {
			obj[side+"_unbounded"] = false
		}
		if _, ok := obj[side+"_included"]; !ok {
			obj[side+"_included"] = !open
		}
		if bound, ok := obj[side]; ok && open && w.isEmptyBound(bound, boundType) {
			delete(obj, side)
		}
	}
}

// isEmptyBound reports whether an interval bound is empty in the encoder's
// sense: nil, or the zero value of a concrete bound type. In JSON that is
// null, the zero literal of a primitive bound, or, where the slot binds the
// bound to the concrete class boundType, an object in which every member other
// than "_type" is a mandatory attribute of that class holding a zero literal.
// The SDK holds such a bound by value, its mandatory attributes as values and
// its optional ones behind pointers, so an optional member present, even one
// holding a zero literal, makes the bound non-empty. The mandatory attributes
// of every concrete bound type the RM binds (DV_QUANTITY, DV_COUNT,
// DV_PROPORTION, DV_DATE_TIME, DV_DATE) are all primitives. A bound behind an
// unbound or abstract parameter, boundType empty, is held by reference, so an
// object there is never empty.
func (w walker) isEmptyBound(v any, boundType string) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		if boundType == "" {
			return false
		}
		rmType := boundType
		if t, ok := x["_type"].(string); ok {
			rmType = t
		}
		mandatory := w.lookup.RequiredAttributes(rmType)
		for name, m := range x {
			if name == "_type" {
				continue
			}
			if !slices.Contains(mandatory, name) || !isZeroLiteral(m) {
				return false
			}
		}
		return true
	default:
		return isZeroLiteral(v)
	}
}

// isZeroLiteral reports whether v is the JSON spelling of a primitive zero
// value: 0, the empty string or false.
func isZeroLiteral(v any) bool {
	switch x := v.(type) {
	case json.Number:
		f, err := strconv.ParseFloat(x.String(), 64)
		return err == nil && f == 0
	case string:
		return x == ""
	case bool:
		return !x
	}
	return false
}

// isEmptyBytes reports whether a byte-list value is empty: the empty base64
// string, or an empty JSON array.
func isEmptyBytes(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// normaliseNumber replaces a number held by a numeric primitive attribute,
// written bare or quoted, with the literal the encoder writes for its value: a
// Real or Double as encoding/json formats a float64, an Integer or Integer64 in
// decimal. A value that is not a number of that kind is returned unchanged, so
// the difference shows when the output is compared.
func normaliseNumber(attrType string, v any) any {
	var lit string
	switch x := v.(type) {
	case json.Number:
		lit = x.String()
	case string:
		lit = x
	default:
		return v
	}
	switch attrType {
	case "Real", "Double":
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return v
		}
		b, err := json.Marshal(f)
		if err != nil {
			return v
		}
		return json.Number(b)
	case "Integer", "Integer64":
		bits := 64
		if attrType == "Integer" {
			bits = 32
		}
		n, err := strconv.ParseInt(lit, 10, bits)
		if err != nil {
			return v
		}
		return json.Number(strconv.FormatInt(n, 10))
	}
	return v
}

// pointerOrRoot renders the empty pointer (the whole document) readably.
func pointerOrRoot(ptr string) string {
	if ptr == "" {
		return "(document root)"
	}
	return ptr
}

// escapeToken escapes one member name as an RFC 6901 reference token.
func escapeToken(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}
