package canxml_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// canonicalJSONCassettes resolves the JSON cassette directory
// relative to this test file. It is the source-of-truth set of RM
// graphs that the cross-format invariant validates.
const canonicalJSONCassettes = "../../../testkit/cassettes/canonical_json"

// canonicalXMLCassettes resolves the XML cassette directory.
const canonicalXMLCassettes = "../../../testkit/cassettes/canonical_xml"

// TestCrossFormatRoundTripFromJSONCassettes exercises the
// `JSON → struct → XML → struct → JSON` invariant against every
// vendored Composition cassette. Equality is asserted *structurally*
// (after null/absent normalisation) — byte equality across the JSON
// and XML wire shapes is not meaningful.
//
// This is the strongest shared invariant with the canjson plan:
// failures indicate a bug in either codec.
func TestCrossFormatRoundTripFromJSONCassettes(t *testing.T) {
	entries, err := os.ReadDir(canonicalJSONCassettes)
	if err != nil {
		t.Fatalf("read JSON cassette dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Fatal("no JSON cassettes discovered — check testkit/cassettes/canonical_json/")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(canonicalJSONCassettes, name))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			// JSON → struct A
			var a rm.Composition
			if err := canjson.Unmarshal(raw, &a); err != nil {
				t.Fatalf("JSON Unmarshal: %v", err)
			}
			// struct A → XML
			xb, err := canxml.Marshal(&a)
			if err != nil {
				t.Fatalf("XML Marshal: %v", err)
			}
			// XML → struct C
			var c rm.Composition
			if err := canxml.Unmarshal(xb, &c); err != nil {
				t.Fatalf("XML Unmarshal: %v\nbody: %s", err, xb)
			}
			// struct C → JSON
			jd, err := canjson.Marshal(&c)
			if err != nil {
				t.Fatalf("JSON re-Marshal: %v", err)
			}
			// Structural equivalence: re-encode A as JSON too and
			// compare the normalised JSON trees.
			ja, err := canjson.Marshal(&a)
			if err != nil {
				t.Fatalf("JSON canonical encode for A: %v", err)
			}
			a1, err := normaliseJSON(ja)
			if err != nil {
				t.Fatalf("normalise A: %v", err)
			}
			d1, err := normaliseJSON(jd)
			if err != nil {
				t.Fatalf("normalise D: %v", err)
			}
			if !jsonEqual(a1, d1) {
				aj, _ := json.MarshalIndent(a1, "", "  ")
				dj, _ := json.MarshalIndent(d1, "", "  ")
				t.Errorf("cross-format invariant violated:\n--- A (JSON→struct→JSON) ---\n%s\n--- D (JSON→struct→XML→struct→JSON) ---\n%s", aj, dj)
			}
		})
	}
}

// TestCrossFormatXMLCassetteRoundTrip — every vendored XML cassette
// round-trips byte-stable through canxml. Mirror of
// TestRoundTripCassettes in canjson for the XML wire.
func TestCrossFormatXMLCassetteRoundTrip(t *testing.T) {
	entries, err := os.ReadDir(canonicalXMLCassettes)
	if err != nil {
		t.Fatalf("read XML cassette dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".xml" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Skip("no XML cassettes vendored yet")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(canonicalXMLCassettes, name))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			// Pick the target type from the root element local name. The
			// minimal cassette set uses dv_quantity and composition;
			// extend this switch as new cassettes land.
			var into func() any
			switch {
			case bytes.HasPrefix(body, []byte("<dv_quantity")):
				into = func() any { return new(rm.DVQuantity) }
			case bytes.HasPrefix(body, []byte("<composition")):
				into = func() any { return new(rm.Composition) }
			default:
				t.Skipf("no factory wired for cassette %q", name)
			}
			v1 := into()
			if err := canxml.Unmarshal(body, v1); err != nil {
				t.Fatalf("first decode: %v", err)
			}
			b1, err := canxml.Marshal(v1)
			if err != nil {
				t.Fatalf("first encode: %v", err)
			}
			v2 := into()
			if err := canxml.Unmarshal(b1, v2); err != nil {
				t.Fatalf("second decode: %v\nbody: %s", err, b1)
			}
			b2, err := canxml.Marshal(v2)
			if err != nil {
				t.Fatalf("second encode: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("round-trip not byte-stable for %s:\n--- b1 ---\n%s\n--- b2 ---\n%s", name, b1, b2)
			}
		})
	}
}

// normaliseJSON parses data, strips map entries whose value is nil
// (the SDK treats null and absent equivalently on decode and emits
// absent on encode), and recursively normalises nested structures.
func normaliseJSON(data []byte) (any, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return stripNulls(v), nil
}

// stripNulls recursively removes nil-valued entries from maps.
func stripNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if val == nil {
				continue
			}
			cleaned := stripNulls(val)
			if cleaned == nil {
				continue
			}
			out[k] = cleaned
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripNulls(e)
		}
		return out
	default:
		return v
	}
}

// jsonEqual compares two normalised JSON values for structural
// equality.
func jsonEqual(a, b any) bool {
	switch ax := a.(type) {
	case map[string]any:
		bx, ok := b.(map[string]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for k, av := range ax {
			bv, ok := bx[k]
			if !ok || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	case []any:
		bx, ok := b.([]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for i := range ax {
			if !jsonEqual(ax[i], bx[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
