package restprobes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The media types PROBE-108 expects on the wire. They are spelled out here
// rather than read from composition.Format, so the probe's oracle is not the
// code under test.
const (
	probe108MediaJSON       = "application/json"
	probe108MediaXML        = "application/xml"
	probe108MediaFlat       = "application/openehr.wt.flat+json"
	probe108MediaStructured = "application/openehr.wt.structured+json"
)

// probe108Paths are the canonical-JSON paths of the identifying values
// PROBE-108 compares between the composition written and each composition
// decoded from the wire. Each is a value the FLAT and STRUCTURED round trip
// keeps. The simplified formats rewrite others (the rm_version in
// archetype_details, the names of entries below the root), so comparing
// whole compositions across codecs would fail a faithful server.
//
// Beside these the probe compares one data value: the first ELEMENT value
// under content, found by a walk of the written composition in document
// order.
var probe108Paths = []string{
	"archetype_node_id",
	"name/value",
	"archetype_details/template_id/value",
	"language/code_string",
	"territory/code_string",
	"category/defining_code/code_string",
	"content/0/archetype_node_id",
}

// probe108Arm is one write PROBE-108 makes and what its request must carry.
type probe108Arm struct {
	name   string
	format composition.Format
	// simplified marks FLAT and STRUCTURED, which are written and read
	// against the Web Template and carry the template id.
	simplified bool
	// formatQuery makes the write and the read name their format with query
	// parameters; formatParam is the `format` value both must then send.
	formatQuery bool
	formatParam string
	// readBack reads the written version back in the same format.
	readBack bool
	// contentType and accept are the write's headers; accept is the read's
	// Accept as well.
	contentType string
	accept      string
}

// webTemplate is the Web Template the arm hands to the format options:
// FLAT and STRUCTURED need it, canonical XML is written and read without it.
func (a probe108Arm) webTemplate(wt *webtemplate.WebTemplate) *webtemplate.WebTemplate {
	if a.simplified {
		return wt
	}
	return nil
}

var probe108Arms = []probe108Arm{
	{name: "XML", format: composition.FormatXML, readBack: true, contentType: probe108MediaXML, accept: probe108MediaXML},
	{name: "FLAT", format: composition.FormatFlat, simplified: true, readBack: true, contentType: probe108MediaFlat, accept: probe108MediaFlat},
	{name: "STRUCTURED", format: composition.FormatStructured, simplified: true, readBack: true, contentType: probe108MediaStructured, accept: probe108MediaStructured},
	{name: "FLAT (format query)", format: composition.FormatFlat, simplified: true, formatQuery: true, formatParam: "FLAT", readBack: true, contentType: probe108MediaJSON, accept: probe108MediaJSON},
}

// Probe108CompositionFormats implements PROBE-108: a composition written and
// read in canonical XML, FLAT and STRUCTURED travels with the format's media
// types and, in FLAT and STRUCTURED, the template id, and the bodies the
// server returns decode in the format asked for.
//
// For each of the three formats the probe saves comp under
// `Prefer: return=representation` and reads the version the write named back
// in the same format. Then it saves comp in FLAT once more with the format
// query option, which names the format and the template with query
// parameters beside `application/json`, and reads that version back with the
// read's format query option, which names the format with a query parameter
// beside `Accept: application/json`.
//
// The probe holds two oracles of its own. The media types are spelled out
// here, not read from the leaf. Each request body is decoded by the probe
// with the codec of the format it asked for, so a body the leaf serialised
// in the wrong format fails even when the server is lenient. The bodies are
// then compared with comp on a fixed set of identifying values and on one
// data value, never byte for byte: the codecs keep different detail.
//
// wt and comp must round-trip through canonical XML, FLAT and STRUCTURED on
// those paths. The probe checks that before it sends anything and returns an
// error, not a verdict, when they do not: that is a harness input problem,
// not a deployment finding.
//
// captured returns the requests the backend received; the probe reads the
// requests each call adds, and fails when any of them carries a
// `.schema`-suffixed media type in its Content-Type or Accept header.
func Probe108CompositionFormats(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID openehrclient.EHRID, wt *webtemplate.WebTemplate, comp *rm.Composition) (Result, error) { // PROBE-108 (REQ-173)
	r := Result{Probe: "PROBE-108"}
	if c == nil || ehrID == "" || wt == nil || comp == nil {
		return r, errors.New("PROBE-108: missing required inputs (client/ehr/webTemplate/comp)")
	}
	if captured == nil {
		return r, errors.New("PROBE-108: nil captured-request recorder")
	}
	if wt.TemplateID == "" {
		return r, errors.New("PROBE-108: the Web Template carries no template id")
	}
	want, err := probe108Marks(comp)
	if err != nil {
		return r, fmt.Errorf("PROBE-108: %w", err)
	}
	if err := probe108RoundTrips(comp, wt, want); err != nil {
		return r, fmt.Errorf("PROBE-108: %w", err)
	}
	writePath, err := servicePath(c, "/ehr/"+string(ehrID)+"/composition")
	if err != nil {
		return r, fmt.Errorf("PROBE-108: %w", err)
	}

	for _, arm := range probe108Arms {
		uid, fault := probe108Write(ctx, c, captured, ehrID, comp, wt, arm, writePath, want)
		if fault != "" {
			r.Status = "fail"
			r.Detail = fault
			return r, nil
		}
		if !arm.readBack {
			continue
		}
		readPath, err := servicePath(c, "/ehr/"+string(ehrID)+"/composition/"+string(uid))
		if err != nil {
			return r, fmt.Errorf("PROBE-108: %w", err)
		}
		if fault := probe108Read(ctx, c, captured, ehrID, uid, wt, arm, readPath, want); fault != "" {
			r.Status = "fail"
			r.Detail = fault
			return r, nil
		}
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("XML, FLAT and STRUCTURED each wrote with their media types and read back equal on %d paths; "+
		"FLAT under the format query options wrote with format=FLAT and templateId=%s and read back with format=FLAT; "+
		"no .schema media type", len(want), wt.TemplateID)
	return r, nil
}

// probe108Write saves comp in the arm's format and checks the request and the
// representation. It returns the version uid the write named, or the probe
// detail of the first fault.
func probe108Write(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID openehrclient.EHRID, comp *rm.Composition, wt *webtemplate.WebTemplate, arm probe108Arm, writePath string, want map[string]string) (openehrclient.VersionUID, string) {
	opts := []composition.WriteOption{
		composition.WithPrefer(transport.PreferRepresentation),
		composition.WithFormat(arm.format, arm.webTemplate(wt)),
	}
	if arm.formatQuery {
		opts = append(opts, composition.WithFormatQuery())
	}
	before := len(captured())
	out, meta, err := composition.Save(ctx, c, ehrID, comp, opts...)
	reqs := requestsSince(captured, before)
	if fault := schemaMediaTypeFault(reqs); fault != "" {
		return "", arm.name + " write: " + fault
	}
	if err != nil {
		if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
			return "", fmt.Sprintf("%s write committed, but its representation did not decode as %s: %v", arm.name, arm.format, err)
		}
		return "", fmt.Sprintf("%s write failed: %v", arm.name, err)
	}
	if len(reqs) != 1 {
		return "", fmt.Sprintf("%s write issued %d requests, want exactly 1", arm.name, len(reqs))
	}
	req := reqs[0]
	if req.Method != http.MethodPost || req.URL.Path != writePath {
		return "", fmt.Sprintf("%s write went to %s %s, want POST %s", arm.name, req.Method, req.URL.Path, writePath)
	}
	if got := req.Header.Get("Content-Type"); got != arm.contentType {
		return "", fmt.Sprintf("%s write: Content-Type = %q, want %q", arm.name, got, arm.contentType)
	}
	if got := req.Header.Get("Accept"); got != arm.accept {
		return "", fmt.Sprintf("%s write: Accept = %q, want %q", arm.name, got, arm.accept)
	}
	if arm.simplified {
		if got := req.Header.Get("openehr-template-id"); got != wt.TemplateID {
			return "", fmt.Sprintf("%s write: openehr-template-id = %q, want the Web Template's id %q", arm.name, got, wt.TemplateID)
		}
	}
	if arm.formatQuery {
		q := req.URL.Query()
		if got := q.Get("format"); got != arm.formatParam {
			return "", fmt.Sprintf("%s write: query parameter format = %q, want %q", arm.name, got, arm.formatParam)
		}
		if got := q.Get("templateId"); got != wt.TemplateID {
			return "", fmt.Sprintf("%s write: query parameter templateId = %q, want the Web Template's id %q", arm.name, got, wt.TemplateID)
		}
	}
	body, err := readRequestBody(req)
	if err != nil {
		return "", fmt.Sprintf("%s write: reading the request body: %v", arm.name, err)
	}
	sent, err := probe108Decode(arm.format, body, wt)
	if err != nil {
		return "", fmt.Sprintf("%s write: the request body does not decode as %s: %v", arm.name, arm.format, err)
	}
	if diff, err := probe108Diff(want, sent); err != nil || diff != "" {
		return "", fmt.Sprintf("%s write: the request body differs from the composition given at %s", arm.name, diffOrErr(diff, err))
	}
	if out == nil {
		return "", arm.name + " write returned no composition from its representation"
	}
	if diff, err := probe108Diff(want, out); err != nil || diff != "" {
		return "", fmt.Sprintf("%s write: the representation differs from the composition written at %s", arm.name, diffOrErr(diff, err))
	}
	if meta == nil || meta.VersionUID == "" {
		return "", arm.name + " write named no version to read back (no ETag or Location holds a version uid)"
	}
	return meta.VersionUID, ""
}

// probe108Read reads the version uid names back in the arm's format, by
// query parameter when the arm names its format that way, and checks the
// request and the composition decoded from the 200 body. It returns the probe
// detail of the first fault, or "".
func probe108Read(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID openehrclient.EHRID, uid openehrclient.VersionUID, wt *webtemplate.WebTemplate, arm probe108Arm, readPath string, want map[string]string) string {
	opts := []composition.GetOption{composition.WithGetFormat(arm.format, arm.webTemplate(wt))}
	if arm.formatQuery {
		opts = append(opts, composition.WithGetFormatQuery())
	}
	before := len(captured())
	got, _, err := composition.Get(ctx, c, ehrID, openehrclient.VersionOf(uid), opts...)
	reqs := requestsSince(captured, before)
	if fault := schemaMediaTypeFault(reqs); fault != "" {
		return arm.name + " read: " + fault
	}
	if err != nil {
		return fmt.Sprintf("%s read failed: %v", arm.name, err)
	}
	if len(reqs) != 1 {
		return fmt.Sprintf("%s read issued %d requests, want exactly 1", arm.name, len(reqs))
	}
	req := reqs[0]
	if req.Method != http.MethodGet || req.URL.Path != readPath {
		return fmt.Sprintf("%s read went to %s %s, want GET %s", arm.name, req.Method, req.URL.Path, readPath)
	}
	if accept := req.Header.Get("Accept"); accept != arm.accept {
		return fmt.Sprintf("%s read: Accept = %q, want %q", arm.name, accept, arm.accept)
	}
	if arm.formatQuery {
		if format := req.URL.Query().Get("format"); format != arm.formatParam {
			return fmt.Sprintf("%s read: query parameter format = %q, want %q", arm.name, format, arm.formatParam)
		}
	}
	if got == nil {
		return arm.name + " read returned no composition"
	}
	if diff, err := probe108Diff(want, got); err != nil || diff != "" {
		return fmt.Sprintf("%s read: the composition read back differs from the one written at %s", arm.name, diffOrErr(diff, err))
	}
	return ""
}

// schemaMediaTypeFault names the first `.schema`-suffixed media type among
// the Content-Type and Accept values of reqs, or returns "" when there is
// none. Those two headers are where a request carries a media type; each
// Accept value is split into its media ranges, so a retired type listed
// beside a current one is caught too.
func schemaMediaTypeFault(reqs []*http.Request) string {
	for _, req := range reqs {
		for _, header := range []string{"Content-Type", "Accept"} {
			for _, value := range req.Header.Values(header) {
				for mediaRange := range strings.SplitSeq(value, ",") {
					if isSchemaMediaType(mediaRange) {
						return fmt.Sprintf("%s carries the retired .schema media type %q", header, strings.TrimSpace(mediaRange))
					}
				}
			}
		}
	}
	return ""
}

// isSchemaMediaType reports whether a media range names a `.schema`-suffixed
// type, such as `application/openehr.wt.flat.schema+json`: its subtype,
// without parameters or a structured-syntax suffix, ends in ".schema".
func isSchemaMediaType(mediaRange string) bool {
	mediaType, _, _ := strings.Cut(mediaRange, ";")
	_, subtype, ok := strings.Cut(strings.ToLower(strings.TrimSpace(mediaType)), "/")
	if !ok {
		return false
	}
	base, _, _ := strings.Cut(subtype, "+")
	return strings.HasSuffix(base, ".schema")
}

// requestsSince returns the requests the recorder holds beyond the first
// before, or none when it holds no more than that.
func requestsSince(captured func() []*http.Request, before int) []*http.Request {
	reqs := captured()
	if len(reqs) <= before {
		return nil
	}
	return reqs[before:]
}

// readRequestBody returns a captured request's body; a request without one
// has an empty body.
func readRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	return io.ReadAll(req.Body)
}

// probe108Encode serialises comp in f with the format's own codec.
func probe108Encode(f composition.Format, comp *rm.Composition, wt *webtemplate.WebTemplate) ([]byte, error) {
	switch f {
	case composition.FormatJSON:
		return canjson.Marshal(comp)
	case composition.FormatXML:
		return canxml.Marshal(comp)
	case composition.FormatFlat:
		return simplified.MarshalFlat(comp, wt)
	case composition.FormatStructured:
		return simplified.MarshalStructured(comp, wt)
	default:
		return nil, fmt.Errorf("no codec for %s", f)
	}
}

// probe108Decode decodes body in f with the format's own codec.
func probe108Decode(f composition.Format, body []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	switch f {
	case composition.FormatJSON:
		var comp rm.Composition
		if err := canjson.Unmarshal(body, &comp); err != nil {
			return nil, err
		}
		return &comp, nil
	case composition.FormatXML:
		var comp rm.Composition
		if err := canxml.Unmarshal(body, &comp); err != nil {
			return nil, err
		}
		return &comp, nil
	case composition.FormatFlat:
		return simplified.UnmarshalFlat(body, wt)
	case composition.FormatStructured:
		return simplified.UnmarshalStructured(body, wt)
	default:
		return nil, fmt.Errorf("no codec for %s", f)
	}
}

// probe108RoundTrips checks the probe's precondition: comp serialises in
// canonical XML, FLAT and STRUCTURED and decodes back equal on the compared
// paths, so a difference the probe later reports comes from the wire, not
// from a codec.
func probe108RoundTrips(comp *rm.Composition, wt *webtemplate.WebTemplate, want map[string]string) error {
	for _, f := range []composition.Format{composition.FormatXML, composition.FormatFlat, composition.FormatStructured} {
		body, err := probe108Encode(f, comp, wt)
		if err != nil {
			return fmt.Errorf("the composition does not serialise as %s: %w", f, err)
		}
		back, err := probe108Decode(f, body, wt)
		if err != nil {
			return fmt.Errorf("the composition does not decode back from %s: %w", f, err)
		}
		diff, err := probe108Diff(want, back)
		if err != nil {
			return fmt.Errorf("the composition decoded back from %s: %w", f, err)
		}
		if diff != "" {
			return fmt.Errorf("the composition does not round-trip through %s on the compared paths (%s)", f, diff)
		}
	}
	return nil
}

// probe108Marks returns the values PROBE-108 compares, keyed by their
// canonical-JSON path: those at probe108Paths and the first ELEMENT value
// under content. It fails when comp has no value at one of them, since a
// path the composition lacks would compare nothing.
func probe108Marks(comp *rm.Composition) (map[string]string, error) {
	tree, err := canonicalTree(comp)
	if err != nil {
		return nil, err
	}
	marks := make(map[string]string, len(probe108Paths)+1)
	for _, p := range probe108Paths {
		v, ok := lookupPath(tree, p)
		if !ok {
			return nil, fmt.Errorf("the composition has no value at %s to compare", p)
		}
		marks[p] = v
	}
	elementPath, ok := firstElementValuePath(tree)
	if !ok {
		return nil, errors.New("the composition has no ELEMENT value under content to compare")
	}
	v, _ := lookupPath(tree, elementPath)
	marks[elementPath] = v
	return marks, nil
}

// probe108Diff compares got with the marks of the composition written, on the
// marks' own paths. It returns "" when every value is equal, and otherwise
// the first differing path, in sorted order, with the kind of mismatch:
// absent, or a value that differs. It never names the values: they are
// clinical data, and the result lands in the probe's Detail and in logs.
func probe108Diff(want map[string]string, got *rm.Composition) (string, error) {
	tree, err := canonicalTree(got)
	if err != nil {
		return "", err
	}
	for _, p := range slices.Sorted(maps.Keys(want)) {
		v, ok := lookupPath(tree, p)
		if !ok {
			return p + ": absent", nil
		}
		if v != want[p] {
			return p + ": value differs", nil
		}
	}
	return "", nil
}

// diffOrErr renders the outcome of probe108Diff for a probe detail.
func diffOrErr(diff string, err error) string {
	if err != nil {
		return fmt.Sprintf("(the composition does not render as canonical JSON: %v)", err)
	}
	return diff
}

// canonicalTree renders comp as canonical JSON and decodes it into a generic
// tree, numbers kept as their literal text so no value is rounded.
func canonicalTree(comp *rm.Composition) (any, error) {
	body, err := canjson.Marshal(comp)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// lookupPath returns the canonical JSON of the value at a slash-separated
// path of object members and array indexes. ok is false when the path does
// not resolve or ends on a null or an empty string.
func lookupPath(tree any, path string) (string, bool) {
	node := tree
	for seg := range strings.SplitSeq(path, "/") {
		switch n := node.(type) {
		case map[string]any:
			node = n[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n) {
				return "", false
			}
			node = n[i]
		default:
			return "", false
		}
	}
	if node == nil || node == "" {
		return "", false
	}
	text, err := json.Marshal(node)
	if err != nil {
		return "", false
	}
	return string(text), true
}

// firstElementValuePath walks the content of a canonical-JSON composition in
// document order (array items in order, object members by name) and returns
// the path of the first ELEMENT's value.
func firstElementValuePath(tree any) (string, bool) {
	root, ok := tree.(map[string]any)
	if !ok {
		return "", false
	}
	return findElementValue(root["content"], "content")
}

func findElementValue(node any, at string) (string, bool) {
	switch n := node.(type) {
	case map[string]any:
		if n["_type"] == "ELEMENT" && n["value"] != nil {
			return at + "/value", true
		}
		for _, k := range slices.Sorted(maps.Keys(n)) {
			if p, ok := findElementValue(n[k], at+"/"+k); ok {
				return p, true
			}
		}
	case []any:
		for i, item := range n {
			if p, ok := findElementValue(item, at+"/"+strconv.Itoa(i)); ok {
				return p, true
			}
		}
	}
	return "", false
}
