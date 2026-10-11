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
//   - a member whose value is null is removed;
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
// [Prune] applies the first change only. A caller counting what the input
// carries on the members the RM defines uses it.
//
// Attribute names, attribute types and the class graph come from
// [rminfo.Default]. Member order is not part of either result: compare with
// [wireequiv.Equivalent].
//
// The package reads its input with the v1 encoding/json package and keeps every
// number as its literal text, for the reason testkit/wireequiv gives: it is an
// independent reader of the wire bytes, not the codec under test.
package expectedform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// Build returns the expected form of input, a canonical-JSON document whose
// root decodes into the RM type rootType. The package comment lists the changes
// it makes. An error means the input cannot be read against the RM: it is not
// a single JSON object, an object carries a "_type" no RM class has, an object
// leaves out "_type" where the declared type is abstract or a type parameter
// with no binding, or a member's value has the wrong JSON kind for its
// attribute. The decoder refuses such inputs too, so an error here never hides
// a difference.
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

// boundBindings binds the type parameter of a DV_INTERVAL slot whose RM type
// argument is a concrete class, keyed by the class that declares the slot and
// the slot's name. rminfo reports the interval's bounds by the parameter's name
// ("T") and does not carry the binding, so it is written out here from the
// openEHR RM 1.2 class definitions. A slot not listed binds the parameter to an
// abstract class (DV_ORDERED) or leaves it open, so a bound there must carry
// "_type". TestBoundBindingsMatchTheRM checks the table against rminfo and the
// generated rm types, in both directions.
var boundBindings = map[string]string{
	"DV_QUANTITY.normal_range":         "DV_QUANTITY",
	"DV_COUNT.normal_range":            "DV_COUNT",
	"DV_PROPORTION.normal_range":       "DV_PROPORTION",
	"PARTICIPATION.time":               "DV_DATE_TIME",
	"CAPABILITY.time_validity":         "DV_DATE",
	"CONTACT.time_validity":            "DV_DATE",
	"PARTY_RELATIONSHIP.time_validity": "DV_DATE",
	"ROLE.time_validity":               "DV_DATE",
}

// intervalFlags are the four boundary flags BASE Interval declares. A class
// that carries all four is read as an interval.
var intervalFlags = [...]string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"}

// transform decodes input, walks it from the root and encodes the result.
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

// walker carries what every step of the walk needs: the RM answers and full,
// which selects Build over Prune.
type walker struct {
	lookup  rminfo.Lookup
	hier    rminfo.Hierarchy
	absence rminfo.AbsenceReporter
	full    bool
}

// newWalker asserts the optional rminfo capabilities the walk needs, so a
// Lookup without them fails here rather than misreading a type.
func newWalker(l rminfo.Lookup, full bool) (walker, error) {
	h, ok := l.(rminfo.Hierarchy)
	if !ok {
		return walker{}, errors.New("expectedform: the RM lookup does not implement rminfo.Hierarchy")
	}
	a, ok := l.(rminfo.AbsenceReporter)
	if !ok {
		return walker{}, errors.New("expectedform: the RM lookup does not implement rminfo.AbsenceReporter")
	}
	return walker{lookup: l, hier: h, absence: a, full: full}, nil
}

// kind classifies an attribute's declared type name.
type kind int

const (
	kindClass     kind = iota // an RM class in the universe
	kindPrimitive             // a BMM primitive: String, Integer, Real, Octet, Hash, …
	kindParameter             // a generic type parameter, such as "T"
)

// kindOf classifies an attribute's declared type name.
func (w walker) kindOf(name string) kind {
	if _, known := w.hier.IsAbstract(name); known {
		return kindClass
	}
	if w.absence.AbsenceReason(name) == rminfo.AbsencePrimitive {
		return kindPrimitive
	}
	return kindParameter
}

// object walks one JSON object whose declared RM type is declared. boundType
// binds the type parameter of the object's own attributes when the slot that
// holds it binds one (see boundBindings), and is empty otherwise.
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
		attrType, ok := w.lookup.AttributeRMType(rmType, name)
		if !ok {
			continue // names no attribute of rmType
		}
		if val == nil {
			if !w.full {
				out[name] = val
			}
			continue
		}
		container, _ := w.lookup.IsContainer(rmType, name)
		child := ptr + "/" + escapeToken(name)
		nv, keep, err := w.member(rmType, name, attrType, container, boundType, val, child)
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
			settleInterval(out, boundType != "")
		}
	}
	return out, nil
}

// resolve returns obj's RM type: its own "_type" when it carries one, else the
// declared type of the slot that holds it.
func (w walker) resolve(obj map[string]any, declared, ptr string) (string, error) {
	switch t := obj["_type"].(type) {
	case string:
		if _, known := w.hier.IsAbstract(t); !known {
			return "", fmt.Errorf("%s: _type %q names no RM class", pointerOrRoot(ptr), t)
		}
		return t, nil
	case nil:
		// Absent, or null: the declared type applies.
	default:
		return "", fmt.Errorf("%s: _type is not a string", pointerOrRoot(ptr))
	}
	if declared == "" {
		return "", fmt.Errorf("%s: _type is elided where the RM declares a type parameter with no binding", pointerOrRoot(ptr))
	}
	abstract, known := w.hier.IsAbstract(declared)
	if !known {
		return "", fmt.Errorf("%s: _type is elided and the declared type %q is not an RM class", pointerOrRoot(ptr), declared)
	}
	if abstract {
		return "", fmt.Errorf("%s: _type is elided where the declared type %s is abstract", pointerOrRoot(ptr), declared)
	}
	return declared, nil
}

// member walks the value of attribute name on rmType. The second result is
// false when the expected form drops the member.
func (w walker) member(rmType, name, attrType string, container bool, boundType string, val any, ptr string) (any, bool, error) {
	k := w.kindOf(attrType)
	if k == kindParameter {
		attrType, k = boundType, kindClass
		if attrType == "" {
			// No binding: the value must carry its own _type, which
			// resolve requires. A JSON primitive is kept as written.
			if _, isObj := val.(map[string]any); !isObj {
				return val, true, nil
			}
		}
	}
	childBound := ""
	if site, ok := w.hier.DeclaredOn(rmType, name); ok {
		childBound = boundBindings[site+"."+name]
	}
	if !container {
		return w.single(attrType, k, childBound, val, ptr)
	}
	if attrType == "Octet" {
		// DV_MULTIMEDIA.data and integrity_check: a byte list written as one
		// base64 string. An empty one is dropped on re-encode.
		if w.full && isEmptyBytes(val) {
			return nil, false, nil
		}
		return val, true, nil
	}
	list, ok := val.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s: want a JSON array for a container attribute", ptr)
	}
	if w.full && len(list) == 0 && name == "mappings" && w.declaredOn(rmType, name) == "DV_TEXT" {
		return nil, false, nil
	}
	out := make([]any, len(list))
	for i, e := range list {
		nv, _, err := w.single(attrType, k, childBound, e, ptr+"/"+strconv.Itoa(i))
		if err != nil {
			return nil, false, err
		}
		out[i] = nv
	}
	return out, true, nil
}

// single walks one value of declared type attrType, a member's value or one
// element of a container.
func (w walker) single(attrType string, k kind, boundType string, val any, ptr string) (any, bool, error) {
	if k == kindPrimitive {
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

// declaredOn returns the class that declares attribute name on rmType, or the
// empty string.
func (w walker) declaredOn(rmType, name string) string {
	site, _ := w.hier.DeclaredOn(rmType, name)
	return site
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
// kept as written, an included open side among them. concreteBound is true
// when the slot binds the bound to a concrete class, which the SDK holds by
// value, so a bound carrying only zero values is empty there.
func settleInterval(obj map[string]any, concreteBound bool) {
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
		if bound, ok := obj[side]; ok && open && isEmptyBound(bound, concreteBound) {
			delete(obj, side)
		}
	}
}

// isEmptyBound reports whether an interval bound is empty: null, the zero
// literal of a primitive bound, or, where the bound type is a concrete class,
// an object whose every member other than "_type" holds a zero literal. That is
// the zero value of every concrete bound type the RM binds (DV_QUANTITY,
// DV_COUNT, DV_PROPORTION, DV_DATE_TIME, DV_DATE), whose mandatory attributes
// are all primitives. A bound behind an unbound or abstract parameter is held
// by reference, so an object there is never empty.
func isEmptyBound(v any, concreteBound bool) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		if !concreteBound {
			return false
		}
		for name, m := range x {
			if name != "_type" && !isZeroLiteral(m) {
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
