package demographic_test

// format_get_test.go — REQ-173 Reads at the demographic leaf: Get in
// canonical JSON or canonical XML, the raw body, and the outcomes REQ-151
// keeps in both formats.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// testPersonXML reads the vendored PERSON in canonical XML whose root
// element carries xsi:type="PERSON".
func testPersonXML(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "corpus", "compositions", "TestPerson.v2.xml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// TestGetWithoutOptionsSendsJSON pins REQ-173 Reads at the demographic leaf:
// with no option Get asks for canonical JSON and decodes it.
func TestGetWithoutOptionsSendsJSON(t *testing.T) {
	rec := newRecorder(t, http.StatusOK, mediaJSON, fixture(t, "person.json"))
	got, _, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person, openehrclient.LatestOf(personVOID))
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if accept := rec.only(t).header.Get("Accept"); accept != mediaJSON {
		t.Errorf("Accept = %q, want %q", accept, mediaJSON)
	}
	sameParty(t, "Get", got, jsonParty(t, "person.json"))
}

// TestGetXMLEachPartyType pins REQ-173 WithGetFormat at the demographic
// leaf: Get sends application/xml as Accept and decodes the canonical XML
// body as the class the call names, for every PARTY type.
func TestGetXMLEachPartyType(t *testing.T) {
	for _, pf := range partyFixtures {
		t.Run(string(pf.typ), func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaXML, fixture(t, pf.xml))
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), pf.typ,
				openehrclient.LatestOf(personVOID), demographic.WithGetFormat(demographic.FormatXML))
			if err != nil {
				t.Fatalf("Get(%s, XML) error = %v", pf.typ, err)
			}
			req := rec.only(t)
			if accept := req.header.Get("Accept"); accept != mediaXML {
				t.Errorf("Accept = %q, want %q", accept, mediaXML)
			}
			if req.path != "/openehr/v1/demographic/"+string(pf.typ)+"/"+personVOID {
				t.Errorf("path = %q", req.path)
			}
			if gotType := fmt.Sprintf("%T", got); gotType != pf.wantType {
				t.Errorf("decoded %s, want %s", gotType, pf.wantType)
			}
			sameParty(t, "Get", got, jsonParty(t, pf.json))
			if meta == nil || meta.VersionUID != personVersion {
				t.Errorf("VersionUID = %+v, want %q", meta, personVersion)
			}
		})
	}
}

// TestGetXMLRootTypeOfTheClass pins that a root xsi:type naming the class
// the call names is read: the vendored PERSON whose root carries
// xsi:type="PERSON" decodes as a PERSON.
func TestGetXMLRootTypeOfTheClass(t *testing.T) {
	rec := newRecorder(t, http.StatusOK, mediaXML, testPersonXML(t))
	got, _, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
		openehrclient.LatestOf(personVOID), demographic.WithGetFormat(demographic.FormatXML))
	if err != nil {
		t.Fatalf("Get(PERSON root) error = %v", err)
	}
	person, ok := got.(*rm.Person)
	if !ok {
		t.Fatalf("decoded %T, want *rm.Person", got)
	}
	if person.ArchetypeNodeID != "openEHR-DEMOGRAPHIC-PERSON.person.v2" || len(person.Identities) == 0 {
		t.Errorf("decoded PERSON lacks its content: node id %q, %d identities", person.ArchetypeNodeID, len(person.Identities))
	}
}

// TestGetXMLRootNamesOtherClass pins REQ-173 with REQ-151 at the
// demographic leaf: a canonical XML body whose root names another class than
// the one the call names, by its xsi:type or, with none, by its element name,
// is the typed decode failure, with the version metadata beside it and no
// party. The root is read before the body, so a body of another shape gets
// this error and not a child's. The xsi:type reads as REQ-056 reads a
// discriminator: one that names no openEHR class, a namespace-prefixed one
// included, wraps typereg.ErrUnknownType, and another class
// typereg.ErrTypeMismatch.
func TestGetXMLRootNamesOtherClass(t *testing.T) {
	cases := []struct {
		name  string
		typ   demographic.Type
		body  []byte
		cause error
	}{
		{name: "ORGANISATION root on a person read", typ: demographic.Person, body: withRootType(t, fixture(t, "person.xml"), "ORGANISATION"), cause: typereg.ErrTypeMismatch},
		{name: "PERSON root on an organisation read", typ: demographic.Organisation, body: testPersonXML(t), cause: typereg.ErrTypeMismatch},
		{name: "prefixed PERSON root on a person read", typ: demographic.Person, body: withRootType(t, fixture(t, "person.xml"), "ns2:PERSON"), cause: typereg.ErrUnknownType},
		{name: "unknown class root on a person read", typ: demographic.Person, body: withRootType(t, fixture(t, "person.xml"), "NOT_A_CLASS"), cause: typereg.ErrUnknownType},
		{name: "prefixed VERSIONED_PARTY root on a person read", typ: demographic.Person, body: withRootType(t, fixture(t, "versioned_party.xml"), "ns2:VERSIONED_PARTY"), cause: typereg.ErrUnknownType},
		{name: "VERSIONED_PARTY root on a person read", typ: demographic.Person, body: withRootType(t, fixture(t, "versioned_party.xml"), "VERSIONED_PARTY"), cause: typereg.ErrTypeMismatch},
		{name: "organisation element with no xsi:type on a person read", typ: demographic.Person, body: fixture(t, "organisation.xml"), cause: typereg.ErrTypeMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaXML, tc.body)
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), tc.typ,
				openehrclient.LatestOf(personVOID), demographic.WithGetFormat(demographic.FormatXML))
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("Get error = %v, want a *transport.DecodeError", err)
			}
			if !errors.Is(err, tc.cause) {
				t.Errorf("Get error = %v, want it to wrap %v", err, tc.cause)
			}
			if got != nil {
				t.Errorf("Get returned %T beside the error, want nil", got)
			}
			if meta == nil || meta.VersionUID != personVersion {
				t.Errorf("metadata = %+v, want VersionUID %q", meta, personVersion)
			}
		})
	}
}

// TestGetXMLRootNamedForNoClassReadsAsTheCallsClass pins the other side of
// REQ-173's root rule: a root with no xsi:type whose element name names no
// openEHR class, such as the ITS-XML `party` element, is read as the class
// the call names.
func TestGetXMLRootNamedForNoClassReadsAsTheCallsClass(t *testing.T) {
	body := bytes.Replace(fixture(t, "person.xml"), []byte("<person "), []byte("<party "), 1)
	body = bytes.Replace(body, []byte("</person>"), []byte("</party>"), 1)
	if !bytes.HasPrefix(body, []byte("<party ")) || !bytes.HasSuffix(body, []byte("</party>")) {
		t.Fatalf("person.xml did not rename to a party root: %.40s … %s", body, body[len(body)-20:])
	}
	rec := newRecorder(t, http.StatusOK, mediaXML, body)
	got, _, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
		openehrclient.LatestOf(personVOID), demographic.WithGetFormat(demographic.FormatXML))
	if err != nil {
		t.Fatalf("Get(person, party root) error = %v", err)
	}
	if _, ok := got.(*rm.Person); !ok {
		t.Errorf("Get(person, party root) = %T, want *rm.Person", got)
	}
}

// TestGetJSONTypeNamesOtherClass pins REQ-173 with REQ-151 at the
// demographic leaf: a canonical JSON PARTY whose _type names another class
// than the one the call names is the typed decode failure wrapping
// typereg.ErrTypeMismatch, with the version metadata beside it and no party,
// as an XML root of another class is.
func TestGetJSONTypeNamesOtherClass(t *testing.T) {
	cases := []struct {
		name string
		typ  demographic.Type
		file string
	}{
		{name: "ORGANISATION body on a person read", typ: demographic.Person, file: "organisation.json"},
		{name: "PERSON body on a role read", typ: demographic.Role, file: "person.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaJSON, fixture(t, tc.file))
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), tc.typ, openehrclient.LatestOf(personVOID))
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("Get error = %v, want a *transport.DecodeError", err)
			}
			if !errors.Is(err, typereg.ErrTypeMismatch) {
				t.Errorf("Get error = %v, want it to wrap %v", err, typereg.ErrTypeMismatch)
			}
			if got != nil {
				t.Errorf("Get returned %T beside the error, want nil", got)
			}
			if meta == nil || meta.VersionUID != personVersion {
				t.Errorf("metadata = %+v, want VersionUID %q", meta, personVersion)
			}
		})
	}
}

// TestGetDecoderFollowsAskedFormat pins REQ-173: the decoder is the one the
// caller asked for, never the one the response's Content-Type names.
func TestGetDecoderFollowsAskedFormat(t *testing.T) {
	cases := []struct {
		name        string
		format      demographic.Format
		contentType string
		body        []byte
		wantDecoded bool
	}{
		{name: "XML asked, XML body labelled JSON", format: demographic.FormatXML, contentType: mediaJSON, body: fixture(t, "person.xml"), wantDecoded: true},
		{name: "JSON asked, JSON body labelled XML", format: demographic.FormatJSON, contentType: mediaXML, body: fixture(t, "person.json"), wantDecoded: true},
		{name: "XML asked, JSON body labelled JSON", format: demographic.FormatXML, contentType: mediaJSON, body: fixture(t, "person.json")},
		{name: "JSON asked, XML body labelled XML", format: demographic.FormatJSON, contentType: mediaXML, body: fixture(t, "person.xml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, tc.contentType, tc.body)
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
				openehrclient.LatestOf(personVOID), demographic.WithGetFormat(tc.format))
			if tc.wantDecoded {
				if err != nil {
					t.Fatalf("Get error = %v", err)
				}
				sameParty(t, "Get", got, jsonParty(t, "person.json"))
				return
			}
			if _, ok := errors.AsType[*transport.DecodeError](err); !ok {
				t.Errorf("Get error = %v, want a *transport.DecodeError", err)
			}
			if got != nil || meta == nil {
				t.Errorf("Get returned (%T, %+v), want no party and the metadata", got, meta)
			}
		})
	}
}

// TestGetRawBody pins REQ-173 WithRawBody at the demographic leaf: on a 200
// it holds the response's Content-Type and body in both formats, and a body
// that does not decode is still in hand beside the typed decode failure.
func TestGetRawBody(t *testing.T) {
	cases := []struct {
		name        string
		format      demographic.Format
		contentType string
		body        []byte
		wantErr     bool
	}{
		{name: "JSON", format: demographic.FormatJSON, contentType: mediaJSON + "; charset=utf-8", body: fixture(t, "person.json")},
		{name: "XML", format: demographic.FormatXML, contentType: mediaXML, body: fixture(t, "person.xml")},
		{name: "XML that does not decode", format: demographic.FormatXML, contentType: mediaXML, body: []byte("<person"), wantErr: true},
		{name: "JSON that does not decode", format: demographic.FormatJSON, contentType: mediaJSON, body: []byte(unregisteredParty), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, tc.contentType, tc.body)
			var raw demographic.Raw
			_, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
				openehrclient.LatestOf(personVOID), demographic.WithGetFormat(tc.format), demographic.WithRawBody(&raw))
			if tc.wantErr {
				de, ok := errors.AsType[*transport.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("Get error = %v, want a *transport.DecodeError", err)
				}
				if meta == nil {
					t.Error("Get returned no metadata beside the decode failure")
				}
			} else if err != nil {
				t.Fatalf("Get error = %v", err)
			}
			if raw.ContentType != tc.contentType {
				t.Errorf("Raw.ContentType = %q, want %q", raw.ContentType, tc.contentType)
			}
			if string(raw.Body) != string(tc.body) {
				t.Errorf("Raw.Body = %q, want %q", raw.Body, tc.body)
			}
		})
	}
}

// TestGetEmptyBodyAnd204InBothFormats pins REQ-173 with REQ-151 at the
// demographic leaf: an empty or null 200 body is ErrInvalidShape, and a 204
// (the party was deleted at the requested time) is no party and no error,
// in both formats; WithRawBody holds the 200's empty body and leaves the
// 204 alone.
func TestGetEmptyBodyAnd204InBothFormats(t *testing.T) {
	for _, f := range []demographic.Format{demographic.FormatJSON, demographic.FormatXML} {
		for _, body := range []string{"", "  ", "null"} {
			t.Run(f.String()+"/200 "+fmt.Sprintf("%q", body), func(t *testing.T) {
				rec := newRecorder(t, http.StatusOK, f.MediaType(), []byte(body))
				raw := demographic.Raw{ContentType: "unset"}
				got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
					openehrclient.LatestOf(personVOID), demographic.WithGetFormat(f), demographic.WithRawBody(&raw))
				if !errors.Is(err, transport.ErrInvalidShape) {
					t.Errorf("Get error = %v, want ErrInvalidShape", err)
				}
				if _, ok := errors.AsType[*transport.DecodeError](err); ok {
					t.Errorf("Get error = %v is a DecodeError, want ErrInvalidShape alone", err)
				}
				if got != nil || meta == nil {
					t.Errorf("Get returned (%T, %+v), want no party and the metadata", got, meta)
				}
				if raw.ContentType != f.MediaType() || string(raw.Body) != body {
					t.Errorf("Raw = %+v, want the 200's Content-Type and body", raw)
				}
			})
		}
		t.Run(f.String()+"/204", func(t *testing.T) {
			rec := newRecorder(t, http.StatusNoContent, "", nil)
			raw := demographic.Raw{ContentType: "unset"}
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
				openehrclient.LatestOf(personVOID), demographic.WithGetFormat(f), demographic.WithRawBody(&raw))
			if err != nil {
				t.Errorf("Get(204) error = %v, want nil", err)
			}
			if got != nil || meta == nil || meta.VersionUID != personVersion {
				t.Errorf("Get(204) returned (%T, %+v), want no party and the metadata", got, meta)
			}
			if raw.ContentType != "unset" || raw.Body != nil {
				t.Errorf("Raw = %+v after a 204, want it left as it was", raw)
			}
		})
	}
}

// TestGetRawBodyLeftAsItWasOnError pins REQ-173 WithRawBody: an error
// status leaves dst as it was.
func TestGetRawBodyLeftAsItWasOnError(t *testing.T) {
	rec := newRecorder(t, http.StatusNotFound, mediaJSON, []byte(`{"message":"not found"}`))
	raw := demographic.Raw{ContentType: "unset"}
	_, _, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
		openehrclient.LatestOf(personVOID), demographic.WithRawBody(&raw))
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("Get(404) error = %v, want ErrNotFound", err)
	}
	if raw.ContentType != "unset" || raw.Body != nil {
		t.Errorf("Raw = %+v after a 404, want it left as it was", raw)
	}
}

// TestGetOptionRefusals pins the REQ-173 refusals on the demographic Get:
// a format outside the two and a nil WithRawBody destination fail with
// ErrInvalidConfig and issue no request.
func TestGetOptionRefusals(t *testing.T) {
	cases := []struct {
		name string
		opt  demographic.GetOption
	}{
		{name: "Format(2)", opt: demographic.WithGetFormat(demographic.FormatXML + 1)},
		{name: "Format(-1)", opt: demographic.WithGetFormat(demographic.Format(-1))},
		{name: "WithRawBody(nil)", opt: demographic.WithRawBody(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecorder(t, http.StatusOK, mediaJSON, fixture(t, "person.json"))
			got, meta, err := demographic.Get(t.Context(), newClient(t, rec.srv), demographic.Person,
				openehrclient.LatestOf(personVOID), tc.opt)
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("Get(%s) error = %v, want ErrInvalidConfig", tc.name, err)
			}
			if got != nil || meta != nil {
				t.Errorf("Get(%s) returned (%T, %+v), want nil, nil", tc.name, got, meta)
			}
			if n := rec.count(); n != 0 {
				t.Errorf("server saw %d requests, want 0", n)
			}
		})
	}
}

// TestRepositoryGetTakesOptions pins REQ-173: the Repository's Get takes
// the read options and passes them on.
func TestRepositoryGetTakesOptions(t *testing.T) {
	rec := newRecorder(t, http.StatusOK, mediaXML, fixture(t, "person.xml"))
	var raw demographic.Raw
	got, _, err := demographic.NewRepository(newClient(t, rec.srv)).Get(t.Context(), demographic.Person,
		openehrclient.LatestOf(personVOID), demographic.WithGetFormat(demographic.FormatXML), demographic.WithRawBody(&raw))
	if err != nil {
		t.Fatalf("Repository.Get error = %v", err)
	}
	if accept := rec.only(t).header.Get("Accept"); accept != mediaXML {
		t.Errorf("Accept = %q, want %q", accept, mediaXML)
	}
	sameParty(t, "Repository.Get", got, jsonParty(t, "person.json"))
	if raw.ContentType != mediaXML {
		t.Errorf("Raw.ContentType = %q, want %q", raw.ContentType, mediaXML)
	}
}
