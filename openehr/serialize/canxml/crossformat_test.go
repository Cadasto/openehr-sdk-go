package canxml_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// discoverJSONCassettes walks the canonical_json/ tree one level
// deep so vendored upstream sets (e.g. `ehrbase/`) are exercised
// alongside the SDK's own fixtures. Returns paths relative to
// [canonicalJSONCassettes].
func discoverJSONCassettes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(canonicalJSONCassettes)
	if err != nil {
		t.Fatalf("read JSON cassette dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			subdir := filepath.Join(canonicalJSONCassettes, e.Name())
			subEntries, err := os.ReadDir(subdir)
			if err != nil {
				t.Fatalf("read sub-cassette dir %q: %v", subdir, err)
			}
			for _, se := range subEntries {
				if se.IsDir() || filepath.Ext(se.Name()) != ".json" {
					continue
				}
				out = append(out, filepath.Join(e.Name(), se.Name()))
			}
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// discoverXMLCassettes mirrors [discoverJSONCassettes] for the XML
// tree.
func discoverXMLCassettes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(canonicalXMLCassettes)
	if err != nil {
		t.Fatalf("read XML cassette dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			subdir := filepath.Join(canonicalXMLCassettes, e.Name())
			subEntries, err := os.ReadDir(subdir)
			if err != nil {
				t.Fatalf("read sub-cassette dir %q: %v", subdir, err)
			}
			for _, se := range subEntries {
				if se.IsDir() || filepath.Ext(se.Name()) != ".xml" {
					continue
				}
				out = append(out, filepath.Join(e.Name(), se.Name()))
			}
			continue
		}
		if filepath.Ext(e.Name()) != ".xml" {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// factoryForCassette returns a fresh-target factory matching the
// expected root RM type for a cassette path. The SDK's own JSON
// cassettes are all COMPOSITION; the ehrbase set adds EHR_STATUS
// (filename hint `ehr_status`) and FOLDER (filename hint `folder`).
func factoryForCassette(path string) (func() any, bool) {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(base, "ehr_status"):
		return func() any { return new(rm.EHRStatus) }, true
	case strings.Contains(base, "folder"):
		return func() any { return new(rm.Folder) }, true
	default:
		return func() any { return new(rm.Composition) }, true
	}
}

// factoryForXMLBody picks the factory by sniffing the root element
// local name. Tolerant of both the SDK profile (xmlns="…openehr…")
// and the upstream ehrbase profile (no default xmlns).
func factoryForXMLBody(body []byte) (func() any, bool) {
	// Look at the first non-whitespace `<…` token, skipping the XML
	// declaration if present.
	s := string(body)
	for {
		i := strings.Index(s, "<")
		if i < 0 {
			return nil, false
		}
		s = s[i:]
		if strings.HasPrefix(s, "<?") {
			end := strings.Index(s, "?>")
			if end < 0 {
				return nil, false
			}
			s = s[end+2:]
			continue
		}
		break
	}
	// s now starts with the root element open token.
	switch {
	case strings.HasPrefix(s, "<dv_quantity"):
		return func() any { return new(rm.DVQuantity) }, true
	case strings.HasPrefix(s, "<composition"):
		return func() any { return new(rm.Composition) }, true
	case strings.HasPrefix(s, "<folder"):
		return func() any { return new(rm.Folder) }, true
	case strings.HasPrefix(s, "<ehr_status"):
		return func() any { return new(rm.EHRStatus) }, true
	default:
		return nil, false
	}
}

// TestCrossFormatRoundTripFromJSONCassettes exercises the
// `JSON → struct → XML → struct → JSON` invariant against every
// vendored cassette. Equality is asserted *structurally* (after
// null/absent normalisation) — byte equality across the JSON and
// XML wire shapes is not meaningful.
//
// This is the strongest shared invariant with the canjson plan:
// failures indicate a bug in either codec.
func TestCrossFormatRoundTripFromJSONCassettes(t *testing.T) {
	names := discoverJSONCassettes(t)
	if len(names) == 0 {
		t.Fatal("no JSON cassettes discovered — check testkit/cassettes/canonical_json/")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(canonicalJSONCassettes, name))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			factory, ok := factoryForCassette(name)
			if !ok {
				t.Skipf("no factory wired for cassette %q", name)
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

// TestCrossFormatXMLCassetteRoundTrip — every vendored XML cassette
// round-trips byte-stable through canxml. The first pass (decoding
// the upstream form) may consume non-canonical bytes; from the second
// pass on the encoder's compact canonical form is byte-stable.
func TestCrossFormatXMLCassetteRoundTrip(t *testing.T) {
	names := discoverXMLCassettes(t)
	if len(names) == 0 {
		t.Skip("no XML cassettes vendored yet")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(canonicalXMLCassettes, name))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			factory, ok := factoryForXMLBody(body)
			if !ok {
				t.Skipf("no factory wired for cassette %q (root element not recognised)", name)
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
