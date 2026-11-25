package demographic_test

// format_write_test.go — REQ-173 Writes at the demographic leaf: Create and
// Update in canonical JSON or canonical XML, with the response rule under
// each Prefer and the refusals before any request.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The media types REQ-173 names, spelled out so the tests pin the wire
// values rather than read them back from the package.
const (
	mediaJSON = "application/json"
	mediaXML  = "application/xml"
)

// wireRequest is what the test server saw of one request.
type wireRequest struct {
	method string
	path   string
	header http.Header
	query  url.Values
	body   []byte
}

// recorder records every request it receives and answers each with status,
// an ETag naming personVersion, the given Content-Type when non-empty, and
// body.
type recorder struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []wireRequest
}

func newRecorder(t *testing.T, status int, contentType string, body []byte) *recorder {
	t.Helper()
	rec := &recorder{}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, wireRequest{
			method: r.Method, path: r.URL.Path, header: r.Header.Clone(), query: r.URL.Query(), body: b,
		})
		rec.mu.Unlock()
		w.Header().Set("ETag", `"`+personVersion+`"`)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(rec.srv.Close)
	return rec
}

func (rec *recorder) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.reqs)
}

// only returns the one request the server saw, failing the test otherwise.
func (rec *recorder) only(t *testing.T) wireRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.reqs) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(rec.reqs))
	}
	return rec.reqs[0]
}

// partyWriteOp runs Create or Update with the given options against c.
// Update addresses a party of type typ; Create derives it from the value.
type partyWriteOp struct {
	name   string
	method string
	do     func(ctx context.Context, c *transport.Client, typ demographic.Type, party rm.Party, opts ...demographic.WriteOption) (rm.Party, *openehrclient.VersionMetadata, error)
}

var partyWriteOps = []partyWriteOp{
	{
		name:   "Create",
		method: http.MethodPost,
		do: func(ctx context.Context, c *transport.Client, _ demographic.Type, party rm.Party, opts ...demographic.WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
			return demographic.Create(ctx, c, party, opts...)
		},
	},
	{
		name:   "Update",
		method: http.MethodPut,
		do: func(ctx context.Context, c *transport.Client, typ demographic.Type, party rm.Party, opts ...demographic.WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
			return demographic.Update(ctx, c, typ, personVOID, personVersion, party, opts...)
		},
	},
}

// partyFixtures are the five corpus parties, by type, with their canonical
// JSON and canonical XML files.
var partyFixtures = []struct {
	typ      demographic.Type
	json     string
	xml      string
	bmmName  string
	wantType string
}{
	{typ: demographic.Person, json: "person.json", xml: "person.xml", bmmName: "PERSON", wantType: "*rm.Person"},
	{typ: demographic.Organisation, json: "organisation.json", xml: "organisation.xml", bmmName: "ORGANISATION", wantType: "*rm.Organisation"},
	{typ: demographic.Group, json: "group.json", xml: "group.xml", bmmName: "GROUP", wantType: "*rm.Group"},
	{typ: demographic.Agent, json: "agent.json", xml: "agent.xml", bmmName: "AGENT", wantType: "*rm.Agent"},
	{typ: demographic.Role, json: "role.json", xml: "role.xml", bmmName: "ROLE", wantType: "*rm.Role"},
}

// jsonParty decodes a corpus party from its canonical JSON file.
func jsonParty(t *testing.T, name string) rm.Party {
	t.Helper()
	p, err := typereg.DecodeAs[rm.Party](fixture(t, name))
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return p
}

// sameParty fails the test unless got and want have the same concrete type
// and the same canonical JSON.
func sameParty(t *testing.T, label string, got, want rm.Party) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("%s: got %T, want %T", label, got, want)
	}
	gj, err := canjson.Marshal(got)
	if err != nil {
		t.Fatalf("%s: encode got: %v", label, err)
	}
	wj, err := canjson.Marshal(want)
	if err != nil {
		t.Fatalf("%s: encode want: %v", label, err)
	}
	if !bytes.Equal(gj, wj) {
		t.Errorf("%s differs:\n got %s\nwant %s", label, gj, wj)
	}
}

// withRootType returns body, a canonical XML document canxml wrote, with an
// xsi:type naming class on its root element.
func withRootType(t *testing.T, body []byte, class string) []byte {
	t.Helper()
	end := bytes.IndexByte(body, ' ')
	if end < 0 || body[0] != '<' {
		t.Fatalf("withRootType: no root start tag in %q", body)
	}
	out := append([]byte{}, body[:end]...)
	out = append(out, ` xsi:type="`+class+`"`...)
	return append(out, body[end:]...)
}

// TestFormatMediaTypeAndName pins REQ-173 Formats at the demographic leaf:
// the zero value is canonical JSON, the two formats send their own media
// types, and a value outside the two names no media type.
func TestFormatMediaTypeAndName(t *testing.T) {
	var zero demographic.Format
	if zero != demographic.FormatJSON {
		t.Errorf("zero Format = %v, want FormatJSON", zero)
	}
	cases := []struct {
		format    demographic.Format
		mediaType string
		name      string
	}{
		{format: demographic.FormatJSON, mediaType: mediaJSON, name: "JSON"},
		{format: demographic.FormatXML, mediaType: mediaXML, name: "XML"},
		{format: demographic.FormatXML + 1, mediaType: "", name: "Format(2)"},
		{format: demographic.Format(-1), mediaType: "", name: "Format(-1)"},
	}
	for _, tc := range cases {
		if got := tc.format.MediaType(); got != tc.mediaType {
			t.Errorf("Format(%d).MediaType() = %q, want %q", int(tc.format), got, tc.mediaType)
		}
		if got := tc.format.String(); got != tc.name {
			t.Errorf("Format(%d).String() = %q, want %q", int(tc.format), got, tc.name)
		}
	}
}

// TestWriteXMLRepresentation pins the REQ-173 canonical XML write for every
// PARTY type: Create and Update with WithFormat(FormatXML) under
// PreferRepresentation send application/xml as Content-Type and Accept, a
// body canonical XML decodes back to the party given, and return the party
// decoded from the XML body, as the class the call names.
func TestWriteXMLRepresentation(t *testing.T) {
	for _, op := range partyWriteOps {
		for _, pf := range partyFixtures {
			t.Run(op.name+"/"+string(pf.typ), func(t *testing.T) {
				party := jsonParty(t, pf.json)
				rec := newRecorder(t, http.StatusOK, mediaXML, fixture(t, pf.xml))

				out, meta, err := op.do(t.Context(), newClient(t, rec.srv), pf.typ, party,
					demographic.WithPrefer(transport.PreferRepresentation),
					demographic.WithFormat(demographic.FormatXML))
				if err != nil {
					t.Fatalf("%s(XML, representation) error = %v", op.name, err)
				}
				req := rec.only(t)
				if req.method != op.method {
					t.Errorf("method = %s, want %s", req.method, op.method)
				}
				if got := req.header.Get("Content-Type"); got != mediaXML {
					t.Errorf("Content-Type = %q, want %q", got, mediaXML)
				}
				if got := req.header.Get("Accept"); got != mediaXML {
					t.Errorf("Accept = %q, want %q", got, mediaXML)
				}
				ctor, ok := typereg.Default.Lookup(pf.bmmName)
				if !ok {
					t.Fatalf("no constructor for %s", pf.bmmName)
				}
				sent := ctor()
				if err := canxml.Unmarshal(req.body, sent, canxml.WithStrict()); err != nil {
					t.Fatalf("request body is not canonical XML: %v\n%s", err, req.body)
				}
				sentParty, ok := sent.(rm.Party)
				if !ok {
					t.Fatalf("decoded request body %T is not a PARTY", sent)
				}
				sameParty(t, "request body", sentParty, party)
				sameParty(t, "returned party", out, party)
				if meta == nil || meta.VersionUID != personVersion {
					t.Errorf("VersionUID = %+v, want %q", meta, personVersion)
				}
			})
		}
	}
}

// TestWriteXMLValueParty pins that a party passed by value, which the
// resource path accepts as it accepts a pointer, is written in canonical XML
// as the pointer would be.
func TestWriteXMLValueParty(t *testing.T) {
	person := rm.Person{Name: &rm.DVText{Value: "Jane Doe"}, ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1"}
	rec := newRecorder(t, http.StatusCreated, "", nil)
	if _, _, err := demographic.Create(t.Context(), newClient(t, rec.srv), person,
		demographic.WithFormat(demographic.FormatXML)); err != nil {
		t.Fatalf("Create(value PERSON, XML) error = %v", err)
	}
	req := rec.only(t)
	if req.path != "/openehr/v1/demographic/person" {
		t.Errorf("path = %q, want the person resource", req.path)
	}
	var sent rm.Person
	if err := canxml.Unmarshal(req.body, &sent, canxml.WithStrict()); err != nil {
		t.Fatalf("request body is not canonical XML: %v\n%s", err, req.body)
	}
	sameParty(t, "request body", &sent, &person)
}

// TestWriteAcceptFollowsPrefer pins the REQ-173 response rule at the
// demographic leaf: under PreferRepresentation the write asks for the
// format's media type, under any other Prefer for application/json, the
// media type of the Identifier body; Content-Type is the format's in both.
func TestWriteAcceptFollowsPrefer(t *testing.T) {
	identifier := []byte(`{"uid":"` + personVersion + `"}`)
	prefers := []struct {
		prefer     transport.Prefer
		body       func(f demographic.Format) []byte
		wantFormat bool // Accept is the format's media type
	}{
		{prefer: transport.PreferMinimal, body: func(demographic.Format) []byte { return nil }},
		{prefer: transport.PreferIdentifier, body: func(demographic.Format) []byte { return identifier }},
		{prefer: transport.PreferRepresentation, wantFormat: true, body: func(f demographic.Format) []byte {
			if f == demographic.FormatXML {
				return fixture(t, "person.xml")
			}
			return fixture(t, "person.json")
		}},
	}
	party := jsonParty(t, "person.json")
	for _, op := range partyWriteOps {
		for _, f := range []demographic.Format{demographic.FormatJSON, demographic.FormatXML} {
			for _, p := range prefers {
				t.Run(op.name+"/"+f.String()+"/"+string(p.prefer), func(t *testing.T) {
					rec := newRecorder(t, http.StatusOK, "", p.body(f))
					_, meta, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, party,
						demographic.WithFormat(f), demographic.WithPrefer(p.prefer))
					if err != nil {
						t.Fatalf("%s(%v, %s) error = %v", op.name, f, p.prefer, err)
					}
					req := rec.only(t)
					if got := req.header.Get("Content-Type"); got != f.MediaType() {
						t.Errorf("Content-Type = %q, want %q", got, f.MediaType())
					}
					wantAccept := mediaJSON
					if p.wantFormat {
						wantAccept = f.MediaType()
					}
					if got := req.header.Get("Accept"); got != wantAccept {
						t.Errorf("Accept = %q, want %q", got, wantAccept)
					}
					if meta == nil || meta.VersionUID != personVersion {
						t.Errorf("VersionUID = %+v, want %q", meta, personVersion)
					}
				})
			}
		}
	}
}

// TestWriteFormatKeepsOtherHeaders pins the REQ-173 rule that everything a
// write sends besides the body and its media types is what canonical JSON
// sends: If-Match, Prefer, the audit and lifecycle headers, and the path.
func TestWriteFormatKeepsOtherHeaders(t *testing.T) {
	committer := "alice"
	audit := &rm.AuditDetails{
		SystemID:  "cdr.example",
		Committer: rm.PartyIdentified{Name: &committer},
		ChangeType: rm.DVCodedText{
			DVText:       rm.DVText{Value: "modification"},
			DefiningCode: rm.CodePhrase{CodeString: "251"},
		},
	}
	party := jsonParty(t, "person.json")
	headers := []string{"If-Match", "Prefer", "Openehr-Audit-Details", "Openehr-Version"}
	for _, op := range partyWriteOps {
		t.Run(op.name, func(t *testing.T) {
			seen := map[demographic.Format]wireRequest{}
			for _, f := range []demographic.Format{demographic.FormatJSON, demographic.FormatXML} {
				rec := newRecorder(t, http.StatusOK, "", nil)
				if _, _, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, party,
					demographic.WithFormat(f),
					demographic.WithPrefer(transport.PreferMinimal),
					demographic.WithAuditDetails(audit),
					demographic.WithLifecycleState(openehrclient.LifecycleStateComplete)); err != nil {
					t.Fatalf("%s(%v) error = %v", op.name, f, err)
				}
				seen[f] = rec.only(t)
			}
			js, xs := seen[demographic.FormatJSON], seen[demographic.FormatXML]
			if js.path != xs.path || js.method != xs.method {
				t.Errorf("XML write %s %s, JSON write %s %s", xs.method, xs.path, js.method, js.path)
			}
			for _, h := range headers {
				if got, want := xs.header.Get(h), js.header.Get(h); got != want {
					t.Errorf("XML %s = %q, JSON sends %q", h, got, want)
				}
			}
			if js.header.Get("Openehr-Audit-Details") == "" || js.header.Get("Openehr-Version") == "" {
				t.Errorf("JSON write sent no audit or lifecycle header: %v", js.header)
			}
		})
	}
}

// TestWriteFormatRefusals pins the REQ-173 refusal of a format outside the
// two: Create and Update fail with ErrInvalidConfig and issue no request.
func TestWriteFormatRefusals(t *testing.T) {
	party := jsonParty(t, "person.json")
	for _, op := range partyWriteOps {
		for _, f := range []demographic.Format{demographic.FormatXML + 1, demographic.Format(-1)} {
			t.Run(op.name+"/"+f.String(), func(t *testing.T) {
				rec := newRecorder(t, http.StatusCreated, "", nil)
				out, meta, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, party, demographic.WithFormat(f))
				if !errors.Is(err, transport.ErrInvalidConfig) {
					t.Errorf("%s(%v) error = %v, want ErrInvalidConfig", op.name, f, err)
				}
				if out != nil || meta != nil {
					t.Errorf("%s(%v) returned (%v, %v), want nil, nil", op.name, f, out, meta)
				}
				if n := rec.count(); n != 0 {
					t.Errorf("server saw %d requests, want 0", n)
				}
			})
		}
	}
}

// TestWriteTypedNilPartyRefused pins that Create and Update refuse a PARTY
// holding a nil pointer, in both formats, with ErrInvalidConfig and no
// request: the XML encoder would panic on it, and the JSON one would send a
// null body.
func TestWriteTypedNilPartyRefused(t *testing.T) {
	parties := []struct {
		typ   demographic.Type
		party rm.Party
	}{
		{demographic.Person, (*rm.Person)(nil)},
		{demographic.Organisation, (*rm.Organisation)(nil)},
		{demographic.Group, (*rm.Group)(nil)},
		{demographic.Agent, (*rm.Agent)(nil)},
		{demographic.Role, (*rm.Role)(nil)},
	}
	for _, op := range partyWriteOps {
		for _, f := range []demographic.Format{demographic.FormatJSON, demographic.FormatXML} {
			for _, p := range parties {
				t.Run(op.name+"/"+f.String()+"/"+string(p.typ), func(t *testing.T) {
					rec := newRecorder(t, http.StatusCreated, "", nil)
					out, meta, err := op.do(t.Context(), newClient(t, rec.srv), p.typ, p.party, demographic.WithFormat(f))
					if !errors.Is(err, transport.ErrInvalidConfig) {
						t.Errorf("%s(%T(nil), %v) error = %v, want ErrInvalidConfig", op.name, p.party, f, err)
					}
					if out != nil || meta != nil {
						t.Errorf("%s(%T(nil), %v) returned (%v, %v), want nil, nil", op.name, p.party, f, out, meta)
					}
					if n := rec.count(); n != 0 {
						t.Errorf("server saw %d requests, want 0", n)
					}
				})
			}
		}
	}
}

// TestWriteSerialiseFailureIsEncodeError pins REQ-173: a party the codec
// cannot serialise fails the write before any request, as an *EncodeError
// that names the format and wraps the codec's error, never as
// ErrInvalidConfig. Canonical JSON refuses a NaN magnitude with
// canjson.ErrInvalidValue. Canonical XML takes the same path, but no party
// the typed RM can build makes its encoder fail, so it has no row.
func TestWriteSerialiseFailureIsEncodeError(t *testing.T) {
	nan := &rm.Person{
		Name: &rm.DVText{Value: "Jane Doe"},
		Details: &rm.ItemTree{Items: []rm.Item{
			&rm.Element{Value: &rm.DVQuantity{Magnitude: rm.Real(math.NaN()), Units: "kg"}},
		}},
	}
	for _, op := range partyWriteOps {
		for _, opts := range [][]demographic.WriteOption{nil, {demographic.WithFormat(demographic.FormatJSON)}} {
			t.Run(op.name, func(t *testing.T) {
				rec := newRecorder(t, http.StatusCreated, "", nil)
				out, meta, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, nan, opts...)
				encErr, ok := errors.AsType[*demographic.EncodeError](err)
				switch {
				case !ok || encErr == nil:
					t.Errorf("%s error = %v, want an *EncodeError", op.name, err)
				case encErr.Format != demographic.FormatJSON:
					t.Errorf("%s EncodeError.Format = %v, want JSON", op.name, encErr.Format)
				}
				if !errors.Is(err, canjson.ErrInvalidValue) {
					t.Errorf("%s error = %v, want it to wrap canjson.ErrInvalidValue", op.name, err)
				}
				if errors.Is(err, transport.ErrInvalidConfig) {
					t.Errorf("%s error = %v, want a codec failure, not ErrInvalidConfig", op.name, err)
				}
				if out != nil || meta != nil {
					t.Errorf("%s returned (%v, %v), want nil, nil", op.name, out, meta)
				}
				if n := rec.count(); n != 0 {
					t.Errorf("server saw %d requests, want 0", n)
				}
			})
		}
	}
}

// TestEncodeErrorText pins the EncodeError text and unwrap, a nil receiver
// included.
func TestEncodeErrorText(t *testing.T) {
	cause := errors.New("cause")
	e := &demographic.EncodeError{Format: demographic.FormatXML, Err: cause}
	if got, want := e.Error(), "encode the party as XML: cause"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is(EncodeError, cause) = false, want true")
	}
	var nilErr *demographic.EncodeError
	if got, want := nilErr.Error(), "encode the party as JSON"; got != want {
		t.Errorf("nil Error() = %q, want %q", got, want)
	}
	if nilErr.Unwrap() != nil {
		t.Error("nil Unwrap() != nil")
	}
}

// TestWriteXMLRepresentationDecodeFailureIsCommitted pins REQ-173 with
// REQ-151: an XML representation body that does not decode, or
// whose root xsi:type names another class than the one written, is a
// committed write with an unusable representation, carrying the version
// metadata, never a failed write.
func TestWriteXMLRepresentationDecodeFailureIsCommitted(t *testing.T) {
	bodies := []struct {
		name  string
		body  []byte
		cause error
	}{
		{name: "truncated", body: []byte("<person")},
		{name: "other class", body: withRootType(t, fixture(t, "person.xml"), "ORGANISATION"), cause: typereg.ErrTypeMismatch},
	}
	party := jsonParty(t, "person.json")
	for _, op := range partyWriteOps {
		for _, b := range bodies {
			t.Run(op.name+"/"+b.name, func(t *testing.T) {
				rec := newRecorder(t, http.StatusCreated, mediaXML, b.body)
				out, meta, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, party,
					demographic.WithFormat(demographic.FormatXML),
					demographic.WithPrefer(transport.PreferRepresentation))
				nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
				if !ok || nre == nil {
					t.Fatalf("%s error = %v, want *NoRepresentationError", op.name, err)
				}
				if b.cause != nil && !errors.Is(err, b.cause) {
					t.Errorf("%s error = %v, want it to wrap %v", op.name, err, b.cause)
				}
				if out != nil {
					t.Errorf("returned party = %T, want nil", out)
				}
				if meta == nil || meta.VersionUID != personVersion || nre.Meta == nil {
					t.Errorf("version metadata = %+v (error carries %+v), want VersionUID %q", meta, nre.Meta, personVersion)
				}
			})
		}
	}
}

// TestWriteJSONRepresentationOfAnotherClassIsCommitted pins REQ-173 with
// REQ-151: a canonical JSON representation body whose _type names another
// class than the one written is a committed write with an unusable
// representation wrapping typereg.ErrTypeMismatch, carrying the version
// metadata, never a failed write or a party of the wrong class.
func TestWriteJSONRepresentationOfAnotherClassIsCommitted(t *testing.T) {
	party := jsonParty(t, "person.json")
	for _, op := range partyWriteOps {
		t.Run(op.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusCreated, mediaJSON, fixture(t, "organisation.json"))
			out, meta, err := op.do(t.Context(), newClient(t, rec.srv), demographic.Person, party,
				demographic.WithPrefer(transport.PreferRepresentation))
			nre, ok := errors.AsType[*openehrclient.NoRepresentationError](err)
			if !ok || nre == nil {
				t.Fatalf("%s error = %v, want *NoRepresentationError", op.name, err)
			}
			if !errors.Is(err, typereg.ErrTypeMismatch) {
				t.Errorf("%s error = %v, want it to wrap %v", op.name, err, typereg.ErrTypeMismatch)
			}
			if out != nil {
				t.Errorf("returned party = %T, want nil", out)
			}
			if meta == nil || meta.VersionUID != personVersion || nre.Meta == nil {
				t.Errorf("version metadata = %+v (error carries %+v), want VersionUID %q", meta, nre.Meta, personVersion)
			}
		})
	}
}

// TestWriteXMLRootTypeOfTheClassIsAccepted pins that a root xsi:type naming
// the class written is read as that class.
func TestWriteXMLRootTypeOfTheClassIsAccepted(t *testing.T) {
	party := jsonParty(t, "person.json")
	body := withRootType(t, fixture(t, "person.xml"), "PERSON")
	if !strings.Contains(string(body), `<person xsi:type="PERSON" `) {
		t.Fatalf("withRootType wrote %s", body)
	}
	rec := newRecorder(t, http.StatusCreated, mediaXML, body)
	out, _, err := demographic.Create(t.Context(), newClient(t, rec.srv), party,
		demographic.WithFormat(demographic.FormatXML),
		demographic.WithPrefer(transport.PreferRepresentation))
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	sameParty(t, "returned party", out, party)
}
