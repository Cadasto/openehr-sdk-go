package composition_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// formatTemplateID names the vendored OPT and canonical composition the
// FLAT and STRUCTURED write tests use. PROBE-076 round-trips this pair
// through both simplified codecs.
const formatTemplateID = "Test_dv_quantity_open_constraint.v0"

// The media types REQ-173 names, spelled out here so the tests pin the
// strings the wire carries rather than the constants the code uses.
const (
	mediaJSON       = "application/json"
	mediaXML        = "application/xml"
	mediaFlat       = "application/openehr.wt.flat+json"
	mediaStructured = "application/openehr.wt.structured+json"
)

// loadFormatFixture builds the Web Template for formatTemplateID and decodes
// the matching canonical composition, the way PROBE-076 does.
func loadFormatFixture(t *testing.T) (*webtemplate.WebTemplate, *rm.Composition) {
	t.Helper()
	_, wt, comp := loadCompiledFormatFixture(t)
	return wt, comp
}

// loadCompiledFormatFixture is loadFormatFixture that also returns the
// compiled template the Web Template is built from, the one
// simplified.WithTemplate takes.
func loadCompiledFormatFixture(t *testing.T) (*templatecompile.Compiled, *webtemplate.WebTemplate, *rm.Composition) {
	t.Helper()
	optBody, err := os.ReadFile(fixtures.TemplateOpt(formatTemplateID))
	if err != nil {
		t.Fatalf("read OPT: %v", err)
	}
	opt, err := fixtures.ParseOPTBytes(optBody)
	if err != nil {
		t.Fatalf("parse OPT: %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile OPT: %v", err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("build Web Template: %v", err)
	}
	if wt.TemplateID != formatTemplateID {
		t.Fatalf("Web Template id = %q, want %q", wt.TemplateID, formatTemplateID)
	}
	compBody, err := os.ReadFile(fixtures.CompositionJSON(formatTemplateID))
	if err != nil {
		t.Fatalf("read composition: %v", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compBody, &comp); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	return compiled, wt, &comp
}

// fixtureEntryName is the name of the fixture's first content entry, an
// OBSERVATION. FLAT and STRUCTURED carry no name below the root, so only a
// decode given simplified.WithTemplate restores it (REQ-173 Simplified decode
// options).
const fixtureEntryName = "test123"

// firstEntryName returns the name of comp's first content entry, or "" when
// the entry carries none.
func firstEntryName(t *testing.T, comp *rm.Composition) string {
	t.Helper()
	if comp == nil || len(comp.Content) == 0 {
		t.Fatal("composition has no content entry")
	}
	entry, ok := comp.Content[0].(rm.Locatable)
	if !ok || rm.IsTypedNil(entry) {
		t.Fatalf("first content entry %T is not a LOCATABLE", comp.Content[0])
	}
	name := entry.GetName()
	if name == nil || rm.IsTypedNil(name) {
		return ""
	}
	return name.GetValue()
}

// decodeSimplified decodes body in f, FLAT or STRUCTURED, with the codec
// itself and the decode options given: the composition the leaf returns
// when it passes those options on unchanged.
func decodeSimplified(t *testing.T, f composition.Format, body []byte, wt *webtemplate.WebTemplate, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	var (
		comp *rm.Composition
		err  error
	)
	switch f {
	case composition.FormatFlat:
		comp, err = simplified.UnmarshalFlat(body, wt, opts...)
	case composition.FormatStructured:
		comp, err = simplified.UnmarshalStructured(body, wt, opts...)
	case composition.FormatJSON, composition.FormatXML:
		t.Fatalf("decodeSimplified: %v is not a simplified format", f)
	default:
		t.Fatalf("decodeSimplified: no codec for %v", f)
	}
	if err != nil {
		t.Fatalf("decode %v with the codec: %v", f, err)
	}
	return comp
}

// unmatchedFlatKey is a FLAT key whose path the fixture's Web Template does
// not resolve, so a decode leaves it unmatched.
const unmatchedFlatKey = "test_dv_quantity_open_constraint.v0/no_such_node"

// withUnmatchedKey returns the FLAT body with unmatchedFlatKey added in front
// of the keys it holds.
func withUnmatchedKey(t *testing.T, body []byte) []byte {
	t.Helper()
	if !bytes.HasPrefix(body, []byte("{")) {
		t.Fatalf("FLAT body does not open an object: %s", body)
	}
	return append([]byte(`{"`+unmatchedFlatKey+`":"extra",`), body[1:]...)
}

// withoutTemplateID returns a copy of wt whose template id is empty.
func withoutTemplateID(wt *webtemplate.WebTemplate) *webtemplate.WebTemplate {
	cp := *wt
	cp.TemplateID = ""
	return &cp
}

// encodeAs serialises comp in f with the codec the format names, for use as
// the expected request body or as a representation response body.
func encodeAs(t *testing.T, f composition.Format, comp *rm.Composition, wt *webtemplate.WebTemplate) []byte {
	t.Helper()
	var (
		b   []byte
		err error
	)
	switch f {
	case composition.FormatJSON:
		b, err = canjson.Marshal(comp)
	case composition.FormatXML:
		b, err = canxml.Marshal(comp)
	case composition.FormatFlat:
		b, err = simplified.MarshalFlat(comp, wt)
	case composition.FormatStructured:
		b, err = simplified.MarshalStructured(comp, wt)
	default:
		t.Fatalf("encodeAs: no codec for %v", f)
	}
	if err != nil {
		t.Fatalf("encode %v: %v", f, err)
	}
	return b
}

// sameComposition reports a difference between got and want by comparing
// their canonical JSON.
func sameComposition(t *testing.T, label string, got, want *rm.Composition) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: composition is nil", label)
		return
	}
	g, err := canjson.Marshal(got)
	if err != nil {
		t.Fatalf("%s: marshal got: %v", label, err)
	}
	w, err := canjson.Marshal(want)
	if err != nil {
		t.Fatalf("%s: marshal want: %v", label, err)
	}
	if !bytes.Equal(g, w) {
		t.Errorf("%s: composition differs from the one given\n got: %s\nwant: %s", label, g, w)
	}
}

// wireRequest is what the test server saw of one request.
type wireRequest struct {
	method string
	header http.Header
	query  url.Values
	body   []byte
}

// formatServer records every request it receives and answers each with
// status, an ETag naming compositionVUID and body. It fails the test when a
// request carries a `.schema`-suffixed media type in any header or query
// value (REQ-173: no request the leaf sends carries one).
type formatServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []wireRequest
}

func newFormatServer(t *testing.T, status int, contentType string, body []byte) *formatServer {
	t.Helper()
	fs := &formatServer{}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		for k, vs := range r.Header {
			for _, v := range vs {
				if strings.Contains(v, ".schema") {
					t.Errorf("request header %s = %q carries a .schema media type", k, v)
				}
			}
		}
		for k, vs := range r.URL.Query() {
			for _, v := range vs {
				if strings.Contains(v, ".schema") {
					t.Errorf("request query %s = %q carries a .schema media type", k, v)
				}
			}
		}
		fs.mu.Lock()
		fs.reqs = append(fs.reqs, wireRequest{
			method: r.Method, header: r.Header.Clone(), query: r.URL.Query(), body: b,
		})
		fs.mu.Unlock()
		w.Header().Set("ETag", `"`+string(compositionVUID)+`"`)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *formatServer) count() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return len(fs.reqs)
}

// only returns the one request the server saw, failing the test otherwise.
func (fs *formatServer) only(t *testing.T) wireRequest {
	t.Helper()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.reqs) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(fs.reqs))
	}
	return fs.reqs[0]
}

// writeOp runs Save or Update with the given options against c.
type writeOp struct {
	name   string
	method string
	do     func(ctx context.Context, c *transport.Client, comp *rm.Composition, opts ...composition.WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
}

var formatWriteOps = []writeOp{
	{
		name:   "Save",
		method: http.MethodPost,
		do: func(ctx context.Context, c *transport.Client, comp *rm.Composition, opts ...composition.WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
			return composition.Save(ctx, c, ehrIDFixture, comp, opts...)
		},
	},
	{
		name:   "Update",
		method: http.MethodPut,
		do: func(ctx context.Context, c *transport.Client, comp *rm.Composition, opts ...composition.WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
			return composition.Update(ctx, c, ehrIDFixture, compositionVOID, string(compositionVUID), comp, opts...)
		},
	},
}

// TestFormatMediaTypeAndName pins REQ-173 Formats: the zero value is
// canonical JSON, the four formats send their own media types and never a
// `.schema`-suffixed one, and a value outside the four names no media type.
func TestFormatMediaTypeAndName(t *testing.T) {
	var zero composition.Format
	if zero != composition.FormatJSON {
		t.Errorf("zero Format = %v, want FormatJSON", zero)
	}
	cases := []struct {
		format    composition.Format
		mediaType string
		name      string
	}{
		{format: composition.FormatJSON, mediaType: mediaJSON, name: "JSON"},
		{format: composition.FormatXML, mediaType: mediaXML, name: "XML"},
		{format: composition.FormatFlat, mediaType: mediaFlat, name: "FLAT"},
		{format: composition.FormatStructured, mediaType: mediaStructured, name: "STRUCTURED"},
		{format: composition.FormatStructured + 1, mediaType: "", name: "Format(4)"},
		{format: composition.Format(-1), mediaType: "", name: "Format(-1)"},
	}
	for _, tc := range cases {
		if got := tc.format.MediaType(); got != tc.mediaType {
			t.Errorf("Format(%d).MediaType() = %q, want %q", int(tc.format), got, tc.mediaType)
		}
		if got := tc.format.String(); got != tc.name {
			t.Errorf("Format(%d).String() = %q, want %q", int(tc.format), got, tc.name)
		}
		if strings.Contains(tc.format.MediaType(), ".schema") {
			t.Errorf("Format(%d).MediaType() = %q is a .schema media type", int(tc.format), tc.format.MediaType())
		}
	}
}

// TestSaveXMLRepresentation pins the REQ-173 canonical XML write: a Save
// with WithFormat(FormatXML, nil) under PreferRepresentation sends
// application/xml as Content-Type and Accept, a body canonical XML decodes
// back to the composition given, and returns the composition decoded from
// the XML 201 body.
func TestSaveXMLRepresentation(t *testing.T) {
	comp := readComposition(t)
	fs := newFormatServer(t, http.StatusCreated, mediaXML, encodeAs(t, composition.FormatXML, comp, nil))

	out, meta, err := composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp,
		composition.WithPrefer(transport.PreferRepresentation),
		composition.WithFormat(composition.FormatXML, nil),
	)
	if err != nil {
		t.Fatalf("Save(XML, representation) error = %v", err)
	}
	req := fs.only(t)
	if got := req.header.Get("Content-Type"); got != mediaXML {
		t.Errorf("Content-Type = %q, want %q", got, mediaXML)
	}
	if got := req.header.Get("Accept"); got != mediaXML {
		t.Errorf("Accept = %q, want %q", got, mediaXML)
	}
	if got := req.header.Get("Openehr-Template-Id"); got != "" {
		t.Errorf("openehr-template-id = %q, want none (no WithTemplateID on an XML write)", got)
	}
	var sent rm.Composition
	if err := canxml.Unmarshal(req.body, &sent); err != nil {
		t.Fatalf("request body is not canonical XML: %v", err)
	}
	sameComposition(t, "request body", &sent, comp)
	sameComposition(t, "returned composition", out, comp)
	if meta == nil || meta.VersionUID != compositionVUID {
		t.Errorf("VersionUID = %+v, want %q", meta, compositionVUID)
	}
}

// TestSaveSimplifiedFormats pins the REQ-173 FLAT and STRUCTURED writes: the
// format's media type as Content-Type, the bytes the codec writes for the
// composition and the Web Template, the Web Template's id as
// openehr-template-id and no format query parameter. Under PreferMinimal
// Accept is application/json and the version uid comes from the ETag; under
// PreferRepresentation Accept is the format's media type and the body is
// decoded against the Web Template.
func TestSaveSimplifiedFormats(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	formats := []struct {
		format    composition.Format
		mediaType string
	}{
		{format: composition.FormatFlat, mediaType: mediaFlat},
		{format: composition.FormatStructured, mediaType: mediaStructured},
	}
	for _, f := range formats {
		want := encodeAs(t, f.format, comp, wt)
		t.Run(f.format.String()+"/minimal", func(t *testing.T) {
			fs := newFormatServer(t, http.StatusCreated, "", nil)
			out, meta, err := composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp,
				composition.WithFormat(f.format, wt))
			if err != nil {
				t.Fatalf("Save(%v, minimal) error = %v", f.format, err)
			}
			req := fs.only(t)
			if got := req.header.Get("Content-Type"); got != f.mediaType {
				t.Errorf("Content-Type = %q, want %q", got, f.mediaType)
			}
			if got := req.header.Get("Accept"); got != mediaJSON {
				t.Errorf("Accept = %q, want %q", got, mediaJSON)
			}
			if got := req.header.Get("Prefer"); got != "return=minimal" {
				t.Errorf("Prefer = %q, want return=minimal", got)
			}
			if !bytes.Equal(req.body, want) {
				t.Errorf("body differs from the codec's output\n got: %s\nwant: %s", req.body, want)
			}
			if got := req.header.Get("Openehr-Template-Id"); got != wt.TemplateID {
				t.Errorf("openehr-template-id = %q, want %q", got, wt.TemplateID)
			}
			if len(req.query) != 0 {
				t.Errorf("query = %v, want none without WithFormatQuery", req.query)
			}
			if out != nil {
				t.Errorf("returned composition = %+v, want nil under PreferMinimal", out)
			}
			if meta == nil || meta.VersionUID != compositionVUID {
				t.Errorf("VersionUID = %+v, want %q from the ETag", meta, compositionVUID)
			}
		})
		t.Run(f.format.String()+"/representation", func(t *testing.T) {
			fs := newFormatServer(t, http.StatusCreated, f.mediaType, want)
			out, _, err := composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp,
				composition.WithFormat(f.format, wt),
				composition.WithPrefer(transport.PreferRepresentation))
			if err != nil {
				t.Fatalf("Save(%v, representation) error = %v", f.format, err)
			}
			req := fs.only(t)
			if got := req.header.Get("Accept"); got != f.mediaType {
				t.Errorf("Accept = %q, want %q", got, f.mediaType)
			}
			if out == nil {
				t.Fatal("returned composition is nil, want the decoded representation")
			}
			if got := encodeAs(t, f.format, out, wt); !bytes.Equal(got, want) {
				t.Errorf("decoded representation re-encodes differently\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestWriteFormatKeepsOtherHeaders pins the REQ-173 rule that everything a
// write sends and returns besides the body, Content-Type and Accept is what
// it is for canonical JSON: If-Match, Prefer, the audit, lifecycle and
// item-tag headers, and the version metadata. It also pins the acceptance
// case of an Update with FLAT whose representation decodes against the Web
// Template into the returned composition.
func TestWriteFormatKeepsOtherHeaders(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	committer := "Dr Alice"
	opts := []composition.WriteOption{
		composition.WithPrefer(transport.PreferRepresentation),
		composition.WithAuditDetails(&rm.AuditDetails{
			SystemID:   "cdr.example",
			Committer:  rm.PartyIdentified{Name: &committer},
			ChangeType: rm.DVCodedText{DefiningCode: rm.CodePhrase{CodeString: "251"}},
		}),
		composition.WithLifecycleState(openehrclient.LifecycleStateComplete),
		composition.WithObjectItemTags([]openehrclient.ItemTag{{Key: "ward", Value: "4b"}}),
		composition.WithVersionItemTags([]openehrclient.ItemTag{{Key: "source", Value: "import"}}),
		composition.WithTemplateID(wt.TemplateID),
	}
	kept := []string{
		"If-Match", "Prefer", "Openehr-Audit-Details", "Openehr-Version",
		"Openehr-Item-Tag", "Openehr-Version-Item-Tag", "Openehr-Template-Id",
	}
	formats := []struct {
		format    composition.Format
		mediaType string
	}{
		{format: composition.FormatXML, mediaType: mediaXML},
		{format: composition.FormatFlat, mediaType: mediaFlat},
		{format: composition.FormatStructured, mediaType: mediaStructured},
	}
	for _, op := range formatWriteOps {
		status := http.StatusCreated
		if op.method == http.MethodPut {
			status = http.StatusOK
		}
		base := newFormatServer(t, status, mediaJSON, encodeAs(t, composition.FormatJSON, comp, nil))
		_, baseMeta, err := op.do(t.Context(), newClient(t, base.srv), comp, opts...)
		if err != nil {
			t.Fatalf("%s (canonical JSON baseline) error = %v", op.name, err)
		}
		baseReq := base.only(t)
		for _, name := range kept[1:] {
			if baseReq.header.Get(name) == "" {
				t.Fatalf("%s baseline sent no %s; the comparison would prove nothing", op.name, name)
			}
		}
		for _, f := range formats {
			t.Run(op.name+"/"+f.format.String(), func(t *testing.T) {
				body := encodeAs(t, f.format, comp, wt)
				fs := newFormatServer(t, status, f.mediaType, body)
				out, meta, err := op.do(t.Context(), newClient(t, fs.srv), comp,
					append(opts, composition.WithFormat(f.format, wt))...)
				if err != nil {
					t.Fatalf("%s(%v) error = %v", op.name, f.format, err)
				}
				req := fs.only(t)
				if req.method != op.method {
					t.Errorf("method = %q, want %q", req.method, op.method)
				}
				for _, name := range kept {
					if got, want := req.header.Get(name), baseReq.header.Get(name); got != want {
						t.Errorf("%s = %q, want %q as for canonical JSON", name, got, want)
					}
				}
				if got := req.header.Get("Content-Type"); got != f.mediaType {
					t.Errorf("Content-Type = %q, want %q", got, f.mediaType)
				}
				if meta == nil || baseMeta == nil || meta.VersionUID != baseMeta.VersionUID || meta.ETag != baseMeta.ETag {
					t.Errorf("version metadata = %+v, want %+v as for canonical JSON", meta, baseMeta)
				}
				if out == nil {
					t.Fatal("returned composition is nil, want the decoded representation")
				}
				if got := encodeAs(t, f.format, out, wt); !bytes.Equal(got, body) {
					t.Errorf("decoded representation re-encodes differently\n got: %s\nwant: %s", got, body)
				}
			})
		}
	}
}

// TestWriteFormatRefusals pins the REQ-173 refusals on Save and Update: each
// fails with ErrInvalidConfig and issues no request. A FLAT or STRUCTURED
// write with a nil Web Template; a WithTemplateID that differs from the Web
// Template's id; a FLAT write whose two template ids are both empty; a
// Format outside the four.
func TestWriteFormatRefusals(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	noID := withoutTemplateID(wt)
	cases := []struct {
		name string
		opts []composition.WriteOption
	}{
		{name: "FLAT with a nil Web Template", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatFlat, nil),
		}},
		{name: "STRUCTURED with a nil Web Template", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatStructured, nil),
		}},
		{name: "FLAT with a WithTemplateID that differs", opts: []composition.WriteOption{
			composition.WithTemplateID("Other_template.v1"),
			composition.WithFormat(composition.FormatFlat, wt),
		}},
		{name: "STRUCTURED with a WithTemplateID that differs", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatStructured, wt),
			composition.WithTemplateID("Other_template.v1"),
		}},
		{name: "FLAT with both template ids empty", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatFlat, noID),
		}},
		{name: "STRUCTURED with both template ids empty", opts: []composition.WriteOption{
			composition.WithTemplateID(""),
			composition.WithFormat(composition.FormatStructured, noID),
		}},
		{name: "Format one past the four", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatStructured+1, wt),
		}},
		{name: "negative Format", opts: []composition.WriteOption{
			composition.WithFormat(composition.Format(-1), nil),
		}},
	}
	for _, op := range formatWriteOps {
		for _, tc := range cases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				fs := newFormatServer(t, http.StatusCreated, "", nil)
				out, meta, err := op.do(t.Context(), newClient(t, fs.srv), comp, tc.opts...)
				if !errors.Is(err, transport.ErrInvalidConfig) {
					t.Errorf("%s error = %v, want ErrInvalidConfig", op.name, err)
				}
				if out != nil || meta != nil {
					t.Errorf("%s returned (%+v, %+v), want nil, nil", op.name, out, meta)
				}
				if n := fs.count(); n != 0 {
					t.Errorf("server saw %d requests, want 0", n)
				}
			})
		}
	}
}

// TestWriteTemplateID pins the REQ-173 template id rule's accepting arms. A
// FLAT or STRUCTURED write sends openehr-template-id holding WithTemplateID's
// id, or the Web Template's when that option is absent or empty. Canonical
// JSON and XML keep WithTemplateID as it was and never consult the Web
// Template.
func TestWriteTemplateID(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	noID := withoutTemplateID(wt)
	cases := []struct {
		name string
		opts []composition.WriteOption
		want string
	}{
		{name: "FLAT defaults to the Web Template's id", want: wt.TemplateID, opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatFlat, wt),
		}},
		{name: "FLAT with an empty WithTemplateID", want: wt.TemplateID, opts: []composition.WriteOption{
			composition.WithTemplateID(""),
			composition.WithFormat(composition.FormatFlat, wt),
		}},
		{name: "STRUCTURED with an equal WithTemplateID", want: wt.TemplateID, opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatStructured, wt),
			composition.WithTemplateID(wt.TemplateID),
		}},
		{name: "FLAT whose Web Template has no id", want: "Named_template.v1", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatFlat, noID),
			composition.WithTemplateID("Named_template.v1"),
		}},
		{name: "JSON ignores the Web Template's id", want: "Named_template.v1", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatJSON, wt),
			composition.WithTemplateID("Named_template.v1"),
		}},
		{name: "JSON without WithTemplateID sends none", want: "", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatJSON, wt),
		}},
		{name: "XML keeps WithTemplateID", want: "Named_template.v1", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatXML, nil),
			composition.WithTemplateID("Named_template.v1"),
		}},
		{name: "XML without WithTemplateID sends none", want: "", opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatXML, wt),
		}},
	}
	for _, op := range formatWriteOps {
		for _, tc := range cases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				fs := newFormatServer(t, http.StatusCreated, "", nil)
				if _, _, err := op.do(t.Context(), newClient(t, fs.srv), comp, tc.opts...); err != nil {
					t.Fatalf("%s error = %v", op.name, err)
				}
				req := fs.only(t)
				if got := req.header.Get("Openehr-Template-Id"); got != tc.want {
					t.Errorf("openehr-template-id = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// TestWriteAcceptFollowsPrefer pins the REQ-173 response rule: under
// PreferRepresentation the write sends the format's media type as Accept,
// and under any other Prefer it sends application/json, the media type of
// the Identifier body.
func TestWriteAcceptFollowsPrefer(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	formats := []struct {
		format    composition.Format
		mediaType string
	}{
		{format: composition.FormatJSON, mediaType: mediaJSON},
		{format: composition.FormatXML, mediaType: mediaXML},
		{format: composition.FormatFlat, mediaType: mediaFlat},
		{format: composition.FormatStructured, mediaType: mediaStructured},
	}
	prefers := []struct {
		name   string
		prefer transport.Prefer
	}{
		{name: "default", prefer: transport.PreferDefault},
		{name: "minimal", prefer: transport.PreferMinimal},
		{name: "identifier", prefer: transport.PreferIdentifier},
		{name: "representation", prefer: transport.PreferRepresentation},
	}
	for _, op := range formatWriteOps {
		for _, f := range formats {
			for _, p := range prefers {
				t.Run(op.name+"/"+f.format.String()+"/"+p.name, func(t *testing.T) {
					wantAccept := mediaJSON
					var body []byte
					switch p.prefer {
					case transport.PreferRepresentation:
						wantAccept = f.mediaType
						body = encodeAs(t, f.format, comp, wt)
					case transport.PreferIdentifier:
						body = []byte(`{"uid":"` + string(compositionVUID) + `"}`)
					case transport.PreferDefault, transport.PreferMinimal:
					}
					fs := newFormatServer(t, http.StatusCreated, "", body)
					out, meta, err := op.do(t.Context(), newClient(t, fs.srv), comp,
						composition.WithFormat(f.format, wt), composition.WithPrefer(p.prefer))
					if err != nil {
						t.Fatalf("%s(%v, %s) error = %v", op.name, f.format, p.name, err)
					}
					req := fs.only(t)
					if got := req.header.Get("Accept"); got != wantAccept {
						t.Errorf("Accept = %q, want %q", got, wantAccept)
					}
					if p.prefer == transport.PreferRepresentation && out == nil {
						t.Error("returned composition is nil under PreferRepresentation")
					}
					if meta == nil || meta.VersionUID != compositionVUID {
						t.Errorf("VersionUID = %+v, want %q", meta, compositionVUID)
					}
				})
			}
		}
	}
}

// TestWriteFormatQuery pins REQ-173 WithFormatQuery on writes. FLAT and
// STRUCTURED send Content-Type and Accept application/json, the format
// parameter and a templateId parameter holding the resolved template id,
// still send that id as openehr-template-id, and write and read the body in
// the format asked for. Canonical JSON and XML carry no parameter.
func TestWriteFormatQuery(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	simplifiedCases := []struct {
		name       string
		format     composition.Format
		param      string
		wt         *webtemplate.WebTemplate
		templateID string // the WithTemplateID option, empty for none
		wantID     string
	}{
		{name: "FLAT", format: composition.FormatFlat, param: "FLAT", wt: wt, wantID: wt.TemplateID},
		{name: "STRUCTURED", format: composition.FormatStructured, param: "STRUCTURED", wt: wt, wantID: wt.TemplateID},
		{
			name: "FLAT with the id from WithTemplateID", format: composition.FormatFlat, param: "FLAT",
			wt: withoutTemplateID(wt), templateID: "Named_template.v1", wantID: "Named_template.v1",
		},
	}
	prefers := []transport.Prefer{transport.PreferMinimal, transport.PreferRepresentation}
	for _, op := range formatWriteOps {
		for _, tc := range simplifiedCases {
			for _, prefer := range prefers {
				t.Run(op.name+"/"+tc.name+"/"+string(prefer), func(t *testing.T) {
					want := encodeAs(t, tc.format, comp, tc.wt)
					var respBody []byte
					if prefer == transport.PreferRepresentation {
						respBody = want
					}
					fs := newFormatServer(t, http.StatusCreated, mediaJSON, respBody)
					opts := []composition.WriteOption{
						composition.WithFormatQuery(),
						composition.WithFormat(tc.format, tc.wt),
						composition.WithPrefer(prefer),
					}
					if tc.templateID != "" {
						opts = append(opts, composition.WithTemplateID(tc.templateID))
					}
					out, _, err := op.do(t.Context(), newClient(t, fs.srv), comp, opts...)
					if err != nil {
						t.Fatalf("%s(%v, WithFormatQuery) error = %v", op.name, tc.format, err)
					}
					req := fs.only(t)
					if got := req.header.Get("Content-Type"); got != mediaJSON {
						t.Errorf("Content-Type = %q, want %q", got, mediaJSON)
					}
					if got := req.header.Get("Accept"); got != mediaJSON {
						t.Errorf("Accept = %q, want %q", got, mediaJSON)
					}
					if got := req.query["format"]; len(got) != 1 || got[0] != tc.param {
						t.Errorf("format parameter = %q, want [%q]", got, tc.param)
					}
					if got := req.query["templateId"]; len(got) != 1 || got[0] != tc.wantID {
						t.Errorf("templateId parameter = %q, want [%q]", got, tc.wantID)
					}
					if got := req.header.Get("Openehr-Template-Id"); got != tc.wantID {
						t.Errorf("openehr-template-id = %q, want %q", got, tc.wantID)
					}
					if !bytes.Equal(req.body, want) {
						t.Errorf("body differs from the %v codec's output\n got: %s\nwant: %s", tc.format, req.body, want)
					}
					if prefer != transport.PreferRepresentation {
						return
					}
					if out == nil {
						t.Fatal("returned composition is nil, want the decoded representation")
					}
					if got := encodeAs(t, tc.format, out, tc.wt); !bytes.Equal(got, want) {
						t.Errorf("decoded representation re-encodes differently\n got: %s\nwant: %s", got, want)
					}
				})
			}
		}
		canonical := []struct {
			format    composition.Format
			mediaType string
		}{
			{format: composition.FormatJSON, mediaType: mediaJSON},
			{format: composition.FormatXML, mediaType: mediaXML},
		}
		for _, f := range canonical {
			t.Run(op.name+"/"+f.format.String(), func(t *testing.T) {
				fs := newFormatServer(t, http.StatusCreated, "", nil)
				_, _, err := op.do(t.Context(), newClient(t, fs.srv), comp,
					composition.WithFormatQuery(), composition.WithFormat(f.format, nil))
				if err != nil {
					t.Fatalf("%s(%v, WithFormatQuery) error = %v", op.name, f.format, err)
				}
				req := fs.only(t)
				if len(req.query) != 0 {
					t.Errorf("query = %v, want no parameter on %v", req.query, f.format)
				}
				if got := req.header.Get("Content-Type"); got != f.mediaType {
					t.Errorf("Content-Type = %q, want %q", got, f.mediaType)
				}
			})
		}
	}
}

// TestWithFormatLastWins pins that the last WithFormat given decides the
// format (REQ-173).
func TestWithFormatLastWins(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	cases := []struct {
		name      string
		opts      []composition.WriteOption
		mediaType string
	}{
		{name: "XML then FLAT", mediaType: mediaFlat, opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatXML, nil),
			composition.WithFormat(composition.FormatFlat, wt),
		}},
		{name: "FLAT then XML", mediaType: mediaXML, opts: []composition.WriteOption{
			composition.WithFormat(composition.FormatFlat, wt),
			composition.WithFormat(composition.FormatXML, nil),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, http.StatusCreated, "", nil)
			if _, _, err := composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp, tc.opts...); err != nil {
				t.Fatalf("Save error = %v", err)
			}
			if got := fs.only(t).header.Get("Content-Type"); got != tc.mediaType {
				t.Errorf("Content-Type = %q, want %q", got, tc.mediaType)
			}
		})
	}
}

// TestWriteSerialiseFailureSendsNothing pins REQ-173: a composition the
// codec cannot serialise fails the write before any request. The FLAT and
// STRUCTURED codecs refuse an EVENT_CONTEXT setting that carries a value but
// no code.
func TestWriteSerialiseFailureSendsNothing(t *testing.T) {
	wt, _ := loadFormatFixture(t)
	bad := &rm.Composition{Context: &rm.EventContext{Setting: rm.DVCodedText{Value: "home"}}}
	for _, op := range formatWriteOps {
		for _, f := range []composition.Format{composition.FormatFlat, composition.FormatStructured} {
			t.Run(op.name+"/"+f.String(), func(t *testing.T) {
				fs := newFormatServer(t, http.StatusCreated, "", nil)
				out, meta, err := op.do(t.Context(), newClient(t, fs.srv), bad, composition.WithFormat(f, wt))
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
					t.Errorf("%s(%v) error = %v, want the codec's ErrUnsupportedDatatype", op.name, f, err)
				}
				if err != nil && !strings.Contains(err.Error(), "marshal body") {
					t.Errorf("%s(%v) error = %q, want it to name the body marshal", op.name, f, err)
				}
				if out != nil || meta != nil {
					t.Errorf("%s returned (%+v, %+v), want nil, nil", op.name, out, meta)
				}
				if n := fs.count(); n != 0 {
					t.Errorf("server saw %d requests, want 0", n)
				}
			})
		}
	}
}

// TestWriteRepresentationDecodeFailureIsCommitted pins REQ-173 under
// REQ-094: a representation body that does not decode in the format asked
// for is a committed write with an unusable representation, carrying the
// version metadata, never a failed write.
func TestWriteRepresentationDecodeFailureIsCommitted(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	formats := []struct {
		format composition.Format
		body   string
	}{
		{format: composition.FormatXML, body: "<composition"},
		{format: composition.FormatFlat, body: "not json"},
		{format: composition.FormatStructured, body: "[1, 2]"},
	}
	for _, op := range formatWriteOps {
		for _, f := range formats {
			t.Run(op.name+"/"+f.format.String(), func(t *testing.T) {
				fs := newFormatServer(t, http.StatusCreated, f.format.MediaType(), []byte(f.body))
				out, meta, err := op.do(t.Context(), newClient(t, fs.srv), comp,
					composition.WithFormat(f.format, wt),
					composition.WithPrefer(transport.PreferRepresentation))
				nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
				if !ok || nre == nil {
					t.Fatalf("%s(%v) error = %v, want *NoRepresentationError", op.name, f.format, err)
				}
				if _, wire := errors.AsType[*transport.WireError](err); wire {
					t.Errorf("%s(%v) error is a WireError, want a committed write", op.name, f.format)
				}
				if out != nil {
					t.Errorf("returned composition = %+v, want nil", out)
				}
				if meta == nil || meta.VersionUID != compositionVUID || nre.Meta == nil || nre.Meta.VersionUID != compositionVUID {
					t.Errorf("version metadata = %+v (error carries %+v), want VersionUID %q", meta, nre.Meta, compositionVUID)
				}
			})
		}
	}
}

// TestWriteSimplifiedDecodeOptions pins REQ-173 Simplified decode options on
// writes. Under PreferRepresentation a FLAT or STRUCTURED Save or Update hands
// the decode options of WithFormat, unchanged, to the decode of the
// representation, and hands it none when WithFormat has none. With
// simplified.WithTemplate the returned composition carries the names of the
// nodes below the root; without a decode option it carries none.
func TestWriteSimplifiedDecodeOptions(t *testing.T) {
	compiled, wt, comp := loadCompiledFormatFixture(t)
	cases := []struct {
		name     string
		opts     []simplified.Option
		wantName string
	}{
		{name: "with the compiled template", opts: []simplified.Option{simplified.WithTemplate(compiled)}, wantName: fixtureEntryName},
		{name: "without decode options", wantName: ""},
	}
	for _, op := range formatWriteOps {
		status := http.StatusCreated
		if op.method == http.MethodPut {
			status = http.StatusOK
		}
		for _, f := range []composition.Format{composition.FormatFlat, composition.FormatStructured} {
			for _, tc := range cases {
				t.Run(op.name+"/"+f.String()+"/"+tc.name, func(t *testing.T) {
					body := encodeAs(t, f, comp, wt)
					fs := newFormatServer(t, status, f.MediaType(), body)
					out, _, err := op.do(t.Context(), newClient(t, fs.srv), comp,
						composition.WithFormat(f, wt, tc.opts...),
						composition.WithPrefer(transport.PreferRepresentation))
					if err != nil {
						t.Fatalf("%s(%v, representation) error = %v", op.name, f, err)
					}
					if got := firstEntryName(t, out); got != tc.wantName {
						t.Errorf("first content entry name = %q, want %q", got, tc.wantName)
					}
					sameComposition(t, "returned composition", out, decodeSimplified(t, f, body, wt, tc.opts...))
				})
			}
		}
	}
}

// TestWriteDecodeOptionsGovernRepresentation pins REQ-173 Simplified decode
// options with REQ-094: the strict and unmatched options given to WithFormat
// reach the decode of a FLAT representation. A representation holding
// unmatched content decodes without a decode option, is a committed write
// with an unusable representation under simplified.WithStrict, and shows its
// unmatched key to simplified.WithUnmatched. Canonical JSON and XML ignore
// the decode options.
func TestWriteDecodeOptionsGovernRepresentation(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	extra := withUnmatchedKey(t, encodeAs(t, composition.FormatFlat, comp, wt))
	save := func(t *testing.T, contentType string, body []byte, opt composition.WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
		t.Helper()
		fs := newFormatServer(t, http.StatusCreated, contentType, body)
		return composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp,
			opt, composition.WithPrefer(transport.PreferRepresentation))
	}
	t.Run("FLAT without decode options", func(t *testing.T) {
		out, _, err := save(t, mediaFlat, extra, composition.WithFormat(composition.FormatFlat, wt))
		if err != nil || out == nil {
			t.Fatalf("Save(FLAT) of a representation with an unmatched key = (%v, %v), want it decoded", out, err)
		}
	})
	t.Run("FLAT with WithStrict", func(t *testing.T) {
		out, meta, err := save(t, mediaFlat, extra, composition.WithFormat(composition.FormatFlat, wt, simplified.WithStrict()))
		nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
		if !ok || nre == nil {
			t.Fatalf("Save(FLAT, WithStrict) error = %v, want *NoRepresentationError", err)
		}
		if !errors.Is(err, simplified.ErrUnknownPath) {
			t.Errorf("Save(FLAT, WithStrict) error = %v, want it to wrap simplified.ErrUnknownPath", err)
		}
		if out != nil {
			t.Errorf("returned composition = %+v, want nil", out)
		}
		if meta == nil || meta.VersionUID != compositionVUID {
			t.Errorf("version metadata = %+v, want VersionUID %q", meta, compositionVUID)
		}
	})
	t.Run("FLAT with WithUnmatched", func(t *testing.T) {
		var seen []string
		out, _, err := save(t, mediaFlat, extra, composition.WithFormat(composition.FormatFlat, wt,
			simplified.WithUnmatched(func(path string) { seen = append(seen, path) })))
		if err != nil || out == nil {
			t.Fatalf("Save(FLAT, WithUnmatched) = (%v, %v), want it decoded", out, err)
		}
		if len(seen) != 1 || seen[0] != unmatchedFlatKey {
			t.Errorf("WithUnmatched saw %q, want [%q]", seen, unmatchedFlatKey)
		}
	})
	for _, f := range []composition.Format{composition.FormatJSON, composition.FormatXML} {
		t.Run(f.String()+" with WithStrict", func(t *testing.T) {
			out, _, err := save(t, f.MediaType(), encodeAs(t, f, comp, nil),
				composition.WithFormat(f, nil, simplified.WithStrict()))
			if err != nil {
				t.Fatalf("Save(%v, WithStrict) error = %v", f, err)
			}
			sameComposition(t, "returned composition", out, comp)
		})
	}
}

// TestWithFormatCopiesDecodeOptions pins that WithFormat keeps its own copy of
// the decode options (REQ-173): a caller who changes its slice after building
// the option does not change what the representation is decoded with.
func TestWithFormatCopiesDecodeOptions(t *testing.T) {
	compiled, wt, comp := loadCompiledFormatFixture(t)
	opts := []simplified.Option{simplified.WithTemplate(compiled)}
	format := composition.WithFormat(composition.FormatFlat, wt, opts...)
	opts[0] = simplified.WithStrict()
	fs := newFormatServer(t, http.StatusCreated, mediaFlat, withUnmatchedKey(t, encodeAs(t, composition.FormatFlat, comp, wt)))
	out, _, err := composition.Save(t.Context(), newClient(t, fs.srv), ehrIDFixture, comp,
		format, composition.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		t.Fatalf("Save error = %v, want the representation decoded with the options given to WithFormat", err)
	}
	if got := firstEntryName(t, out); got != fixtureEntryName {
		t.Errorf("first content entry name = %q, want %q from the compiled template", got, fixtureEntryName)
	}
}
