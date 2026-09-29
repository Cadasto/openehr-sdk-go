package canxml_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// discoverJSONFixtures returns vendored *.canonical.json fixtures.
func discoverJSONFixtures(t *testing.T) []fixtures.CompositionJSONRel {
	t.Helper()
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("list JSON fixtures: %v", err)
	}
	return rels
}

func discoverXMLFixtures(t *testing.T) []string {
	t.Helper()
	rels, err := fixtures.ListRMXML()
	if err != nil {
		t.Fatalf("list XML fixtures: %v", err)
	}
	return rels
}

func factoryForFixture(rel fixtures.CompositionJSONRel) (func() any, bool) {
	return fixtures.FactoryForJSONRel(rel)
}

// TestCrossFormatRoundTripFromJSONFixtures exercises the
// `JSON → struct → XML → struct → JSON` invariant against every
// vendored fixture. Equality is asserted *structurally* (after
// null/absent normalisation) — byte equality across the JSON and
// XML wire shapes is not meaningful.
//
// This is the strongest shared invariant with the canjson plan:
// failures indicate a bug in either codec.
func TestCrossFormatRoundTripFromJSONFixtures(t *testing.T) {
	names := discoverJSONFixtures(t)
	if len(names) == 0 {
		t.Fatal("no JSON fixtures discovered — check testkit/corpus/")
	}
	for _, rel := range names {
		t.Run(rel.Rel, func(t *testing.T) {
			raw, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			factory, ok := factoryForFixture(rel)
			if !ok {
				t.Skipf("no factory wired for fixture %q", rel.Rel)
			}
			// JSON → struct A
			a := factory()
			if err := canjson.Unmarshal(raw, a); err != nil {
				t.Fatalf("JSON Unmarshal: %v", err)
			}
			// struct A → XML
			xb, err := canxml.Marshal(a)
			if err != nil {
				t.Fatalf("XML Marshal: %v", err)
			}
			// XML → struct C
			c := factory()
			if err := canxml.Unmarshal(xb, c); err != nil {
				t.Fatalf("XML Unmarshal: %v\nbody: %s", err, xb)
			}
			// struct C → JSON
			jd, err := canjson.Marshal(c)
			if err != nil {
				t.Fatalf("JSON re-Marshal: %v", err)
			}
			// Structural equivalence: re-encode A as JSON too and
			// compare the normalised JSON trees.
			ja, err := canjson.Marshal(a)
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

// TestCrossFormatVendorFixtureXML decodes CODE24 vendor XML for
// fixtures that ship composition.xml alongside composition.json.
func TestCrossFormatVendorFixtureXML(t *testing.T) {
	ids, err := fixtures.TemplateIDsWithCompositionXML()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected at least one fixture with vendor XML")
	}
	for _, tid := range ids {
		t.Run(tid, func(t *testing.T) {
			body, err := os.ReadFile(fixtures.CanonicalXML(tid))
			if err != nil {
				t.Fatalf("read XML: %v", err)
			}
			factory, ok := fixtures.FactoryForXMLBody(body)
			if !ok {
				t.Fatalf("no factory for %s", tid)
			}
			v := factory()
			if err := canxml.Unmarshal(body, v); err != nil {
				t.Fatalf("XML Unmarshal: %v", err)
			}
		})
	}
}

// TestCrossFormatXMLFixtureRoundTrip — every vendored XML fixture
// round-trips byte-stable through canxml. The first pass (decoding
// the upstream form) may consume non-canonical bytes; from the second
// pass on the encoder's compact canonical form is byte-stable.
//
// REQ-056: the encoder's compact canonical XML is byte-stable from the second
// pass on, for every vendored XML fixture.
func TestCrossFormatXMLFixtureRoundTrip(t *testing.T) {
	names := discoverXMLFixtures(t)
	if len(names) == 0 {
		t.Skip("no XML fixtures vendored yet")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(fixtures.CorpusRoot(), filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			factory, ok := fixtures.FactoryForXMLBody(body)
			if !ok {
				t.Skipf("no factory wired for fixture %q (root element not recognised)", name)
			}
			v1 := factory()
			if err := canxml.Unmarshal(body, v1); err != nil {
				t.Fatalf("first decode: %v", err)
			}
			b1, err := canxml.Marshal(v1)
			if err != nil {
				t.Fatalf("first encode: %v", err)
			}
			v2 := factory()
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
