package canjson_test

import (
	"bytes"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestDecodeNullEqualsAbsent — REQ-052 documents that the codec
// treats `null` and ABSENT as equivalent on decode and emits ABSENT
// on encode. Verified by feeding a payload with `"precision": null`
// and asserting (a) decode succeeds, (b) the resulting struct has
// nil there, (c) re-encoding emits no `precision` key.
func TestDecodeNullEqualsAbsent(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":1.0,"units":"kg","precision":null,"accuracy":null}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if q.Precision != nil {
		t.Errorf("Precision = %v; want nil for null input", *q.Precision)
	}
	if q.Accuracy != nil {
		t.Errorf("Accuracy = %v; want nil for null input", *q.Accuracy)
	}
	b, err := canjson.Marshal(&q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), `"precision"`) || strings.Contains(string(b), `"accuracy"`) {
		t.Errorf("re-encode must omit nil-pointer optional fields: %s", b)
	}
}

// TestISO8601PassthroughOnString — the codec MUST NOT parse ISO
// 8601 strings into time.Time at the codec layer (REQ-046). String
// values reach the typed helpers (*_ext.go) untouched.
func TestISO8601PassthroughOnString(t *testing.T) {
	in := []byte(`{"_type":"DV_DATE_TIME","value":"2026-05-16T12:34:56.789+02:00"}`)
	var d rm.DVDateTime
	if err := canjson.Unmarshal(in, &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Value != "2026-05-16T12:34:56.789+02:00" {
		t.Errorf("Value = %q; want passthrough of original string", d.Value)
	}
	b, err := canjson.Marshal(&d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"2026-05-16T12:34:56.789+02:00"`) {
		t.Errorf("re-encode must preserve ISO 8601 string verbatim: %s", b)
	}
}

// TestEmptyContainerEncodesAbsent — REQ-052: BMM container
// properties with cardinality.lower == 0 emit ABSENT, not `[]`.
// The complement (decode of a fixture that omits the container leaves
// the slice nil) is also asserted to keep null-vs-absent symmetric.
func TestEmptyContainerEncodesAbsent(t *testing.T) {
	in := []byte(`{
		"_type": "COMPOSITION",
		"archetype_node_id": "x",
		"name": {"_type":"DV_TEXT","value":"x"},
		"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
		"territory": {"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
		"category": {"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
		"composer": {"_type":"PARTY_SELF"}
	}`)
	var c rm.Composition
	if err := canjson.Unmarshal(in, &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if c.Content != nil {
		t.Errorf("Content = %v; want nil when input omits the key", c.Content)
	}
	if c.Links != nil {
		t.Errorf("Links = %v; want nil when input omits the key", c.Links)
	}
	b, err := canjson.Marshal(&c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{`"content"`, `"links"`} {
		if strings.Contains(string(b), banned) {
			t.Errorf("output must omit empty container %s: %s", banned, b)
		}
	}
}

// TestDecodeRecursiveFolder — deep FOLDER trees must decode without
// stack overflow at reasonable depths (>= 8). openEHR's directory
// model is unbounded in principle; this guards a representative
// nesting.
func TestDecodeRecursiveFolder(t *testing.T) {
	// Build a JSON tree 10 folders deep.
	tail := `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"leaf"},"archetype_node_id":"at0001"}`
	for range 9 {
		tail = `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"node"},"archetype_node_id":"at0001","folders":[` + tail + `]}`
	}
	var f rm.Folder
	if err := canjson.Unmarshal([]byte(tail), &f); err != nil {
		t.Fatalf("Unmarshal deep folder: %v", err)
	}
	// Walk to the leaf, counting depth.
	depth := 0
	cur := &f
	for cur != nil && len(cur.Folders) > 0 {
		depth++
		cur = &cur.Folders[0]
	}
	if depth != 9 {
		t.Errorf("depth = %d; want 9", depth)
	}
}

// TestUnmarshalMaxDepthExceeded — a FOLDER tree nested past the decode
// depth cap (REQ-108) is refused by canjson.Unmarshal, not decoded. The
// guard lives in the shared generated decode body, so it holds on the
// canjson entry point and not only on typereg.Registry.Decode. This is a
// can-fail test: with the guard removed the over-deep document decodes
// without error and the errors.Is assertion below fails.
func TestUnmarshalMaxDepthExceeded(t *testing.T) {
	// Each FOLDER wrapper adds two bracket levels (the object and its
	// "folders" array), so ~300 wrappers clear the 512 cap with margin.
	tail := `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"leaf"},"archetype_node_id":"at0001"}`
	for range 300 {
		tail = `{"_type":"FOLDER","name":{"_type":"DV_TEXT","value":"node"},"archetype_node_id":"at0001","folders":[` + tail + `]}`
	}
	var f rm.Folder
	err := canjson.Unmarshal([]byte(tail), &f)
	if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
		t.Errorf("Unmarshal over-deep folder: err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded)", err)
	}
	// A depth refusal is not a shape failure: the nested value that trips the
	// cap must not come back wrapped in ErrInvalidShape by the enclosing
	// values' decode.
	if errors.Is(err, canjson.ErrInvalidShape) {
		t.Errorf("Unmarshal over-deep folder: err = %v; want no canjson.ErrInvalidShape on a depth refusal", err)
	}
	// The refusal passes through the enclosing values unchanged, so its text
	// stays one path long. Re-wrapped at every level, the message grows with
	// the square of the depth (tens of kilobytes here) on hostile input.
	if err != nil && len(err.Error()) > 1024 {
		t.Errorf("Unmarshal over-deep folder: error text is %d bytes, want at most 1024 (the depth refusal must not be re-wrapped per level)", len(err.Error()))
	}
}

// bracketDepth returns the maximum object/array nesting depth of a JSON
// document, ignoring brackets inside strings. The depth tests below use it so
// each one documents the number it constructs rather than trusting a comment.
func bracketDepth(data []byte) int {
	depth, deepest := 0, 0
	inStr, esc := false, false
	for _, b := range data {
		if inStr {
			switch {
			case esc:
				esc = false
			case b == '\\':
				esc = true
			case b == '"':
				inStr = false
			}
			continue
		}
		switch b {
		case '"':
			inStr = true
		case '{', '[':
			depth++
			deepest = max(deepest, depth)
		case '}', ']':
			depth--
		}
	}
	return deepest
}

// folderWithClusterTree builds folders FOLDER wrappers (each adds two levels:
// the object and its "folders" array) around an innermost FOLDER whose
// "details" ITEM_TREE holds clusters nested CLUSTERs (each adds two levels: the
// object and its "items" array). The details and every items entry are
// polymorphic slots (ITEM_STRUCTURE, ITEM), so the CLUSTER chain is decoded
// inside buffered slot values rather than on the document's own decoder.
func folderWithClusterTree(folders, clusters int) []byte {
	var tree strings.Builder
	for range clusters {
		tree.WriteString(`{"_type":"CLUSTER","archetype_node_id":"at0002","items":[`)
	}
	tree.WriteString(strings.Repeat(`]}`, clusters))
	doc := `{"_type":"FOLDER","archetype_node_id":"at0001","details":{"_type":"ITEM_TREE","archetype_node_id":"at0003","items":[` + tree.String() + `]}}`
	for range folders {
		doc = `{"_type":"FOLDER","archetype_node_id":"at0001","folders":[` + doc + `]}`
	}
	return []byte(doc)
}

// depthRoutes are the five decode routes a depth test drives: the two canjson
// entry points, bare encoding/json/v2 and bare v1 encoding/json callers driving
// the generated methods, and the registry chokepoint (REQ-108 names all five).
//
// errCap bounds the refusal's text on each route. The canjson and v2 routes
// report one path, so their text stays within 1024 bytes. Bare v1 builds its
// own dotted Go-field path to the failing value (`Folders.Folders.…`, one
// segment per enclosing level), so its text runs to a few kilobytes at this
// depth; 8192 is a generous fixed bound that still catches the quadratic
// per-level re-wrapping the cap exists to prevent (tens of kilobytes here).
//
// countsBrackets marks the registry route: typereg.Default.Decode measures
// every bracket of its buffered input, where the concrete routes count RM
// values where they open, so the boundary test gives it its own pair.
var depthRoutes = []struct {
	name           string
	decode         func([]byte) error
	errCap         int
	countsBrackets bool
}{
	{"canjson.Unmarshal", func(b []byte) error {
		var f rm.Folder
		return canjson.Unmarshal(b, &f)
	}, 1024, false},
	{"canjson.Decoder.Decode", func(b []byte) error {
		var f rm.Folder
		return canjson.NewDecoder(bytes.NewReader(b)).Decode(&f)
	}, 1024, false},
	{"json/v2.Unmarshal", func(b []byte) error {
		var f rm.Folder
		return jsonv2.Unmarshal(b, &f)
	}, 1024, false},
	{"json/v1.Unmarshal", func(b []byte) error {
		var f rm.Folder
		return jsonv1.Unmarshal(b, &f)
	}, 8192, false},
	{"typereg.Default.Decode", func(b []byte) error {
		_, err := typereg.Default.Decode(b)
		return err
	}, 1024, true},
}

// TestUnmarshalMaxDepthAcrossPolymorphicSlots pins the REQ-108 depth bound
// across polymorphic slots. A polymorphic slot is buffered and decoded on a
// fresh decoder, so the concrete values inside it measure their depth from the
// slot, not from the document root. Two chains that each stay under the cap
// (200 FOLDER wrappers, then 200 nested CLUSTERs inside the innermost FOLDER's
// details) together nest well past it; DecodePolymorphic adds the enclosing
// decoder's stack depth to the slot's own nesting, so the whole document is
// refused on every route. A control at about half the depth decodes cleanly.
//
// Can-fail: with the dec.StackDepth() term dropped from DecodePolymorphic's
// depth check, the four concrete routes (canjson.Unmarshal, Decoder.Decode,
// bare json/v2, bare v1) decode the over-deep document without error, so their
// over-deep sub-tests fail; typereg.Default.Decode measures the whole buffer
// and refuses it either way.
func TestUnmarshalMaxDepthAcrossPolymorphicSlots(t *testing.T) {
	deep := folderWithClusterTree(200, 200)
	if d := bracketDepth(deep); d <= 512 {
		t.Fatalf("constructed over-deep document nests %d levels; want more than the 512 cap", d)
	} else {
		t.Logf("over-deep document nests %d levels", d)
	}
	shallow := folderWithClusterTree(100, 100)
	if d := bracketDepth(shallow); d > 512 {
		t.Fatalf("constructed control document nests %d levels; want at most the 512 cap", d)
	}

	for _, route := range depthRoutes {
		t.Run(route.name+"/over-deep", func(t *testing.T) {
			err := route.decode(deep)
			if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
				t.Fatalf("%s(200 folders + 200 clusters) err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded)", route.name, err)
			}
			if errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("%s(200 folders + 200 clusters) err = %v; a depth refusal must not carry canjson.ErrInvalidShape", route.name, err)
			}
			if n := len(err.Error()); n > route.errCap {
				t.Errorf("%s(200 folders + 200 clusters): error text is %d bytes, want at most %d (the depth refusal must not be re-wrapped per level)", route.name, n, route.errCap)
			}
		})
		t.Run(route.name+"/control", func(t *testing.T) {
			if err := route.decode(shallow); err != nil {
				t.Errorf("%s(100 folders + 100 clusters) = %v; want a document under the cap to decode", route.name, err)
			}
		})
	}
}

// TestUnmarshalMaxDepthBoundary pins both exact REQ-108 fences at the 512
// boundary on the concrete routes: the one DecodeInto applies to a concrete
// value, and the one DecodePolymorphic applies to a value reached through a
// polymorphic slot. With n FOLDER wrappers the leaf FOLDER opens at depth 2n+1
// and each object member of the leaf at 2n+2, so 255 wrappers put a leaf
// member at 512 and one level inside it at 513.
//
//   - concrete at 512 decodes: the leaf's archetype_details ARCHETYPED is a
//     concrete field, so DecodeInto fences it (stack depth 511 plus one).
//   - polymorphic at 512 decodes: the leaf's name DV_TEXT sits in a
//     polymorphic slot, so DecodePolymorphic fences it (enclosing depth 511
//     plus the slot's one level).
//   - concrete at 513 refused: 256 wrappers around a nameless leaf put the leaf
//     FOLDER itself at 513, refused by DecodeInto with "(513 > 512)".
//   - polymorphic at 513 refused: the leaf's name DV_CODED_TEXT carries a
//     CODE_PHRASE at 513, reached through the polymorphic slot, so
//     DecodePolymorphic refuses the slot (511 plus its two levels) with
//     "(513 > 512)".
//
// Can-fail (each checked with go test -overlay on a patched copy of
// streaming.go):
//   - DecodeInto's "d > maxDecodeDepth" changed to "d >= maxDecodeDepth"
//     refuses the concrete 512 case with "(512 > 512)".
//   - DecodeInto's "+ 1" changed to "+ 0" lets the concrete 513 case decode;
//     changed to "+ 2" it refuses the concrete 512 case, and reports the
//     concrete 513 case as "(514 > 512)" instead of the pinned "(513 > 512)".
//   - DecodePolymorphic's "jsonNestingDepth(raw)" changed to
//     "jsonNestingDepth(raw) - 1" lets the polymorphic 513 case decode.
//
// typereg.Default.Decode counts every bracket of its input rather than RM
// values where they open, so it gets its own pair: a document whose bracket
// depth is exactly 512 decodes, and one at 513 is refused with "(513 > 512)".
// The pair reuses the concrete documents above, whose bracket depths the test
// asserts. Can-fail: Registry.Decode's "d > maxDecodeDepth" changed to
// "d >= maxDecodeDepth" refuses the 512 document.
func TestUnmarshalMaxDepthBoundary(t *testing.T) {
	wrap := func(n int, leaf string) []byte {
		doc := leaf
		for range n {
			doc = `{"_type":"FOLDER","archetype_node_id":"at0001","folders":[` + doc + `]}`
		}
		return []byte(doc)
	}
	cases := []struct {
		name  string
		doc   []byte
		depth int  // constructed nesting depth, asserted with bracketDepth
		ok    bool // true: must decode; false: refused with "(513 > 512)"
	}{
		{"concrete at 512 decodes", wrap(255, `{"_type":"FOLDER","archetype_node_id":"at0001","archetype_details":{"_type":"ARCHETYPED","rm_version":"1.1.0"}}`), 512, true},
		{"polymorphic at 512 decodes", wrap(255, `{"_type":"FOLDER","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"leaf"}}`), 512, true},
		{"concrete at 513 refused", wrap(256, `{"_type":"FOLDER","archetype_node_id":"at0001"}`), 513, false},
		{"polymorphic at 513 refused", wrap(255, `{"_type":"FOLDER","archetype_node_id":"at0001","name":{"_type":"DV_CODED_TEXT","value":"x","defining_code":{"_type":"CODE_PHRASE","code_string":"a"}}}`), 513, false},
	}
	for _, tc := range cases {
		if d := bracketDepth(tc.doc); d != tc.depth {
			t.Fatalf("%s: constructed document nests %d levels; want exactly %d", tc.name, d, tc.depth)
		}
	}

	// The registry pair: the concrete 512 and 513 documents, measured in
	// brackets, which is what the registry counts.
	registryCases := []struct {
		name  string
		doc   []byte
		depth int
		ok    bool
	}{
		{"bracket depth 512 decodes", cases[0].doc, 512, true},
		{"bracket depth 513 refused", cases[2].doc, 513, false},
	}
	for _, tc := range registryCases {
		if d := bracketDepth(tc.doc); d != tc.depth {
			t.Fatalf("registry %s: constructed document nests %d brackets; want exactly %d", tc.name, d, tc.depth)
		}
	}

	for _, route := range depthRoutes {
		routeCases := cases
		if route.countsBrackets {
			routeCases = registryCases
		}
		for _, tc := range routeCases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				err := route.decode(tc.doc)
				if tc.ok {
					if err != nil {
						t.Errorf("%s(%s, deepest value at %d) = %v; want nil (if this changes on purpose, update REQ-108)", route.name, tc.name, tc.depth, err)
					}
					return
				}
				if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
					t.Fatalf("%s(%s, deepest value at %d) err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded)", route.name, tc.name, tc.depth, err)
				}
				if want := fmt.Sprintf("(%d > %d)", 513, 512); !strings.Contains(err.Error(), want) {
					t.Errorf("%s(%s) err = %v; want the text to contain %q", route.name, tc.name, err, want)
				}
			})
		}
	}
}

// TestUnmarshalUndeclaredNestingDependsOnBuffering pins REQ-108's
// buffered-value rule for nesting inside a member the target type does not
// declare. A concrete value decoded outside a slot skips an undeclared member
// through the tokenizer, so 600 levels of arrays under `zz` on a top-level
// DV_QUANTITY decode (bounded only by jsontext's own limit). The same
// DV_QUANTITY in the polymorphic ELEMENT.value slot is buffered, and
// DecodePolymorphic counts every bracket of the buffered value, undeclared
// members included, so it is refused with typereg.ErrMaxDepthExceeded.
//
// typereg.Default.Decode buffers its whole input and counts every bracket, so
// the same top-level DV_QUANTITY is refused there. The measure differs for a
// declared member too: 255 FOLDER wrappers around a leaf whose declared
// feeder_audit.originating_system_item_ids is `[]` put that empty array at
// bracket depth 513. canjson.Unmarshal decodes it (DecodeInto counts RM values
// where they open, and the deepest RM value, the FEEDER_AUDIT, opens at 512),
// while typereg.Default.Decode refuses it (513 brackets).
//
// Can-fail (checked with go test -overlay on a patched copy of streaming.go):
// dropping DecodePolymorphic's jsonNestingDepth(raw) check lets the slot case
// decode, so its sub-test fails.
func TestUnmarshalUndeclaredNestingDependsOnBuffering(t *testing.T) {
	quantity := `{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","zz":` +
		strings.Repeat("[", 600) + strings.Repeat("]", 600) + `}`
	if d := bracketDepth([]byte(quantity)); d != 601 {
		t.Fatalf("constructed DV_QUANTITY nests %d levels; want 601 (the object plus 600 arrays)", d)
	}

	t.Run("top-level concrete value skips the undeclared member", func(t *testing.T) {
		var q rm.DVQuantity
		if err := canjson.Unmarshal([]byte(quantity), &q); err != nil {
			t.Fatalf("Unmarshal(DV_QUANTITY with 600-deep undeclared zz) = %v; want nil (the tokenizer skips an undeclared member; if this changes on purpose, update REQ-108)", err)
		}
		if q.Magnitude != 80.5 || q.Units != "kg" {
			t.Errorf("got Magnitude=%v Units=%q; want 80.5 kg (if this changes on purpose, update REQ-108)", q.Magnitude, q.Units)
		}
	})
	t.Run("registry decode counts every bracket", func(t *testing.T) {
		_, err := typereg.Default.Decode([]byte(quantity))
		if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
			t.Fatalf("typereg.Default.Decode(DV_QUANTITY with 600-deep undeclared zz) err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded)", err)
		}
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("typereg.Default.Decode(DV_QUANTITY with 600-deep undeclared zz) err = %v; a depth refusal must not carry canjson.ErrInvalidShape", err)
		}
	})
	t.Run("declared empty array at bracket depth 513", func(t *testing.T) {
		doc := `{"_type":"FOLDER","archetype_node_id":"at0001","feeder_audit":{"_type":"FEEDER_AUDIT","originating_system_item_ids":[]}}`
		for range 255 {
			doc = `{"_type":"FOLDER","archetype_node_id":"at0001","folders":[` + doc + `]}`
		}
		if d := bracketDepth([]byte(doc)); d != 513 {
			t.Fatalf("constructed document nests %d brackets; want exactly 513", d)
		}
		var f rm.Folder
		if err := canjson.Unmarshal([]byte(doc), &f); err != nil {
			t.Errorf("canjson.Unmarshal(empty declared array at bracket depth 513) = %v; want nil: DecodeInto counts RM values where they open, and the FEEDER_AUDIT opens at 512 (if this changes on purpose, update REQ-108)", err)
		}
		_, err := typereg.Default.Decode([]byte(doc))
		if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
			t.Errorf("typereg.Default.Decode(empty declared array at bracket depth 513) err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded): the registry counts every bracket", err)
		}
	})
	t.Run("value in a polymorphic slot counts every bracket", func(t *testing.T) {
		in := `{"_type":"ELEMENT","archetype_node_id":"at0001","name":{"_type":"DV_TEXT","value":"n"},"value":` + quantity + `}`
		var el rm.Element
		err := canjson.Unmarshal([]byte(in), &el)
		if !errors.Is(err, typereg.ErrMaxDepthExceeded) {
			t.Fatalf("Unmarshal(ELEMENT.value = DV_QUANTITY with 600-deep undeclared zz) err = %v; want errors.Is(_, typereg.ErrMaxDepthExceeded)", err)
		}
		if errors.Is(err, canjson.ErrInvalidShape) {
			t.Errorf("Unmarshal(ELEMENT.value = DV_QUANTITY with 600-deep undeclared zz) err = %v; a depth refusal must not carry canjson.ErrInvalidShape", err)
		}
	})
}
