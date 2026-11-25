package demographicprobes_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/demographic"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/sandbox"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	demographicprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/demographic"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, b *sandbox.Backend) *transport.Client {
	t.Helper()
	c, err := probe.NewClient("https://sandbox.local/openehr/v1", b.HTTPClient(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "corpus", "its_rest", "demographic", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	return b
}

// partyBody is one PARTY in both canonical formats: its canonical JSON, its
// canonical XML, and its class, which the XML version envelope names on
// its `data` element.
type partyBody struct {
	json  []byte
	xml   []byte
	class string
}

// partyFixture reads the corpus PARTY name (e.g. "person") in both formats.
func partyFixture(t *testing.T, name, class string) partyBody {
	t.Helper()
	return partyBody{json: fixture(t, name+".json"), xml: fixture(t, name+".xml"), class: class}
}

// xmlVersionEnvelope wraps partyXML, a canonical XML PARTY as canxml writes
// it, as the `data` of an ORIGINAL_VERSION, with an xsi:type naming class.
func xmlVersionEnvelope(partyXML []byte, class string) []byte {
	s := string(partyXML)
	open := strings.IndexByte(s, '>')
	closeTag := strings.LastIndex(s, "</")
	attrs := s[strings.IndexByte(s, ' '):open]
	attrs = strings.Replace(attrs, ` xmlns="http://schemas.openehr.org/v1"`, "", 1)
	attrs = strings.Replace(attrs, ` xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`, "", 1)
	return fmt.Appendf(nil,
		`<original_version xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
			`<uid><value>%s::cdr::1</value></uid><data xsi:type="%s"%s>%s</data></original_version>`,
		demographicprobes073VOID, class, attrs, s[open+1:closeTag])
}

// partyEchoBackend serves the typed PARTY body for typed reads and writes,
// and wraps the version body in an ORIGINAL_VERSION envelope on the version
// sub-path, each in format f, so one fixture drives the whole create → get →
// get-version round-trip the probe asserts. A request that does not ask for
// f, or a write that does not send f, is answered 406, so a probe that
// ignores the format fails.
func partyEchoBackend(f demographic.Format, typed, version partyBody) *sandbox.Backend {
	return sandbox.Scripted(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != f.MediaType() || r.Method == http.MethodPost && r.Header.Get("Content-Type") != f.MediaType() {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Header().Set("ETag", `"`+string(demographicprobes073VOID)+`::cdr::1"`)
		w.Header().Set("Location", r.URL.Path)
		xmlAsked := f == demographic.FormatXML
		if strings.Contains(r.URL.Path, "/version") {
			if xmlAsked {
				_, _ = w.Write(xmlVersionEnvelope(version.xml, version.class))
				return
			}
			env := fmt.Sprintf(
				`{"_type":"ORIGINAL_VERSION","uid":{"_type":"OBJECT_VERSION_ID","value":"%s::cdr::1"},"data":%s}`,
				demographicprobes073VOID, version.json,
			)
			_, _ = w.Write([]byte(env))
			return
		}
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		if xmlAsked {
			_, _ = w.Write(typed.xml)
			return
		}
		_, _ = w.Write(typed.json)
	})
}

// formats are the two representations PROBE-073 runs in.
var formats = []demographic.Format{demographic.FormatJSON, demographic.FormatXML}

// demographicprobes073VOID mirrors the probe's fixed versioned-object id for
// the version-envelope uid the fake server emits.
const demographicprobes073VOID = "demographic-probe-vo-1"

// REQ-050: PROBE-073 round-trips every PARTY type through the
// 1.1.0 demographic endpoints, in canonical JSON and, under REQ-173, in
// canonical XML.
func TestProbe073DemographicRoundTrip(t *testing.T) {
	cases := []struct {
		party rm.Party
		typ   demographic.Type
		class string
	}{
		{&rm.Person{Name: rm.DVText{Value: "Jane Doe"}}, demographic.Person, "PERSON"},
		{&rm.Organisation{Name: rm.DVText{Value: "Acme Hospital"}}, demographic.Organisation, "ORGANISATION"},
		{&rm.Group{Name: rm.DVText{Value: "Cardiology Team"}}, demographic.Group, "GROUP"},
		{&rm.Agent{Name: rm.DVText{Value: "Triage Bot"}}, demographic.Agent, "AGENT"},
		{&rm.Role{Name: rm.DVText{Value: "Attending Physician"}}, demographic.Role, "ROLE"},
	}
	for _, f := range formats {
		for _, tc := range cases {
			t.Run(f.String()+"/"+string(tc.typ), func(t *testing.T) {
				body := partyFixture(t, string(tc.typ), tc.class)
				b := partyEchoBackend(f, body, body)

				r, err := demographicprobes.Probe073DemographicRoundTrip(
					context.Background(), newClient(t, b), tc.party, tc.typ, f,
				)
				if err != nil {
					t.Fatalf("Probe073: %v", err)
				}
				if r.Status != "pass" {
					t.Fatalf("Probe073 status=%q detail=%q", r.Status, r.Detail)
				}
				if r.Probe != "PROBE-073" {
					t.Errorf("probe id = %q, want PROBE-073", r.Probe)
				}
			})
		}
	}
}

// TestProbe073DetectsTypeDrift confirms the probe fails at the create hop
// when the wire body's discriminator does not round-trip to the input
// party's concrete type: its `_type` in JSON, the root element's `xsi:type`
// in XML.
func TestProbe073DetectsTypeDrift(t *testing.T) {
	for _, f := range formats {
		t.Run(f.String(), func(t *testing.T) {
			// Server returns a PERSON body, but the probe is told to expect an ORGANISATION.
			person := partyFixture(t, "person", "PERSON")
			person.xml = []byte(strings.Replace(string(person.xml), "<person ", `<person xsi:type="PERSON" `, 1))
			b := partyEchoBackend(f, person, person)

			r, err := demographicprobes.Probe073DemographicRoundTrip(
				context.Background(), newClient(t, b),
				&rm.Organisation{Name: rm.DVText{Value: "x"}}, demographic.Organisation, f,
			)
			if err != nil {
				t.Fatalf("Probe073: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("expected fail on type drift, got status=%q", r.Status)
			}
			if !strings.Contains(r.Detail, "create") {
				t.Errorf("detail %q should reference the create hop", r.Detail)
			}
		})
	}
}

// TestProbe073DetectsVersionDataDrift drives a server whose typed body matches
// the input (PERSON) but whose ORIGINAL_VERSION envelope `data` drifts to a
// different concrete type, in either format. Create/Get pass; only the
// VERSION-envelope hop, whose `data` is decoded by its discriminator, must
// catch the drift.
func TestProbe073DetectsVersionDataDrift(t *testing.T) {
	for _, f := range formats {
		t.Run(f.String(), func(t *testing.T) {
			b := partyEchoBackend(f, partyFixture(t, "person", "PERSON"), partyFixture(t, "organisation", "ORGANISATION"))

			r, err := demographicprobes.Probe073DemographicRoundTrip(
				context.Background(), newClient(t, b),
				&rm.Person{Name: rm.DVText{Value: "Jane Doe"}}, demographic.Person, f,
			)
			if err != nil {
				t.Fatalf("Probe073: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("expected fail on VERSION-data drift, got status=%q", r.Status)
			}
			if !strings.Contains(r.Detail, "VERSION") {
				t.Errorf("detail %q should reference the VERSION hop", r.Detail)
			}
		})
	}
}
