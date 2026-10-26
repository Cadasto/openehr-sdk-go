package canxml_test

// primitive_error_test.go: a primitive element whose text does not decode as
// its type, such as a Boolean written `maybe` (REQ-052 § Value-free decode
// errors, REQ-056). The decode fails with a *canxml.DecodeError that wraps
// canxml.ErrInvalidShape, keeps the conversion's own error reachable and
// carries none of the element's text. Unmarshal and a Decoder given an option
// name the element by its path from the root (REQ-170's path form), on its
// own; a Decoder without options names the element alone, inside the slot
// envelopes it passed through.

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// primitiveCase is a document holding one primitive element whose text does
// not decode as the element's type.
type primitiveCase struct {
	name string
	in   string
	into func() any
	// path is the element's path from the root of the value; alone is the
	// element's name alone, as a Decoder without options writes it.
	path, alone string
	// slot is the text of the slot envelopes a Decoder without options writes
	// before the failure, and "" for an element outside a slot.
	slot string
	// cause is the conversion's refusal as the DecodeError writes it, and raw
	// its own text, which may name the value.
	cause, raw string
	// value is the element's text, which no error text may carry.
	value string
	// reach reports whether err keeps the conversion's typed error reachable.
	reach func(error) bool
}

// isNumError reports whether err's chain holds a *strconv.NumError.
func isNumError(err error) bool {
	ne, ok := errors.AsType[*strconv.NumError](err)
	return ok && ne != nil
}

// isBase64Error reports whether err's chain holds a base64 refusal.
func isBase64Error(err error) bool {
	_, ok := errors.AsType[base64.CorruptInputError](err)
	return ok
}

// isTimeParseError reports whether err's chain holds a *time.ParseError.
func isTimeParseError(err error) bool {
	pe, ok := errors.AsType[*time.ParseError](err)
	return ok && pe != nil
}

// anyError holds for every error: the cause has no type of its own to reach,
// only its text.
func anyError(error) bool { return true }

// primitiveCases covers each primitive kind the RM decodes from text, outside
// a polymorphic slot and inside one, a Character refused through its
// UnmarshalText, an Octet list that is not base64, an AOM 1.4 interval bound,
// a bound held in an interface-typed member, a bound typed as a pointer to a
// TextUnmarshaler, and an interval boundary flag that is no boolean.
func primitiveCases(t *testing.T) []primitiveCase {
	t.Helper()
	st := &rm.EHRStatus{
		ArchetypeNodeID: "openEHR-EHR-EHR_STATUS.generic.v1",
		Name:            &rm.DVText{Value: "EHR Status"},
		IsModifiable:    true,
		IsQueryable:     true,
	}
	doc, err := canxml.Marshal(st)
	if err != nil {
		t.Fatalf("canxml.Marshal(EHR_STATUS) = %v", err)
	}
	const queryable = "<is_queryable>true</is_queryable>"
	if !strings.Contains(string(doc), queryable) {
		t.Fatalf("canxml.Marshal(EHR_STATUS) wrote\n%s\nwith no %s", doc, queryable)
	}
	ehrStatus := strings.Replace(string(doc), queryable, "<is_queryable>maybe</is_queryable>", 1)

	const media = `<media_type><terminology_id><value>IANA_media-types</value></terminology_id><code_string>text/plain</code_string></media_type>`
	const target = `<target><terminology_id><value>SNOMED-CT</value></terminology_id><code_string>371532007</code_string></target>`
	return []primitiveCase{
		{
			name: "Boolean", in: ehrStatus, into: func() any { return new(rm.EHRStatus) },
			path: "/ehr_status[1]/is_queryable[1]", alone: "/is_queryable",
			cause: "strconv.ParseBool: invalid syntax", raw: `strconv.ParseBool: parsing "maybe": invalid syntax`,
			value: "maybe", reach: isNumError,
		},
		{
			name: "Integer",
			in:   `<dv_multimedia ` + ucNS + `>` + media + `<size>7777x</size></dv_multimedia>`,
			into: func() any { return new(rm.DVMultimedia) },
			path: "/dv_multimedia[1]/size[1]", alone: "/size",
			cause: "strconv.ParseInt: invalid syntax", raw: `strconv.ParseInt: parsing "7777x": invalid syntax`,
			value: "7777x", reach: isNumError,
		},
		{
			name: "optional Integer out of range",
			in:   `<dv_quantity ` + ucNS + `><magnitude>1</magnitude><units>mm</units><precision>2147483648</precision></dv_quantity>`,
			into: func() any { return new(rm.DVQuantity) },
			path: "/dv_quantity[1]/precision[1]", alone: "/precision",
			cause: "strconv.ParseInt: value out of range", raw: `strconv.ParseInt: parsing "2147483648": value out of range`,
			value: "2147483648", reach: isNumError,
		},
		{
			name: "Integer64",
			in:   `<dv_count ` + ucNS + `><magnitude>98765.4321</magnitude></dv_count>`,
			into: func() any { return new(rm.DVCount) },
			path: "/dv_count[1]/magnitude[1]", alone: "/magnitude",
			cause: "strconv.ParseInt: invalid syntax", raw: `strconv.ParseInt: parsing "98765.4321": invalid syntax`,
			value: "98765.4321", reach: isNumError,
		},
		{
			name: "Real",
			in:   `<dv_quantity ` + ucNS + `><magnitude>7777kg</magnitude><units>kg</units></dv_quantity>`,
			into: func() any { return new(rm.DVQuantity) },
			path: "/dv_quantity[1]/magnitude[1]", alone: "/magnitude",
			cause: "strconv.ParseFloat: invalid syntax", raw: `strconv.ParseFloat: parsing "7777kg": invalid syntax`,
			value: "7777kg", reach: isNumError,
		},
		{
			name: "Real in a slot",
			in:   slotElement("DV_QUANTITY", `<magnitude>7777kg</magnitude><units>kg</units>`),
			into: func() any { return new(rm.Element) },
			path: "/element[1]/value[1]/magnitude[1]", alone: "/magnitude", slot: `decode /value (_type="DV_QUANTITY"): `,
			cause: "strconv.ParseFloat: invalid syntax", raw: `strconv.ParseFloat: parsing "7777kg": invalid syntax`,
			value: "7777kg", reach: isNumError,
		},
		{
			name: "Integer64 in a slot",
			in:   slotElement("DV_COUNT", `<magnitude>98765.4321</magnitude>`),
			into: func() any { return new(rm.Element) },
			path: "/element[1]/value[1]/magnitude[1]", alone: "/magnitude", slot: `decode /value (_type="DV_COUNT"): `,
			cause: "strconv.ParseInt: invalid syntax", raw: `strconv.ParseInt: parsing "98765.4321": invalid syntax`,
			value: "98765.4321", reach: isNumError,
		},
		{
			name: "Character",
			in:   `<term_mapping ` + ucNS + `><match>Jane</match>` + target + `</term_mapping>`,
			into: func() any { return new(rm.TermMapping) },
			path: "/term_mapping[1]/match[1]", alone: "/match",
			cause: "rm.Character: must be exactly one character, got 4", raw: "rm.Character: must be exactly one character, got 4",
			value: "Jane", reach: anyError,
		},
		{
			name: "Octet list not base64",
			in:   `<dv_multimedia ` + ucNS + `><data>Jane@7777</data>` + media + `<size>5</size></dv_multimedia>`,
			into: func() any { return new(rm.DVMultimedia) },
			path: "/dv_multimedia[1]/data[1]", alone: "/data",
			cause: "illegal base64 data at input byte 4", raw: "illegal base64 data at input byte 4",
			value: "Jane@7777", reach: isBase64Error,
		},
		{
			name: "AOM 1.4 interval bound",
			in: `<c_complex_object ` + ucNS + `><rm_type_name>OBSERVATION</rm_type_name><occurrences><lower>one7777</lower><upper>1</upper></occurrences>` +
				`<node_id>at0000</node_id></c_complex_object>`,
			into: func() any { return new(aom14.CComplexObject) },
			path: "/c_complex_object[1]/occurrences[1]/lower[1]", alone: "/lower",
			cause: "strconv.ParseInt: invalid syntax", raw: `strconv.ParseInt: parsing "one7777": invalid syntax`,
			value: "one7777", reach: isNumError,
		},
		{
			name: "Integer held in an interface-typed bound",
			in:   `<iv ` + ucNS + `><lower>7777x</lower></iv>`,
			into: func() any { return &rm.Interval[any]{Lower: new(rm.Integer)} },
			path: "/iv[1]/lower[1]", alone: "/lower",
			cause: "strconv.ParseInt: invalid syntax", raw: `strconv.ParseInt: parsing "7777x": invalid syntax`,
			value: "7777x", reach: isNumError,
		},
		{
			name: "Character held in an interface-typed bound",
			in:   `<iv ` + ucNS + `><lower>Jane</lower></iv>`,
			into: func() any { return &rm.Interval[any]{Lower: new(rm.Character)} },
			path: "/iv[1]/lower[1]", alone: "/lower",
			cause: "rm.Character: must be exactly one character, got 4", raw: "rm.Character: must be exactly one character, got 4",
			value: "Jane", reach: anyError,
		},
		{
			name: "Character behind a pointer-typed bound",
			in:   `<iv ` + ucNS + `><lower>Jane</lower></iv>`,
			into: func() any { return new(rm.Interval[*rm.Character]) },
			path: "/iv[1]/lower[1]", alone: "/lower",
			cause: "rm.Character: must be exactly one character, got 4", raw: "rm.Character: must be exactly one character, got 4",
			value: "Jane", reach: anyError,
		},
		{
			name: "time behind a pointer-typed bound",
			in:   `<iv ` + ucNS + `><lower>Jane-7777</lower></iv>`,
			into: func() any { return new(rm.Interval[*time.Time]) },
			path: "/iv[1]/lower[1]", alone: "/lower",
			cause: "cause withheld (*time.ParseError)", raw: `parsing time "Jane-7777" as "2006-01-02T15:04:05Z07:00": cannot parse "Jane-7777" as "2006"`,
			value: "Jane-7777", reach: isTimeParseError,
		},
		{
			name: "interval flag of a DV_COUNT normal range",
			in: `<dv_count ` + ucNS + `><normal_range><lower xsi:type="DV_COUNT"><magnitude>1</magnitude></lower>` +
				`<lower_included>yes7777</lower_included></normal_range><magnitude>5</magnitude></dv_count>`,
			into: func() any { return new(rm.DVCount) },
			path: "/dv_count[1]/normal_range[1]/lower_included[1]", alone: "/lower_included",
			cause: "strconv.ParseBool: invalid syntax", raw: `strconv.ParseBool: parsing "yes7777": invalid syntax`,
			value: "yes7777", reach: isNumError,
		},
		{
			name: "interval flag of a DV_INTERVAL in a slot",
			in: slotElement("DV_INTERVAL", `<lower xsi:type="DV_COUNT"><magnitude>1</magnitude></lower>`+
				`<upper xsi:type="DV_COUNT"><magnitude>5</magnitude></upper><upper_included>yes7777</upper_included>`),
			into: func() any { return new(rm.Element) },
			path: "/element[1]/value[1]/upper_included[1]", alone: "/upper_included", slot: `decode /value (_type="DV_INTERVAL"): `,
			cause: "strconv.ParseBool: invalid syntax", raw: `strconv.ParseBool: parsing "yes7777": invalid syntax`,
			value: "yes7777", reach: isNumError,
		},
		{
			name: "interval flag of AOM 1.4 occurrences",
			in: `<c_complex_object ` + ucNS + `><rm_type_name>OBSERVATION</rm_type_name><occurrences><lower>1</lower><upper>1</upper>` +
				`<lower_unbounded>yes7777</lower_unbounded></occurrences><node_id>at0000</node_id></c_complex_object>`,
			into: func() any { return new(aom14.CComplexObject) },
			path: "/c_complex_object[1]/occurrences[1]/lower_unbounded[1]", alone: "/lower_unbounded",
			cause: "strconv.ParseBool: invalid syntax", raw: `strconv.ParseBool: parsing "yes7777": invalid syntax`,
			value: "yes7777", reach: isNumError,
		},
		{
			name: "interval flag of a Go-typed Interval",
			in:   `<iv ` + ucNS + `><lower>1</lower><upper_unbounded>yes7777</upper_unbounded></iv>`,
			into: func() any { return new(rm.Interval[int]) },
			path: "/iv[1]/upper_unbounded[1]", alone: "/upper_unbounded",
			cause: "strconv.ParseBool: invalid syntax", raw: `strconv.ParseBool: parsing "yes7777": invalid syntax`,
			value: "yes7777", reach: isNumError,
		},
	}
}

// reaches reports whether err's tree holds an error whose own text is want.
func reaches(err error, want string) bool {
	if err == nil {
		return false
	}
	if err.Error() == want {
		return true
	}
	switch u := err.(type) { //nolint:errorlint // each link of the tree is read in turn
	case interface{ Unwrap() error }:
		return reaches(u.Unwrap(), want)
	case interface{ Unwrap() []error }:
		for _, e := range u.Unwrap() {
			if reaches(e, want) {
				return true
			}
		}
	}
	return false
}

// assertPrimitiveFailure checks every facet of a primitive failure: err is
// itself a *canxml.DecodeError; its text is slot, then the element's path or
// name, then canxml's shape sentinel and the conversion's refusal, with none
// of the element's text; outside a slot it names the element in Path and no
// type; it wraps canxml.ErrInvalidShape and not canxml.ErrUnknownElement;
// the conversion's own error stays reachable; and Value holds nothing, since
// canonical XML keeps no literal.
func assertPrimitiveFailure(t *testing.T, call string, err error, tc primitiveCase, path, slot string) {
	t.Helper()
	de, ok := errors.AsType[*canxml.DecodeError](err)
	if !ok || de == nil {
		t.Fatalf("%s err = %T %v; want a *canxml.DecodeError", call, err, err)
	}
	if error(de) != err { //nolint:errorlint // identity: the DecodeError must be err itself, not one it wraps
		t.Errorf("%s err = %T %v; want the *canxml.DecodeError itself, not wrapped", call, err, err)
	}
	if want := slot + "decode " + path + ": canxml: invalid XML shape: " + tc.cause; err.Error() != want {
		t.Errorf("%s err = %q\nwant %q", call, err.Error(), want)
	}
	if slot == "" && (de.Path != path || de.Type != "") {
		t.Errorf("%s DecodeError{Path: %q, Type: %q}; want {Path: %q, Type: \"\"}", call, de.Path, de.Type, path)
	}
	if strings.Contains(err.Error(), tc.value) {
		t.Errorf("%s err = %q; it carries the element's text %q", call, err, tc.value)
	}
	if !errors.Is(err, canxml.ErrInvalidShape) {
		t.Errorf("%s err = %v; want errors.Is(_, canxml.ErrInvalidShape)", call, err)
	}
	if errors.Is(err, canxml.ErrUnknownElement) {
		t.Errorf("%s err = %v; a primitive failure is no unmatched content, want no canxml.ErrUnknownElement", call, err)
	}
	if !reaches(err, tc.raw) || !tc.reach(err) {
		t.Errorf("%s err = %v; want the conversion's own error %q reachable through unwrapping", call, err, tc.raw)
	}
	if got := de.Value.Reveal(); got != nil {
		t.Errorf("%s DecodeError.Value.Reveal() = %#v; want nothing: canonical XML holds no literal", call, got)
	}
}

// TestREQ052_XMLPrimitiveFailureIsAValueFreeDecodeError pins issue 314: the
// text of a primitive element that does not decode as its type fails the
// decode with a *canxml.DecodeError, never the conversion's bare error, whose
// text quotes the value. Unmarshal, in every mode, and a Decoder given an
// option name the element by its path from the root, on its own, even inside
// a polymorphic slot. A Decoder without options cannot read the stream again,
// so it names the element alone, inside the envelope of each slot it passed
// through.
//
// Can-fail control: return the conversion's error as encoding/xml gives it
// and every row fails; drop the second pass Unmarshal makes on a primitive
// failure and its default rows name the element alone; return a tracked
// decode's failure inside the slot envelopes and the slot rows of Unmarshal
// and the Decoder with options name the slot.
func TestREQ052_XMLPrimitiveFailureIsAValueFreeDecodeError(t *testing.T) {
	tracked := map[string][]canxml.DecoderOption{
		"WithUnmatched": {canxml.WithUnmatched(func(string) {})},
		"WithStrict":    {canxml.WithStrict()},
	}
	for _, tc := range primitiveCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			for mode, opts := range xmlDecodeModes() {
				err := canxml.Unmarshal([]byte(tc.in), tc.into(), opts...)
				assertPrimitiveFailure(t, "canxml.Unmarshal("+mode+")", err, tc, tc.path, "")
			}
			for mode, opts := range tracked {
				err := canxml.NewDecoder(strings.NewReader(tc.in), opts...).Decode(tc.into())
				assertPrimitiveFailure(t, "canxml.Decoder("+mode+").Decode", err, tc, tc.path, "")
			}
			err := canxml.NewDecoder(strings.NewReader(tc.in)).Decode(tc.into())
			assertPrimitiveFailure(t, "canxml.Decoder(default).Decode", err, tc, tc.alone, tc.slot)
		})
	}
}

// deepPrimitiveTree returns an ITEM_TREE holding depth CLUSTER items, each
// inside the one before, with an ELEMENT innermost whose DV_COUNT magnitude
// is no whole number, and the path of that magnitude.
func deepPrimitiveTree(depth int) ([]byte, string) {
	var b, path strings.Builder
	b.WriteString(`<item_tree ` + ucNS + ` archetype_node_id="at0">`)
	path.WriteString("/item_tree[1]")
	for range depth {
		b.WriteString(`<items xsi:type="CLUSTER" archetype_node_id="at1">`)
		path.WriteString("/items[1]")
	}
	b.WriteString(`<items xsi:type="ELEMENT" archetype_node_id="at2"><value xsi:type="DV_COUNT"><magnitude>98765.4321</magnitude></value></items>`)
	path.WriteString("/items[1]/value[1]/magnitude[1]")
	for range depth {
		b.WriteString(`</items>`)
	}
	b.WriteString(`</item_tree>`)
	return []byte(b.String()), path.String()
}

// TestREQ056_XMLDeepPrimitiveFailureCostsLinearly pins the cost of a
// primitive failure deep inside nested polymorphic slots, which Unmarshal
// decodes a second time to name the element by its full path: the error,
// built and rendered, costs memory in proportion to the depth, as the deep
// dispatch failure does. The second pass is one more linear pass, never one
// per slot.
//
// Can-fail control: decode once more per slot on the way out, or format the
// failure below at every slot, and the bytes allocated grow with the square
// of the depth.
func TestREQ056_XMLDeepPrimitiveFailureCostsLinearly(t *testing.T) {
	const depth = 1500
	inN, pathN := deepPrimitiveTree(depth)
	in2N, path2N := deepPrimitiveTree(2 * depth)
	msgN, allocN, errN := decodeDeep(t, inN)
	msg2N, alloc2N, err2N := decodeDeep(t, in2N)

	for _, tc := range []struct {
		depth int
		path  string
		err   error
	}{{depth, pathN, errN}, {2 * depth, path2N, err2N}} {
		de, ok := errors.AsType[*canxml.DecodeError](tc.err)
		if !ok || de == nil || de.Path != tc.path {
			t.Fatalf("canxml.Unmarshal(depth %d) err = %.200v; want a *canxml.DecodeError naming the magnitude by its full path", tc.depth, tc.err)
		}
		if !errors.Is(tc.err, canxml.ErrInvalidShape) {
			t.Errorf("canxml.Unmarshal(depth %d) err = %.200v; want errors.Is(_, canxml.ErrInvalidShape)", tc.depth, tc.err)
		}
	}
	if !strings.HasSuffix(msg2N, "/magnitude[1]: canxml: invalid XML shape: strconv.ParseInt: invalid syntax") {
		t.Errorf("canxml.Unmarshal(depth %d) message ends %q; want the magnitude and the refusal", 2*depth, msg2N[max(0, len(msg2N)-120):])
	}
	if len(msg2N) > 3*len(msgN) {
		t.Errorf("message length: depth %d gives %d bytes, depth %d gives %d; want it to grow with the depth", depth, len(msgN), 2*depth, len(msg2N))
	}
	if alloc2N > 3*allocN {
		t.Errorf("bytes allocated: depth %d costs %d, depth %d costs %d (%.1fx); want at most 3x: the cost grows with the square of the depth",
			depth, allocN, 2*depth, alloc2N, float64(alloc2N)/float64(allocN))
	}
	t.Logf("depth %d: %d bytes allocated, message %d bytes; depth %d: %d bytes allocated, message %d bytes",
		depth, allocN, len(msgN), 2*depth, alloc2N, len(msg2N))
}
