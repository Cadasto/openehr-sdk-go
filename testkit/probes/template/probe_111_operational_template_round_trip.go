package templateprobes

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// Probe111OperationalTemplates returns every ADL 1.4 operational template the
// corpus vendors: each `.opt` file under [fixtures.CorpusRoot], found by
// walking the whole tree, as a slash-separated path relative to that root,
// sorted. It reaches the files [fixtures.ListAllOPTs] leaves out, the ones
// beside the cross-format sets among them.
func Probe111OperationalTemplates() ([]string, error) { // PROBE-111 (REQ-176)
	root := fixtures.CorpusRoot()
	var names []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || filepath.Ext(path) != ".opt" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("PROBE-111: walk the corpus: %w", err)
	}
	slices.Sort(names)
	return names, nil
}

// probe111SchemaRatchetFile lists the corpus operational templates the pinned
// AM Release-1.4 schemas refuse as written (REQ-176 § Round trip, PROBE-111).
// make operational-template-schema-check holds the list to what xmllint
// reports, so an entry that no longer fires fails as stale.
//
//go:embed probe_111_schema_ratchet.txt
var probe111SchemaRatchetFile string

// Probe111SchemaRatchetEntry is one corpus operational template the pinned
// AM Release-1.4 schemas refuse as written.
type Probe111SchemaRatchetEntry struct {
	// Name is the template's path relative to the corpus root, as
	// [Probe111OperationalTemplates] gives it.
	Name string
	// Reason says why the schemas refuse the document.
	Reason string
}

// Probe111SchemaRatchet returns the corpus operational templates the pinned
// AM Release-1.4 schemas refuse as written, each with its reason, in file
// order. PROBE-111 runs only its round-trip leg on them, and its schema leg
// on every other one. The list lives in probe_111_schema_ratchet.txt beside
// the probe: one template per line, its name and its reason separated by a
// tab, with blank lines and lines starting with `#` ignored.
func Probe111SchemaRatchet() ([]Probe111SchemaRatchetEntry, error) { // PROBE-111 (REQ-176)
	return probe111ParseRatchet(probe111SchemaRatchetFile)
}

func probe111ParseRatchet(text string) ([]Probe111SchemaRatchetEntry, error) {
	var entries []Probe111SchemaRatchetEntry
	seen := map[string]bool{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, ok := strings.Cut(line, "\t")
		name, reason = strings.TrimSpace(name), strings.TrimSpace(reason)
		if !ok || name == "" || reason == "" {
			return nil, fmt.Errorf("PROBE-111: schema ratchet line %d: want a template name and its reason, separated by a tab", i+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("PROBE-111: schema ratchet line %d: %s is listed twice", i+1, name)
		}
		seen[name] = true
		entries = append(entries, Probe111SchemaRatchetEntry{Name: name, Reason: reason})
	}
	return entries, nil
}

// Probe111OperationalTemplateRoundTrip implements PROBE-111's round-trip leg
// for one ADL 1.4 operational template document. It decodes body into the
// generated [aom14.OperationalTemplate] with [canxml.WithUnmatched] and
// requires no unmatched item, decodes it again with [canxml.WithStrict] and
// requires no refusal, encodes the value with [canxml.Marshal], and decodes
// that document once more, which must report nothing unmatched and give a
// value equal to the first. The two values compare through their canonical
// JSON, written by canjson and read back as generic JSON values
// ([wireequiv.Equivalent]): every member the model declares, compared by
// value, with the entries of a Hash member as object members whose order
// carries no meaning.
//
// Then it compares the re-encoded document with body, read as elements,
// attributes and text: the same elements in the same order, with the same
// attributes and text. The comparison sets aside the XML declaration,
// comments, namespace declarations and white space between elements, and
// normalises both sides. The entries of a Hash member (a run of
// StringDictionaryItem elements keyed by `id`, or of `details` and
// `translations` keyed by their language code) compare as a set; the text
// of a numeric leaf compares by its decimal value, so `1.0` equals `1`; an
// interval that leaves out a boundary flag compares as one that writes the
// flag's default (an absent `*_unbounded` is true where its side carries no
// bound, an absent `*_included` true where its side is bounded); an
// `xsi:type` compares as the qualified name it denotes; and the root
// element's `xsi:type` is set aside. On body alone it first applies the
// rewrites the encoder makes: `match_negated` removed, a `timezone_validity`
// or `operator` word written as its integer code, `T_ARCHETYPE_ROOT` written
// as `C_ARCHETYPE_ROOT`, and the empty `value` of an ORDINAL or SCALE
// `symbol` left out.
//
// Each check that does not hold fails the probe, named in Detail with the
// first difference; a nil or empty body is a framework error.
func Probe111OperationalTemplateRoundTrip(body []byte) (Result, error) { // PROBE-111 (REQ-176)
	return probe111RoundTrip(body, canxml.Unmarshal)
}

// Probe111ReEncode decodes body into the generated [aom14.OperationalTemplate]
// and returns its canonical XML: the document PROBE-111's schema leg
// validates against the pinned AM Release-1.4 schemas.
func Probe111ReEncode(body []byte) ([]byte, error) { // PROBE-111 (REQ-176)
	var ot aom14.OperationalTemplate
	if err := canxml.Unmarshal(body, &ot); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out, err := canxml.Marshal(&ot)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return out, nil
}

// probe111Decoder is the decode step of the round-trip leg: canxml.Unmarshal
// in the probe, a double that withholds a member in the can-fail control
// (probe_111_guard_internal_test.go).
type probe111Decoder func(data []byte, v any, opts ...canxml.DecoderOption) error

// probe111RoundTrip runs the round-trip leg with decode as every decode step
// (REQ-176 § Round trip).
func probe111RoundTrip(body []byte, decode probe111Decoder) (Result, error) {
	r := Result{Probe: "PROBE-111"}
	if len(body) == 0 {
		return r, errors.New("PROBE-111: empty operational template body")
	}
	fail := func(format string, args ...any) (Result, error) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf(format, args...)
		return r, nil
	}

	var unmatched []string
	var first aom14.OperationalTemplate
	if err := decode(body, &first, canxml.WithUnmatched(func(p string) { unmatched = append(unmatched, p) })); err != nil {
		return fail("decode: %v", err)
	}
	if len(unmatched) > 0 {
		return fail("decode: %d unmatched items, the first at %s", len(unmatched), unmatched[0])
	}
	var strict aom14.OperationalTemplate
	if err := decode(body, &strict, canxml.WithStrict()); err != nil {
		return fail("strict decode refuses the document: %v", err)
	}

	out, err := canxml.Marshal(&first)
	if err != nil {
		return fail("encode: %v", err)
	}
	var second aom14.OperationalTemplate
	if err := decode(out, &second, canxml.WithUnmatched(func(p string) { unmatched = append(unmatched, p) })); err != nil {
		return fail("decode of the re-encoded document: %v", err)
	}
	if len(unmatched) > 0 {
		return fail("decode of the re-encoded document: %d unmatched items, the first at %s", len(unmatched), unmatched[0])
	}
	d, err := probe111SameValue(&first, &second)
	if err != nil {
		return fail("compare the decodes: %v", err)
	}
	if d != "" {
		return fail("the decode of the re-encoded document differs from the first decode: %s", d)
	}

	d, err = probe111CompareDocuments(body, out)
	if err != nil {
		return fail("compare: %v", err)
	}
	if d != "" {
		return fail("the re-encoded document differs from the input at %s", d)
	}
	r.Status = "pass"
	return r, nil
}

// probe111SameValue returns how two decoded values differ, compared through
// their canonical JSON read back as generic JSON values; "" when they are
// equal. The comparison needs no reflection, and canjson, a second encoder
// of the same model, writes every member the model declares.
func probe111SameValue(a, b *aom14.OperationalTemplate) (string, error) {
	ja, err := canjson.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("canonical JSON of the first decode: %w", err)
	}
	jb, err := canjson.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("canonical JSON of the second decode: %w", err)
	}
	if ok, why := wireequiv.Equivalent(ja, jb); !ok {
		return why, nil
	}
	return "", nil
}
