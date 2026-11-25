package demographic_test

// versioned_format_test.go — REQ-173 Reads and REQ-174 Representation at the
// versioned_party reads: canonical JSON or canonical XML, the raw body, and
// the outcomes REQ-151 keeps in both formats.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// versionedRead is one of the five versioned_party reads, called with
// options; result is what it returned, or nil.
type versionedRead struct {
	name    string
	path    string
	json    string
	xml     string
	rootBMM string
	call    func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error)
}

// nilIfNil returns nil for a nil *T, so a typed nil compares equal to nil.
func nilIfNil[T any](v *T) any {
	if v == nil {
		return nil
	}
	return v
}

func versionedReads() []versionedRead {
	at := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	base := "/openehr/v1/demographic/versioned_party/" + personVOID
	return []versionedRead{
		{
			name: "GetVersionedParty", path: base, json: "versioned_party.json", xml: "versioned_party.xml", rootBMM: "VERSIONED_PARTY",
			call: func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error) {
				out, meta, err := demographic.GetVersionedParty(t.Context(), c, personVOID, opts...)
				return nilIfNil(out), meta, err
			},
		},
		{
			name: "GetRevisionHistory", path: base + "/revision_history", json: "revision_history.json", xml: "revision_history.xml", rootBMM: "REVISION_HISTORY",
			call: func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error) {
				out, meta, err := demographic.GetRevisionHistory(t.Context(), c, personVOID, opts...)
				return nilIfNil(out), meta, err
			},
		},
		{
			name: "GetVersion", path: base + "/version", json: "original_version.json", xml: "original_version.xml", rootBMM: "ORIGINAL_VERSION",
			call: func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error) {
				out, meta, err := demographic.GetVersion(t.Context(), c, personVOID, opts...)
				return nilIfNil(out), meta, err
			},
		},
		{
			name: "GetVersionAtTime", path: base + "/version", json: "original_version.json", xml: "original_version.xml", rootBMM: "ORIGINAL_VERSION",
			call: func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error) {
				out, meta, err := demographic.GetVersionAtTime(t.Context(), c, personVOID, at, opts...)
				return nilIfNil(out), meta, err
			},
		},
		{
			name: "GetVersionByID", path: base + "/version/" + personVersion, json: "original_version.json", xml: "original_version.xml", rootBMM: "ORIGINAL_VERSION",
			call: func(t *testing.T, c *transport.Client, opts ...demographic.GetOption) (any, *openehrclient.VersionMetadata, error) {
				out, meta, err := demographic.GetVersionByID(t.Context(), c, personVOID, personVersion, opts...)
				return nilIfNil(out), meta, err
			},
		},
	}
}

// sameVersionedResult fails the test unless got, read in XML, carries what
// want, read in canonical JSON from the twin fixture, carries.
func sameVersionedResult(t *testing.T, got, want any) {
	t.Helper()
	switch w := want.(type) {
	case *rm.VersionedParty:
		g, ok := got.(*rm.VersionedParty)
		if !ok || g.UID.Value != w.UID.Value || g.TimeCreated.Value != w.TimeCreated.Value {
			t.Errorf("VERSIONED_PARTY = %+v, want %+v", got, w)
		}
	case *rm.RevisionHistory:
		g, ok := got.(*rm.RevisionHistory)
		if !ok || len(g.Items) != len(w.Items) || len(g.Items) == 0 || g.Items[0].VersionID.Value != w.Items[0].VersionID.Value {
			t.Errorf("REVISION_HISTORY = %+v, want %+v", got, w)
		}
	case *demographic.PartyVersion:
		g, ok := got.(*demographic.PartyVersion)
		if !ok {
			t.Fatalf("version read returned %T, want *PartyVersion", got)
		}
		if g.UID.Value != w.UID.Value || g.LifecycleState.Value != w.LifecycleState.Value {
			t.Errorf("version = uid %q, lifecycle %q; want %q, %q", g.UID.Value, g.LifecycleState.Value, w.UID.Value, w.LifecycleState.Value)
		}
		if g.PrecedingVersionUID == nil || g.PrecedingVersionUID.Value != w.PrecedingVersionUID.Value {
			t.Errorf("preceding_version_uid = %+v, want %+v", g.PrecedingVersionUID, w.PrecedingVersionUID)
		}
		if g.CommitAudit == nil || g.Contribution == nil {
			t.Errorf("commit_audit %v, contribution %v: want both", g.CommitAudit, g.Contribution)
		}
		sameParty(t, "VERSION data", g.Party, w.Party)
	default:
		t.Fatalf("unexpected want %T", want)
	}
}

// TestVersionedReadsXML pins REQ-173 Reads with REQ-174 Representation at
// the versioned_party reads: with WithGetFormat(FormatXML) each sends
// application/xml as Accept and decodes the canonical XML body into what the
// canonical JSON body decodes to, the ORIGINAL_VERSION<PARTY> data included.
// With no option each sends application/json.
func TestVersionedReadsXML(t *testing.T) {
	for _, rd := range versionedReads() {
		t.Run(rd.name, func(t *testing.T) {
			jrec := newRecorder(t, http.StatusOK, mediaJSON, fixture(t, rd.json))
			want, _, err := rd.call(t, newClient(t, jrec.srv))
			if err != nil {
				t.Fatalf("%s(JSON) error = %v", rd.name, err)
			}
			if accept := jrec.only(t).header.Get("Accept"); accept != mediaJSON {
				t.Errorf("%s with no option: Accept = %q, want %q", rd.name, accept, mediaJSON)
			}

			xrec := newRecorder(t, http.StatusOK, mediaXML, fixture(t, rd.xml))
			got, meta, err := rd.call(t, newClient(t, xrec.srv), demographic.WithGetFormat(demographic.FormatXML))
			if err != nil {
				t.Fatalf("%s(XML) error = %v", rd.name, err)
			}
			req := xrec.only(t)
			if accept := req.header.Get("Accept"); accept != mediaXML {
				t.Errorf("%s(XML): Accept = %q, want %q", rd.name, accept, mediaXML)
			}
			if req.path != rd.path {
				t.Errorf("%s(XML): path = %q, want %q", rd.name, req.path, rd.path)
			}
			if meta == nil || meta.ETag != personVersion {
				t.Errorf("%s(XML): metadata = %+v, want the response headers", rd.name, meta)
			}
			sameVersionedResult(t, got, want)
		})
	}
}

// xmlVersionOf wraps the canonical XML of a corpus party as the data of an
// ORIGINAL_VERSION, with the data's xsi:type set to class unless class is
// empty.
func xmlVersionOf(t *testing.T, partyXML []byte, class string) []byte {
	t.Helper()
	start := bytes.IndexByte(partyXML, ' ')
	open := bytes.IndexByte(partyXML, '>')
	closeTag := bytes.LastIndex(partyXML, []byte("</"))
	if start < 0 || open < 0 || closeTag < 0 {
		t.Fatalf("xmlVersionOf: not a canonical XML party: %q", partyXML)
	}
	// Keep the attributes after the namespace declarations, which the
	// envelope declares once.
	attrs := string(partyXML[start:open])
	attrs = strings.Replace(attrs, ` xmlns="http://schemas.openehr.org/v1"`, "", 1)
	attrs = strings.Replace(attrs, ` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`, "", 1)
	discriminator := ""
	if class != "" {
		discriminator = ` xsi:type="` + class + `"`
	}
	return fmt.Appendf(nil,
		`<original_version xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
			`<uid><value>%s</value></uid><data%s%s>%s</data></original_version>`,
		personVersion, discriminator, attrs, partyXML[open+1:closeTag])
}

// TestGetVersionXMLEachPartyType pins REQ-173 at the version reads: the
// ORIGINAL_VERSION<PARTY> data decodes by its xsi:type into the concrete
// PARTY type, for every type.
func TestGetVersionXMLEachPartyType(t *testing.T) {
	for _, pf := range partyFixtures {
		t.Run(string(pf.typ), func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaXML, xmlVersionOf(t, fixture(t, pf.xml), pf.bmmName))
			pv, _, err := demographic.GetVersion(t.Context(), newClient(t, rec.srv), personVOID,
				demographic.WithGetFormat(demographic.FormatXML))
			if err != nil {
				t.Fatalf("GetVersion(XML) error = %v", err)
			}
			if gotType := fmt.Sprintf("%T", pv.Party); gotType != pf.wantType {
				t.Errorf("VERSION data decoded %s, want %s", gotType, pf.wantType)
			}
			sameParty(t, "VERSION data", pv.Party, jsonParty(t, pf.json))
			if pv.UID.Value != personVersion {
				t.Errorf("version uid = %q, want %q", pv.UID.Value, personVersion)
			}
		})
	}
}

// TestGetVersionWithoutData pins REQ-174 Responses at the versioned_party
// version read, in both formats: a version with no data, the RM's logical
// deletion the pin's UMOriginalVersionOfParty does not admit, returns the
// envelope, no party and no error.
func TestGetVersionWithoutData(t *testing.T) {
	cases := []struct {
		name   string
		format demographic.Format
		body   string
	}{
		{name: "JSON, data absent", format: demographic.FormatJSON, body: `{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"` + personVersion + `"}}`},
		{name: "JSON, data null", format: demographic.FormatJSON, body: `{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"` + personVersion + `"},"data":null}`},
		{name: "XML, data absent", format: demographic.FormatXML, body: `<original_version xmlns="http://schemas.openehr.org/v1"><uid><value>` + personVersion + `</value></uid></original_version>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, tc.format.MediaType(), []byte(tc.body))
			pv, _, err := demographic.GetVersion(t.Context(), newClient(t, rec.srv), personVOID,
				demographic.WithGetFormat(tc.format))
			if err != nil {
				t.Fatalf("GetVersion(%s) error = %v", tc.name, err)
			}
			if pv == nil || pv.Party != nil || pv.UID.Value != personVersion {
				t.Errorf("GetVersion(%s) = %+v, want the envelope and no party", tc.name, pv)
			}
		})
	}
}

// TestGetVersionImportedVersionRefused pins REQ-174 Responses at the
// versioned_party version read, in both formats: an IMPORTED_VERSION, which
// the pin's UVersionOfParty admits, is the typed decode failure wrapping
// typereg.ErrTypeMismatch, with the metadata beside it and no version. In
// XML the root names it by its xsi:type, or, as canxml writes one, by its
// `imported_version` element name alone.
func TestGetVersionImportedVersionRefused(t *testing.T) {
	jsonBody := bytes.Replace(fixture(t, "original_version.json"), []byte(`"ORIGINAL_VERSION"`), []byte(`"IMPORTED_VERSION"`), 1)
	if !bytes.Contains(jsonBody, []byte(`"IMPORTED_VERSION"`)) {
		t.Fatal("original_version.json carries no ORIGINAL_VERSION _type to replace")
	}
	cases := []struct {
		name   string
		format demographic.Format
		body   []byte
	}{
		{name: "JSON", format: demographic.FormatJSON, body: jsonBody},
		{name: "XML root xsi:type", format: demographic.FormatXML, body: withRootType(t, fixture(t, "original_version.xml"), "IMPORTED_VERSION")},
		{name: "XML imported_version element", format: demographic.FormatXML, body: canxmlImportedVersion(t)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, tc.format.MediaType(), tc.body)
			pv, meta, err := demographic.GetVersion(t.Context(), newClient(t, rec.srv), personVOID,
				demographic.WithGetFormat(tc.format))
			if de, ok := errors.AsType[*transport.DecodeError](err); !ok || de == nil {
				t.Fatalf("GetVersion(%v, IMPORTED_VERSION) error = %v, want a *transport.DecodeError", tc.format, err)
			}
			if !errors.Is(err, typereg.ErrTypeMismatch) {
				t.Errorf("GetVersion(%v, IMPORTED_VERSION) error = %v, want it to wrap typereg.ErrTypeMismatch", tc.format, err)
			}
			if pv != nil || meta == nil {
				t.Errorf("GetVersion(%v, IMPORTED_VERSION) returned (%+v, %+v), want no version and the metadata", tc.format, pv, meta)
			}
		})
	}
}

// canxmlImportedVersion returns the canonical XML canxml writes for an
// IMPORTED_VERSION wrapping the corpus ORIGINAL_VERSION's envelope: an
// `imported_version` root with no xsi:type.
func canxmlImportedVersion(t *testing.T) []byte {
	t.Helper()
	var ov rm.OriginalVersion[rm.Party]
	if err := canxml.Unmarshal(fixture(t, "original_version.xml"), &ov); err != nil {
		t.Fatalf("decode original_version.xml: %v", err)
	}
	iv := rm.ImportedVersion[rm.Party]{Item: rm.OriginalVersion[any]{UID: ov.UID, LifecycleState: ov.LifecycleState}}
	iv.CommitAudit = ov.CommitAudit
	iv.Contribution = ov.Contribution
	body, err := canxml.Marshal(&iv)
	if err != nil {
		t.Fatalf("encode IMPORTED_VERSION: %v", err)
	}
	if !bytes.HasPrefix(body, []byte("<imported_version ")) || bytes.Contains(body[:bytes.IndexByte(body, '>')], []byte("xsi:type")) {
		t.Fatalf("canxml wrote %.80s, want an imported_version root with no xsi:type", body)
	}
	return body
}

// TestVersionedReadsXMLDecodeFailures pins REQ-173 with REQ-151 at the
// versioned_party reads: an XML body that does not decode, whose root names
// another class than the read decodes, by its xsi:type or, with none, by its
// element name, or whose version data carries no xsi:type, is the typed
// decode failure with the metadata beside it and no result; WithRawBody
// still holds the bytes.
func TestVersionedReadsXMLDecodeFailures(t *testing.T) {
	for _, rd := range versionedReads() {
		other := "revision_history.xml"
		if rd.rootBMM == "REVISION_HISTORY" {
			other = "versioned_party.xml"
		}
		bodies := []struct {
			name  string
			body  []byte
			cause error
		}{
			{name: "truncated", body: []byte("<" + strings.ToLower(rd.rootBMM))},
			{name: "root names another class", body: withRootType(t, fixture(t, rd.xml), "IMPORTED_VERSION"), cause: typereg.ErrTypeMismatch},
			{name: "root element names another class", body: fixture(t, other), cause: typereg.ErrTypeMismatch},
		}
		if rd.rootBMM == "ORIGINAL_VERSION" {
			bodies = append(bodies, struct {
				name  string
				body  []byte
				cause error
			}{name: "data without xsi:type", body: xmlVersionOf(t, fixture(t, "person.xml"), ""), cause: typereg.ErrMissingType})
		}
		for _, b := range bodies {
			t.Run(rd.name+"/"+b.name, func(t *testing.T) {
				rec := newRecorder(t, http.StatusOK, mediaXML, b.body)
				var raw demographic.Raw
				got, meta, err := rd.call(t, newClient(t, rec.srv),
					demographic.WithGetFormat(demographic.FormatXML), demographic.WithRawBody(&raw))
				de, ok := errors.AsType[*transport.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("%s error = %v, want a *transport.DecodeError", rd.name, err)
				}
				if !bytes.Equal(de.Body, b.body) {
					t.Errorf("DecodeError.Body = %q, want the response body", de.Body)
				}
				if b.cause != nil && !errors.Is(err, b.cause) {
					t.Errorf("%s error = %v, want it to wrap %v", rd.name, err, b.cause)
				}
				if got != nil || meta == nil {
					t.Errorf("%s returned (%v, %+v), want no result and the metadata", rd.name, got, meta)
				}
				if !bytes.Equal(raw.Body, b.body) || raw.ContentType != mediaXML {
					t.Errorf("Raw = %+v, want the response's Content-Type and body", raw)
				}
			})
		}
	}
}

// TestVersionedReadsXMLEmptyBodyIsInvalidShape pins REQ-174 in XML: an
// empty or null 2xx body, a 204 included, fails with ErrInvalidShape beside
// the metadata.
func TestVersionedReadsXMLEmptyBodyIsInvalidShape(t *testing.T) {
	responses := []struct {
		name   string
		status int
		body   string
	}{
		{"empty 200", http.StatusOK, ""},
		{"null 200", http.StatusOK, "null"},
		{"204", http.StatusNoContent, ""},
	}
	for _, rd := range versionedReads() {
		for _, resp := range responses {
			t.Run(rd.name+"/"+resp.name, func(t *testing.T) {
				rec := newRecorder(t, resp.status, "", []byte(resp.body))
				got, meta, err := rd.call(t, newClient(t, rec.srv), demographic.WithGetFormat(demographic.FormatXML))
				if !errors.Is(err, transport.ErrInvalidShape) {
					t.Errorf("%s(%s) error = %v, want ErrInvalidShape", rd.name, resp.name, err)
				}
				if got != nil || meta == nil || meta.ETag != personVersion {
					t.Errorf("%s(%s) returned (%v, %+v), want no result and the metadata", rd.name, resp.name, got, meta)
				}
			})
		}
	}
}

// TestVersionedReadsRawBody pins WithRawBody at the versioned_party reads:
// on a 200 it holds the response's Content-Type and body in both formats.
func TestVersionedReadsRawBody(t *testing.T) {
	for _, rd := range versionedReads() {
		for _, f := range []demographic.Format{demographic.FormatJSON, demographic.FormatXML} {
			t.Run(rd.name+"/"+f.String(), func(t *testing.T) {
				body := fixture(t, rd.json)
				if f == demographic.FormatXML {
					body = fixture(t, rd.xml)
				}
				rec := newRecorder(t, http.StatusOK, f.MediaType(), body)
				var raw demographic.Raw
				if _, _, err := rd.call(t, newClient(t, rec.srv), demographic.WithGetFormat(f), demographic.WithRawBody(&raw)); err != nil {
					t.Fatalf("%s(%v) error = %v", rd.name, f, err)
				}
				if raw.ContentType != f.MediaType() || !bytes.Equal(raw.Body, body) {
					t.Errorf("Raw = {%q, %d bytes}, want {%q, %d bytes}", raw.ContentType, len(raw.Body), f.MediaType(), len(body))
				}
			})
		}
	}
}

// TestVersionedReadsOptionRefusals pins the refusals at the versioned_party
// reads: a format outside the two and a nil WithRawBody destination fail
// with ErrInvalidConfig and issue no request.
func TestVersionedReadsOptionRefusals(t *testing.T) {
	opts := []struct {
		name string
		opt  demographic.GetOption
	}{
		{name: "Format(2)", opt: demographic.WithGetFormat(demographic.FormatXML + 1)},
		{name: "WithRawBody(nil)", opt: demographic.WithRawBody(nil)},
	}
	for _, rd := range versionedReads() {
		for _, o := range opts {
			t.Run(rd.name+"/"+o.name, func(t *testing.T) {
				rec := newRecorder(t, http.StatusOK, mediaJSON, fixture(t, rd.json))
				got, meta, err := rd.call(t, newClient(t, rec.srv), o.opt)
				if !errors.Is(err, transport.ErrInvalidConfig) {
					t.Errorf("%s(%s) error = %v, want ErrInvalidConfig", rd.name, o.name, err)
				}
				if got != nil || meta != nil {
					t.Errorf("%s(%s) returned (%v, %+v), want nil, nil", rd.name, o.name, got, meta)
				}
				if n := rec.count(); n != 0 {
					t.Errorf("server saw %d requests, want 0", n)
				}
			})
		}
	}
}

// TestVersionedRepositoryTakesOptions pins that each Repository versioned
// read takes the read options and passes them on.
func TestVersionedRepositoryTakesOptions(t *testing.T) {
	srvBodies := map[string][]byte{
		"versioned_party":  fixture(t, "versioned_party.xml"),
		"revision_history": fixture(t, "revision_history.xml"),
		"version":          fixture(t, "original_version.xml"),
	}
	xml := demographic.WithGetFormat(demographic.FormatXML)
	at := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	calls := []struct {
		name string
		body []byte
		call func(r demographic.Repository) error
	}{
		{"GetVersionedParty", srvBodies["versioned_party"], func(r demographic.Repository) error {
			_, _, err := r.GetVersionedParty(t.Context(), personVOID, xml)
			return err
		}},
		{"GetRevisionHistory", srvBodies["revision_history"], func(r demographic.Repository) error {
			_, _, err := r.GetRevisionHistory(t.Context(), personVOID, xml)
			return err
		}},
		{"GetVersion", srvBodies["version"], func(r demographic.Repository) error {
			_, _, err := r.GetVersion(t.Context(), personVOID, xml)
			return err
		}},
		{"GetVersionAtTime", srvBodies["version"], func(r demographic.Repository) error {
			_, _, err := r.GetVersionAtTime(t.Context(), personVOID, at, xml)
			return err
		}},
		{"GetVersionByID", srvBodies["version"], func(r demographic.Repository) error {
			_, _, err := r.GetVersionByID(t.Context(), personVOID, personVersion, xml)
			return err
		}},
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaXML, tc.body)
			if err := tc.call(demographic.NewRepository(newClient(t, rec.srv))); err != nil {
				t.Fatalf("Repository.%s(XML) error = %v", tc.name, err)
			}
			if accept := rec.only(t).header.Get("Accept"); accept != mediaXML {
				t.Errorf("Repository.%s: Accept = %q, want %q", tc.name, accept, mediaXML)
			}
		})
	}
}
