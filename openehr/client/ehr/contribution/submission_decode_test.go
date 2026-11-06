package contribution_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/expectedform"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// decodeSeed is a value written into a body whose decode is refused, so a
// test can assert that the refusal's text does not carry it.
const decodeSeed = "seeded-value-5f3c9a"

const (
	codedCreation = `{"_type":"DV_CODED_TEXT","value":"creation","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"249"}}`
	codedComplete = `{"_type":"DV_CODED_TEXT","value":"complete","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"532"}}`
	committerJSON = `{"_type":"PARTY_IDENTIFIED","name":"alice"}`
	// itemAuditJSON is the persisted AUDIT_DETAILS an imported item carries,
	// time_committed included: the item keeps its own RM shape.
	itemAuditJSON = `{"_type":"AUDIT_DETAILS","system_id":"other.example","committer":{"_type":"PARTY_SELF"},"change_type":` + codedCreation + `,"time_committed":{"_type":"DV_DATE_TIME","value":"2026-05-17T10:00:00Z"}}`
	// compositionJSON carries every member the RM requires of a COMPOSITION.
	compositionJSON = `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.report.v1","name":{"_type":"DV_TEXT","value":"Report"},"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},"territory":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"},"code_string":"GB"},"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"433"}},"composer":{"_type":"PARTY_SELF"}}`
)

// object writes a JSON object of the given members, each written by member.
func object(members ...string) string {
	return "{" + strings.Join(members, ",") + "}"
}

// member writes one JSON object member.
func member(name, value string) string {
	return strconv.Quote(name) + ":" + value
}

// auditJSON is a write-side audit whose `_type` is typ, left out when typ is
// empty, followed by extra members.
func auditJSON(typ string, extra ...string) string {
	var members []string
	if typ != "" {
		members = append(members, member("_type", strconv.Quote(typ)))
	}
	members = append(members,
		member("system_id", `"cdr.example"`),
		member("committer", committerJSON),
		member("change_type", codedCreation))
	return object(append(members, extra...)...)
}

// originalJSON is an ORIGINAL_VERSION element without a data member, followed
// by extra members.
func originalJSON(extra ...string) string {
	return object(append([]string{
		member("_type", `"ORIGINAL_VERSION"`),
		member("commit_audit", auditJSON("AUDIT_DETAILS")),
		member("lifecycle_state", codedComplete),
	}, extra...)...)
}

// importedItemUID is the uid of the item importedJSON writes.
const importedItemUID = "8849182c-82ad-4088-a07f-48ead4180515::other.example::1"

// importedJSON is an IMPORTED_VERSION element whose item carries data, left
// out when data is empty.
func importedJSON(data string) string {
	return importedJSONAudit(auditJSON("AUDIT_DETAILS"), data)
}

// importedJSONAudit is importedJSON with the commit_audit given.
func importedJSONAudit(audit, data string) string {
	item := []string{
		member("_type", `"ORIGINAL_VERSION"`),
		member("contribution", `{"_type":"OBJECT_REF","id":{"_type":"HIER_OBJECT_ID","value":"b7c3a0f2-2f6c-4f0b-9d55-0c1f3d6e8a11"},"namespace":"other.example","type":"CONTRIBUTION"}`),
		member("commit_audit", itemAuditJSON),
		member("uid", `{"_type":"OBJECT_VERSION_ID","value":"`+importedItemUID+`"}`),
		member("lifecycle_state", codedComplete),
	}
	if data != "" {
		item = append(item, member("data", data))
	}
	return object(
		member("_type", `"IMPORTED_VERSION"`),
		member("commit_audit", audit),
		member("item", object(item...)),
	)
}

// bodyJSON is a Contribution_create body of audit and versions.
func bodyJSON(audit string, versions ...string) []byte {
	return []byte(object(member("audit", audit), member("versions", "["+strings.Join(versions, ",")+"]")))
}

// submissionPayloadTargets maps a payload `_type` to the decode target the
// expected form of its version is built for.
var submissionPayloadTargets = map[string]func() any{
	"COMPOSITION": func() any { return new(rm.OriginalVersion[rm.Composition]) },
	"EHR_STATUS":  func() any { return new(rm.OriginalVersion[rm.EHRStatus]) },
	"FOLDER":      func() any { return new(rm.OriginalVersion[rm.Folder]) },
	"EHR_ACCESS":  func() any { return new(rm.OriginalVersion[rm.EHRAccess]) },
}

// expectedSubmission returns the expected form of a corpus submission: the
// document its decode and re-encode must be wire-equivalent to. It is the
// input without the top-level `_type` the corpus records carry and
// Contribution_create does not, with the audit and each version written in
// the expected form testkit/expectedform gives their RM type. Only
// ORIGINAL_VERSION elements are handled; the corpus holds no other.
func expectedSubmission(t *testing.T, raw []byte) []byte {
	t.Helper()
	var in struct {
		Audit    json.RawMessage   `json:"audit"`
		Versions []json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("corpus record is not a JSON object: %v", err)
	}
	audit, err := expectedform.Build(in.Audit, "AUDIT_DETAILS")
	if err != nil {
		t.Fatalf("expected form of audit: %v", err)
	}
	versions := make([]json.RawMessage, len(in.Versions))
	for i, v := range in.Versions {
		var head struct {
			Type string `json:"_type"`
			Data struct {
				Type string `json:"_type"`
			} `json:"data"`
		}
		if err := json.Unmarshal(v, &head); err != nil {
			t.Fatalf("versions[%d]: %v", i, err)
		}
		if head.Type != "ORIGINAL_VERSION" {
			t.Fatalf("versions[%d]._type = %q; the expected form handles ORIGINAL_VERSION only", i, head.Type)
		}
		target, ok := submissionPayloadTargets[head.Data.Type]
		if !ok {
			target = submissionPayloadTargets["COMPOSITION"]
		}
		if versions[i], err = expectedform.BuildFor(v, target()); err != nil {
			t.Fatalf("expected form of versions[%d]: %v", i, err)
		}
	}
	out, err := json.Marshal(struct {
		Audit    json.RawMessage   `json:"audit"`
		Versions []json.RawMessage `json:"versions"`
	}{audit, versions})
	if err != nil {
		t.Fatalf("expected form: %v", err)
	}
	return out
}

// TestUnmarshalSubmissionCorpus decodes every vendored submission record.
// Each one that is a Contribution_create the SDK can place decodes, passes
// Validate, and encodes to the expected form of the record; decoding that
// encoding again gives the same bytes. The two the decoder cannot place are
// refused at the member that makes them so: an empty versions list, and a
// version whose data is a CONTRIBUTION, outside the four versionable types.
func TestUnmarshalSubmissionCorpus(t *testing.T) { // REQ-130
	refused := map[string]string{
		"contributions_invalid_no_versions.json":                                            "/versions",
		"contributions_valid_minimal_minimal_admin.contribution.creation_modification.json": "/versions/0/data/_type",
	}
	paths, err := fixtures.ListSubmissionJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("submission corpus is empty")
	}
	seen := 0
	for _, p := range paths {
		name := filepath.Base(p)
		if _, ok := refused[name]; ok {
			seen++
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			sub, err := contribution.UnmarshalSubmission(raw)
			if want, ok := refused[name]; ok {
				de, isTop := err.(*canjson.DecodeError) //nolint:errorlint // the refusal is the envelope itself, found without unwrapping
				if !isTop || de.Path != want {
					t.Fatalf("UnmarshalSubmission = %v (%T), want a *canjson.DecodeError at %q", err, err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalSubmission: %v", err)
			}
			if err := sub.Validate(); err != nil {
				t.Fatalf("decoded submission fails Validate: %v", err)
			}
			got, err := sub.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if ok, diff := wireequiv.Equivalent(expectedSubmission(t, raw), got); !ok {
				t.Errorf("encoding differs from the record's expected form at %s\n%s", diff, got)
			}
			again, err := contribution.UnmarshalSubmission(got)
			if err != nil {
				t.Fatalf("UnmarshalSubmission of its own encoding: %v", err)
			}
			gotAgain, err := again.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON of the second decode: %v", err)
			}
			if !bytes.Equal(got, gotAgain) {
				t.Errorf("second encoding differs from the first\nfirst:  %s\nsecond: %s", got, gotAgain)
			}
		})
	}
	if seen != len(refused) {
		t.Errorf("found %d of the %d records expected to be refused; the corpus changed", seen, len(refused))
	}
}

// isWrapper reports whether v is a W, one of the eight wrapper
// instantiations.
func isWrapper[W contribution.CommitVersion](v contribution.CommitVersion) bool {
	_, ok := v.(W)
	return ok
}

// TestUnmarshalSubmissionInstantiations decodes each versions[] element as
// the wrapper its `_type` names, its payload type chosen by the `_type` of
// its data (of item.data for an IMPORTED_VERSION), and an element with no
// data, or a null one, with no payload.
func TestUnmarshalSubmissionInstantiations(t *testing.T) { // REQ-130
	payload := func(typ string) string { return object(member("_type", strconv.Quote(typ))) }
	cases := []struct {
		name    string
		version string
		is      func(contribution.CommitVersion) bool
		// noData asserts the decoded ORIGINAL_VERSION carries no payload.
		noData bool
		// imported names the payload `_type` an IMPORTED_VERSION's item must
		// carry back, with the item's uid.
		imported string
	}{
		{name: "ORIGINAL_VERSION<COMPOSITION>", version: originalJSON(member("data", compositionJSON)), is: isWrapper[*contribution.OriginalVersion[rm.Composition]]},
		{name: "ORIGINAL_VERSION<EHR_STATUS>", version: originalJSON(member("data", payload("EHR_STATUS"))), is: isWrapper[*contribution.OriginalVersion[rm.EHRStatus]]},
		{name: "ORIGINAL_VERSION<FOLDER>", version: originalJSON(member("data", payload("FOLDER"))), is: isWrapper[*contribution.OriginalVersion[rm.Folder]]},
		{name: "ORIGINAL_VERSION<EHR_ACCESS>", version: originalJSON(member("data", payload("EHR_ACCESS"))), is: isWrapper[*contribution.OriginalVersion[rm.EHRAccess]]},
		{name: "IMPORTED_VERSION<COMPOSITION>", version: importedJSON(compositionJSON), is: isWrapper[*contribution.ImportedVersion[rm.Composition]], imported: "COMPOSITION"},
		{name: "IMPORTED_VERSION<EHR_STATUS>", version: importedJSON(payload("EHR_STATUS")), is: isWrapper[*contribution.ImportedVersion[rm.EHRStatus]], imported: "EHR_STATUS"},
		{name: "IMPORTED_VERSION<FOLDER>", version: importedJSON(payload("FOLDER")), is: isWrapper[*contribution.ImportedVersion[rm.Folder]], imported: "FOLDER"},
		{name: "IMPORTED_VERSION<EHR_ACCESS>", version: importedJSON(payload("EHR_ACCESS")), is: isWrapper[*contribution.ImportedVersion[rm.EHRAccess]], imported: "EHR_ACCESS"},
		{
			name: "no _type reads as ORIGINAL_VERSION",
			version: object(
				member("commit_audit", auditJSON("AUDIT_DETAILS")),
				member("lifecycle_state", codedComplete),
				member("data", payload("FOLDER"))),
			is: isWrapper[*contribution.OriginalVersion[rm.Folder]],
		},
		{
			name: "empty _type reads as ORIGINAL_VERSION",
			version: object(
				member("_type", `""`),
				member("commit_audit", auditJSON("AUDIT_DETAILS")),
				member("lifecycle_state", codedComplete),
				member("data", payload("EHR_ACCESS"))),
			is: isWrapper[*contribution.OriginalVersion[rm.EHRAccess]],
		},
		{name: "no data member", version: originalJSON(), is: isWrapper[*contribution.OriginalVersion[rm.Composition]], noData: true},
		{name: "null data", version: originalJSON(member("data", "null")), is: isWrapper[*contribution.OriginalVersion[rm.Composition]], noData: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := bodyJSON(auditJSON("AUDIT_DETAILS"), tc.version)
			sub, err := contribution.UnmarshalSubmission(body)
			if err != nil {
				t.Fatalf("UnmarshalSubmission: %v", err)
			}
			if len(sub.Versions) != 1 {
				t.Fatalf("len(Versions) = %d, want 1", len(sub.Versions))
			}
			v := sub.Versions[0]
			if !tc.is(v) {
				t.Fatalf("Versions[0] is %T", v)
			}
			if tc.noData {
				ov := v.(*contribution.OriginalVersion[rm.Composition])
				if ov.Version.Data != nil {
					t.Errorf("Version.Data = %+v, want nil: the element carries no payload", ov.Version.Data)
				}
			}
			got, err := sub.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var encoded struct {
				Versions []map[string]json.RawMessage `json:"versions"`
			}
			if err := json.Unmarshal(got, &encoded); err != nil {
				t.Fatalf("encoding is not JSON: %v\n%s", err, got)
			}
			data, hasData := encoded.Versions[0]["data"]
			if tc.noData && hasData {
				t.Errorf("encoding writes data %s for an element read with none", data)
			}
			if tc.imported != "" {
				var item struct {
					UID struct {
						Value string `json:"value"`
					} `json:"uid"`
					Data struct {
						Type string `json:"_type"`
					} `json:"data"`
				}
				if err := json.Unmarshal(encoded.Versions[0]["item"], &item); err != nil {
					t.Fatalf("encoded item: %v\n%s", err, got)
				}
				if item.UID.Value != importedItemUID || item.Data.Type != tc.imported {
					t.Errorf("encoded item has uid %q and data _type %q, want %q and %q, the item read",
						item.UID.Value, item.Data.Type, importedItemUID, tc.imported)
				}
			}
		})
	}
}

// TestUnmarshalSubmissionAuditType decodes the batch audit and a version's
// commit_audit from each of the three forms the pin asks a server to accept,
// and keeps the form each was read with, so an omitted `_type` encodes back as
// the default AUDIT_DETAILS.
func TestUnmarshalSubmissionAuditType(t *testing.T) { // REQ-130
	cases := []struct {
		name     string
		read     string
		extra    []string // members after the others, such as an explicit empty `_type`
		wantType contribution.AuditType
		wantWire string
	}{
		{name: "AUDIT_DETAILS", read: "AUDIT_DETAILS", wantType: contribution.AuditTypeAuditDetails, wantWire: "AUDIT_DETAILS"},
		{name: "UPDATE_AUDIT", read: "UPDATE_AUDIT", wantType: contribution.AuditTypeUpdateAudit, wantWire: "UPDATE_AUDIT"},
		{name: "omitted", read: "", wantType: "", wantWire: "AUDIT_DETAILS"},
		{name: "empty", read: "", extra: []string{member("_type", `""`)}, wantType: "", wantWire: "AUDIT_DETAILS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			audit := func(extra ...string) string { return auditJSON(tc.read, append(extra, tc.extra...)...) }
			version := object(
				member("_type", `"ORIGINAL_VERSION"`),
				member("commit_audit", audit(member("description", `{"_type":"DV_TEXT","value":"why"}`))),
				member("lifecycle_state", codedComplete),
				member("data", compositionJSON))
			sub, err := contribution.UnmarshalSubmission(bodyJSON(audit(), version, importedJSONAudit(audit(), compositionJSON)))
			if err != nil {
				t.Fatalf("UnmarshalSubmission: %v", err)
			}
			if sub.Audit.Type != tc.wantType {
				t.Errorf("Audit.Type = %q, want %q", sub.Audit.Type, tc.wantType)
			}
			ov := sub.Versions[0].(*contribution.OriginalVersion[rm.Composition])
			if ov.CommitAudit.Type != tc.wantType {
				t.Errorf("commit_audit Type = %q, want %q", ov.CommitAudit.Type, tc.wantType)
			}
			if d, ok := ov.CommitAudit.Description.(*rm.DVText); !ok || d.Value != "why" {
				t.Errorf("commit_audit Description = %#v, want the DV_TEXT read", ov.CommitAudit.Description)
			}
			if c, ok := ov.CommitAudit.Committer.(*rm.PartyIdentified); !ok || c.Name == nil || *c.Name != "alice" {
				t.Errorf("commit_audit Committer = %#v, want the PARTY_IDENTIFIED read", ov.CommitAudit.Committer)
			}
			iv := sub.Versions[1].(*contribution.ImportedVersion[rm.Composition])
			if iv.CommitAudit.Type != tc.wantType {
				t.Errorf("IMPORTED_VERSION commit_audit Type = %q, want %q", iv.CommitAudit.Type, tc.wantType)
			}
			got, err := sub.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var encoded struct {
				Audit    map[string]any `json:"audit"`
				Versions []struct {
					CommitAudit map[string]any `json:"commit_audit"`
				} `json:"versions"`
			}
			if err := json.Unmarshal(got, &encoded); err != nil {
				t.Fatalf("encoding is not JSON: %v\n%s", err, got)
			}
			if encoded.Audit["_type"] != tc.wantWire {
				t.Errorf("audit._type = %v, want %q", encoded.Audit["_type"], tc.wantWire)
			}
			for i, v := range encoded.Versions {
				if v.CommitAudit["_type"] != tc.wantWire {
					t.Errorf("versions[%d].commit_audit._type = %v, want %q", i, v.CommitAudit["_type"], tc.wantWire)
				}
			}
		})
	}
}

// TestUnmarshalSubmissionDropsTimeCommitted reads an audit that carries a
// time_committed, which the write-side audit has no place for: the decode
// succeeds and the encoding leaves it out.
func TestUnmarshalSubmissionDropsTimeCommitted(t *testing.T) { // REQ-130
	timeCommitted := member("time_committed", `{"_type":"DV_DATE_TIME","value":"2026-05-17T10:00:00Z"}`)
	version := object(
		member("_type", `"ORIGINAL_VERSION"`),
		member("commit_audit", auditJSON("AUDIT_DETAILS", timeCommitted)),
		member("lifecycle_state", codedComplete),
		member("data", compositionJSON))
	sub, err := contribution.UnmarshalSubmission(bodyJSON(auditJSON("UPDATE_AUDIT", timeCommitted), version))
	if err != nil {
		t.Fatalf("UnmarshalSubmission: %v", err)
	}
	got, err := sub.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if bytes.Contains(got, []byte("time_committed")) {
		t.Errorf("encoding carries time_committed: %s", got)
	}
}

// TestUnmarshalSubmissionDropsEmptyWriteSideMembers reads a batch audit
// whose system_id is empty, a commit_audit whose system_id is null and a
// version whose uid has an empty value: the decode succeeds, and the encoding
// leaves all three out, as the write-side shape leaves out a member it does
// not hold.
func TestUnmarshalSubmissionDropsEmptyWriteSideMembers(t *testing.T) { // REQ-130
	audit := func(systemID string) string {
		return object(
			member("_type", `"AUDIT_DETAILS"`),
			member("system_id", systemID),
			member("committer", committerJSON),
			member("change_type", codedCreation))
	}
	version := object(
		member("_type", `"ORIGINAL_VERSION"`),
		member("commit_audit", audit("null")),
		member("uid", `{"_type":"OBJECT_VERSION_ID","value":""}`),
		member("lifecycle_state", codedComplete),
		member("data", compositionJSON))
	sub, err := contribution.UnmarshalSubmission(bodyJSON(audit(`""`), version))
	if err != nil {
		t.Fatalf("UnmarshalSubmission: %v", err)
	}
	got, err := sub.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	for _, name := range []string{"system_id", "uid"} {
		if bytes.Contains(got, []byte(`"`+name+`"`)) {
			t.Errorf("encoding carries %s: %s", name, got)
		}
	}
}

// TestUnmarshalSubmissionImportedPayloadKeepsNumbers reads an imported item
// whose payload spells a number with a fraction of zero, an integer past 2^53
// and a number no float64 holds: the decode accepts them and the encoding
// writes each as the body spells it, since the payload is held as the body's
// JSON text, and changing the input buffer after the decode does not reach
// the decoded submission.
func TestUnmarshalSubmissionImportedPayloadKeepsNumbers(t *testing.T) { // REQ-130
	data := strings.Replace(compositionJSON, `"composer":`, `"f":1.0,"n":9007199254740993,"g":1e400,"composer":`, 1)
	if data == compositionJSON {
		t.Fatal("test setup: the composition fixture no longer carries the composer member this test writes before")
	}
	body := bodyJSON(auditJSON("AUDIT_DETAILS"), importedJSON(data))
	want := bytes.Clone(body)
	sub, err := contribution.UnmarshalSubmission(body)
	if err != nil {
		t.Fatalf("UnmarshalSubmission: %v", err)
	}
	for i := range body {
		body[i] = ' '
	}
	got, err := sub.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if ok, diff := wireequiv.Equivalent(want, got); !ok {
		t.Errorf("encoding is not wire-equivalent to the body: %s\ngot: %s", diff, got)
	}
}

// TestUnmarshalSubmissionMissingImportedItem reads an imported item that
// carries none of its members: it decodes, and canjson.WithMissing reports
// each member the RM makes mandatory on it.
func TestUnmarshalSubmissionMissingImportedItem(t *testing.T) { // REQ-130, REQ-175
	audit := auditJSON("AUDIT_DETAILS")
	body := bodyJSON(audit, object(
		member("_type", `"IMPORTED_VERSION"`),
		member("commit_audit", audit),
		member("item", "{}")))
	var got []string
	if _, err := contribution.UnmarshalSubmission(body, canjson.WithMissing(func(p canjson.Pointer) { got = append(got, p.String()) })); err != nil {
		t.Fatalf("UnmarshalSubmission: %v", err)
	}
	slices.Sort(got)
	want := []string{
		"/versions/0/item/commit_audit",
		"/versions/0/item/contribution",
		"/versions/0/item/lifecycle_state",
		"/versions/0/item/uid",
	}
	if !slices.Equal(got, want) {
		t.Errorf("WithMissing received %q, want %q", got, want)
	}
}

// TestUnmarshalSubmissionRefusals pins each body the decoder cannot place: a
// *canjson.DecodeError at the top of the chain whose Path is the JSON Pointer
// of the offending member from the root of the body, whose text holds no
// value from the body, and which wraps the sentinel of its kind. A failure
// inside a payload or an audit comes back with its path from the body's root.
func TestUnmarshalSubmissionRefusals(t *testing.T) { // REQ-130, REQ-052
	seedText := object(member("_type", `"DV_TEXT"`), member("value", strconv.Quote(decodeSeed)))
	seededAudit := auditJSON("AUDIT_DETAILS", member("description", seedText))
	seededComposition := strings.TrimSuffix(compositionJSON, "}") + "," + member("uid", object(member("_type", `"HIER_OBJECT_ID"`), member("value", strconv.Quote(decodeSeed)))) + "}"
	withData := func(data string) string { return originalJSON(member("data", data)) }
	cases := []struct {
		name     string
		body     []byte
		wantPath string
		want     error
	}{
		{name: "body is an array", body: []byte(`["` + decodeSeed + `"]`), wantPath: "", want: canjson.ErrInvalidShape},
		{name: "body is a string", body: []byte(strconv.Quote(decodeSeed)), wantPath: "", want: canjson.ErrInvalidShape},
		{name: "body is null", body: []byte("null"), wantPath: "", want: canjson.ErrInvalidShape},
		{name: "versions absent", body: []byte(object(member("audit", seededAudit))), wantPath: "/versions", want: canjson.ErrInvalidShape},
		{name: "versions null", body: []byte(object(member("audit", seededAudit), member("versions", "null"))), wantPath: "/versions", want: canjson.ErrInvalidShape},
		{name: "versions empty", body: bodyJSON(seededAudit), wantPath: "/versions", want: canjson.ErrInvalidShape},
		{name: "versions an object", body: []byte(object(member("audit", seededAudit), member("versions", "{}"))), wantPath: "/versions", want: canjson.ErrInvalidShape},
		{name: "version not an object", body: bodyJSON(seededAudit, withData(compositionJSON), strconv.Quote(decodeSeed)), wantPath: "/versions/1", want: canjson.ErrInvalidShape},
		{
			name:     "IMPORTED_VERSION with no item",
			body:     bodyJSON(seededAudit, object(member("_type", `"IMPORTED_VERSION"`), member("commit_audit", seededAudit))),
			wantPath: "/versions/0/item", want: canjson.ErrInvalidShape,
		},
		{
			name:     "IMPORTED_VERSION with a null item",
			body:     bodyJSON(seededAudit, object(member("_type", `"IMPORTED_VERSION"`), member("commit_audit", seededAudit), member("item", "null"))),
			wantPath: "/versions/0/item", want: canjson.ErrInvalidShape,
		},
		{
			name:     "version _type names another RM class",
			body:     bodyJSON(seededAudit, strings.Replace(withData(compositionJSON), `"ORIGINAL_VERSION"`, `"OBJECT_REF"`, 1)),
			wantPath: "/versions/0/_type", want: typereg.ErrTypeMismatch,
		},
		{
			name:     "version _type names no RM class",
			body:     bodyJSON(seededAudit, strings.Replace(withData(compositionJSON), `"ORIGINAL_VERSION"`, `"NEW_VERSION"`, 1)),
			wantPath: "/versions/0/_type", want: typereg.ErrUnknownType,
		},
		{
			name:     "version _type not a string",
			body:     bodyJSON(seededAudit, strings.Replace(withData(compositionJSON), `"ORIGINAL_VERSION"`, `7`, 1)),
			wantPath: "/versions/0/_type", want: canjson.ErrInvalidShape,
		},
		{name: "audit _type ATTESTATION", body: bodyJSON(auditJSON("ATTESTATION", member("description", seedText)), withData(compositionJSON)), wantPath: "/audit/_type", want: typereg.ErrTypeMismatch},
		{name: "audit not an object", body: []byte(object(member("audit", strconv.Quote(decodeSeed)), member("versions", "["+withData(compositionJSON)+"]"))), wantPath: "/audit", want: canjson.ErrInvalidShape},
		{
			name: "commit_audit _type names no RM class",
			body: bodyJSON(seededAudit, withData(compositionJSON), object(
				member("_type", `"ORIGINAL_VERSION"`),
				member("commit_audit", auditJSON("PERSISTED_AUDIT")),
				member("lifecycle_state", codedComplete),
				member("data", compositionJSON))),
			wantPath: "/versions/1/commit_audit/_type", want: typereg.ErrUnknownType,
		},
		{name: "data has no _type", body: bodyJSON(seededAudit, withData(object(member("archetype_node_id", strconv.Quote(decodeSeed))))), wantPath: "/versions/0/data", want: typereg.ErrMissingType},
		{name: "data has an empty _type", body: bodyJSON(seededAudit, withData(object(member("_type", `""`), member("archetype_node_id", strconv.Quote(decodeSeed))))), wantPath: "/versions/0/data", want: typereg.ErrMissingType},
		{name: "data _type outside the four", body: bodyJSON(seededAudit, withData(object(member("_type", `"CONTRIBUTION"`), member("uid", seedText)))), wantPath: "/versions/0/data/_type", want: typereg.ErrTypeMismatch},
		{name: "data _type not a string", body: bodyJSON(seededAudit, withData(object(member("_type", "true")))), wantPath: "/versions/0/data/_type", want: canjson.ErrInvalidShape},
		{name: "data not an object", body: bodyJSON(seededAudit, withData(strconv.Quote(decodeSeed))), wantPath: "/versions/0/data", want: canjson.ErrInvalidShape},
		{name: "item.data _type outside the four", body: bodyJSON(seededAudit, importedJSON(object(member("_type", `"PERSON"`)))), wantPath: "/versions/0/item/data/_type", want: typereg.ErrTypeMismatch},
		{name: "item.data has no _type", body: bodyJSON(seededAudit, importedJSON(object(member("archetype_node_id", strconv.Quote(decodeSeed))))), wantPath: "/versions/0/item/data", want: typereg.ErrMissingType},
		{
			name:     "unknown _type inside a payload",
			body:     bodyJSON(seededAudit, withData(compositionJSON), withData(strings.TrimSuffix(seededComposition, "}")+`,"content":[{"_type":"NO_SUCH_ENTRY"}]}`)),
			wantPath: "/versions/1/data/content/0", want: typereg.ErrUnknownType,
		},
		{
			name:     "wrong shape inside a payload",
			body:     bodyJSON(seededAudit, withData(strings.Replace(compositionJSON, `{"_type":"DV_TEXT","value":"Report"}`, `{"_type":"DV_TEXT","value":987654321}`, 1))),
			wantPath: "/versions/0/data/name/value", want: canjson.ErrInvalidShape,
		},
		{
			name:     "unknown committer _type in a commit_audit",
			body:     bodyJSON(seededAudit, strings.Replace(withData(seededComposition), `"PARTY_IDENTIFIED"`, `"PARTY_UNKNOWN"`, 1)),
			wantPath: "/versions/0/commit_audit/committer", want: typereg.ErrUnknownType,
		},
		{
			name:     "wrong shape in the batch audit",
			body:     bodyJSON(strings.Replace(seededAudit, `"value":"creation"`, `"value":987654321`, 1), withData(compositionJSON)),
			wantPath: "/audit/change_type/value", want: canjson.ErrInvalidShape,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := contribution.UnmarshalSubmission(tc.body)
			if err == nil {
				t.Fatalf("UnmarshalSubmission decoded %s\ninto %+v, want a refusal", tc.body, sub)
			}
			if sub != nil {
				t.Errorf("UnmarshalSubmission returned a submission with its error: %+v", sub)
			}
			de, ok := err.(*canjson.DecodeError) //nolint:errorlint // the refusal is the envelope itself, found without unwrapping
			if !ok {
				t.Fatalf("error is %T (%v), want a *canjson.DecodeError at the top of the chain", err, err)
			}
			if de.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q (error: %v)", de.Path, tc.wantPath, err)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("error %v does not wrap %v", err, tc.want)
			}
			for _, value := range []string{decodeSeed, "987654321"} {
				if strings.Contains(err.Error(), value) {
					t.Errorf("error text carries the value %q from the body: %v", value, err)
				}
			}
		})
	}
}

// TestUnmarshalSubmissionMalformed refuses a body that is not valid JSON, or
// holds a member twice, as canjson refuses one: a *canjson.DecodeError whose
// text carries no value from the body.
func TestUnmarshalSubmissionMalformed(t *testing.T) { // REQ-130, REQ-052
	valid := string(bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", compositionJSON))))
	cases := []struct {
		name string
		body string
		want error // a sentinel the refusal wraps, or nil
	}{
		{name: "empty", body: ""},
		{name: "cut short", body: valid[:len(valid)/2] + `"` + decodeSeed},
		{name: "trailing content", body: valid + `"` + decodeSeed + `"`},
		{name: "duplicate member", body: strings.TrimSuffix(valid, "}") + `,"audit":"` + decodeSeed + `"}`, want: canjson.ErrInvalidShape},
		{name: "duplicate member in a payload", body: strings.Replace(valid, `"archetype_node_id"`, `"archetype_node_id":"`+decodeSeed+`","archetype_node_id"`, 1), want: canjson.ErrInvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contribution.UnmarshalSubmission([]byte(tc.body))
			if _, ok := err.(*canjson.DecodeError); !ok { //nolint:errorlint // the refusal is the envelope itself, found without unwrapping
				t.Fatalf("error is %T (%v), want a *canjson.DecodeError at the top of the chain", err, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("error %v does not wrap %v", err, tc.want)
			}
			if strings.Contains(err.Error(), decodeSeed) {
				t.Errorf("error text carries a value from the body: %v", err)
			}
		})
	}
}

// TestUnmarshalSubmissionValidateFails returns Validate's error, and no
// submission, for a body Validate refuses: an audit with no committer.
func TestUnmarshalSubmissionValidateFails(t *testing.T) { // REQ-130
	noCommitter := object(member("_type", `"AUDIT_DETAILS"`), member("system_id", `"cdr.example"`), member("change_type", codedCreation))
	cases := []struct {
		name string
		body []byte
		want string
	}{
		{name: "batch audit", body: bodyJSON(noCommitter, originalJSON(member("data", compositionJSON))), want: "Submission.Audit: committer is nil"},
		{name: "batch audit absent", body: []byte(object(member("versions", "["+originalJSON(member("data", compositionJSON))+"]"))), want: "Submission.Audit: committer is nil"},
		{
			name: "commit_audit",
			body: bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", compositionJSON)), object(
				member("_type", `"ORIGINAL_VERSION"`),
				member("commit_audit", noCommitter),
				member("lifecycle_state", codedComplete))),
			want: "Submission.Versions[1]: commit_audit.committer is nil",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := contribution.UnmarshalSubmission(tc.body)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("UnmarshalSubmission = %v, want Validate's error %q", err, tc.want)
			}
			if sub != nil {
				t.Errorf("UnmarshalSubmission returned a submission with its error: %+v", sub)
			}
		})
	}
}

// encodeSubmission decodes body with opts and returns the encoding of the
// result.
func encodeSubmission(t *testing.T, body []byte, opts ...canjson.DecoderOption) []byte {
	t.Helper()
	sub, err := contribution.UnmarshalSubmission(body, opts...)
	if err != nil {
		t.Fatalf("UnmarshalSubmission: %v", err)
	}
	b, err := sub.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	return b
}

// TestUnmarshalSubmissionUnmatched passes every unmatched member of the
// audits, the versions and their payloads to WithUnmatched, in document order
// and by its JSON Pointer from the root of the body, and decodes the same
// value as without the option; WithStrict refuses the first.
func TestUnmarshalSubmissionUnmatched(t *testing.T) { // REQ-130, REQ-170
	extension := strconv.Quote(decodeSeed)
	body := bodyJSON(
		auditJSON("UPDATE_AUDIT", member("x_audit", extension)),
		object(
			member("_type", `"ORIGINAL_VERSION"`),
			member("x_version", extension),
			member("commit_audit", auditJSON("", member("x_commit", extension))),
			member("lifecycle_state", codedComplete),
			member("data", strings.TrimSuffix(compositionJSON, "}")+","+member("x_data", extension)+"}")),
		importedJSON(strings.TrimSuffix(compositionJSON, "}")+","+member("x_item_data", `{"a":1}`)+"}"),
	)
	want := []string{
		"/audit/x_audit",
		"/versions/0/x_version",
		"/versions/0/commit_audit/x_commit",
		"/versions/0/data/x_data",
	}
	var got []string
	withOption := encodeSubmission(t, body, canjson.WithUnmatched(func(p canjson.Pointer) { got = append(got, p.String()) }))
	if !slices.Equal(got, want) {
		t.Errorf("WithUnmatched received %q, want %q", got, want)
	}
	if def := encodeSubmission(t, body); !bytes.Equal(withOption, def) {
		t.Errorf("decode with WithUnmatched differs from the default\nwith:    %s\ndefault: %s", withOption, def)
	}

	// With the audit written after versions, its member comes last.
	auditLast := []byte(object(
		member("versions", "["+originalJSON(member("x_version", extension), member("data", compositionJSON))+"]"),
		member("audit", auditJSON("AUDIT_DETAILS", member("x_audit", extension)))))
	got = nil
	encodeSubmission(t, auditLast, canjson.WithUnmatched(func(p canjson.Pointer) { got = append(got, p.String()) }))
	if want := []string{"/versions/0/x_version", "/audit/x_audit"}; !slices.Equal(got, want) {
		t.Errorf("WithUnmatched received %q for a body whose audit comes last, want %q", got, want)
	}

	_, err := contribution.UnmarshalSubmission(body, canjson.WithStrict())
	de, ok := err.(*canjson.DecodeError) //nolint:errorlint // the refusal is the envelope itself, found without unwrapping
	if !ok {
		t.Fatalf("WithStrict: error is %T (%v), want a *canjson.DecodeError", err, err)
	}
	if de.Path != want[0] {
		t.Errorf("WithStrict: Path = %q, want %q", de.Path, want[0])
	}
	if !errors.Is(err, canjson.ErrUnknownMember) {
		t.Errorf("WithStrict: error %v does not wrap canjson.ErrUnknownMember", err)
	}
	if strings.Contains(err.Error(), decodeSeed) {
		t.Errorf("WithStrict: error text carries the member's value: %v", err)
	}
}

// TestUnmarshalSubmissionStrictInPayload refuses, under WithStrict, an
// unmatched member inside a payload, naming it by its pointer from the root of
// the body.
func TestUnmarshalSubmissionStrictInPayload(t *testing.T) { // REQ-130, REQ-170
	data := strings.TrimSuffix(compositionJSON, "}") + "," + member("x_note", strconv.Quote(decodeSeed)) + "}"
	body := bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", compositionJSON)), originalJSON(member("data", data)))
	_, err := contribution.UnmarshalSubmission(body, canjson.WithStrict())
	de, ok := err.(*canjson.DecodeError) //nolint:errorlint // the refusal is the envelope itself, found without unwrapping
	if !ok {
		t.Fatalf("error is %T (%v), want a *canjson.DecodeError", err, err)
	}
	if de.Path != "/versions/1/data/x_note" {
		t.Errorf("Path = %q, want %q", de.Path, "/versions/1/data/x_note")
	}
	if !errors.Is(err, canjson.ErrUnknownMember) {
		t.Errorf("error %v does not wrap canjson.ErrUnknownMember", err)
	}
}

// TestUnmarshalSubmissionMissing passes a payload's missing member to
// WithMissing by its pointer from the root of the body, and never a member the
// write-side shape leaves out by design: time_committed and system_id on an
// audit, and a version's uid and contribution, whether absent or null.
func TestUnmarshalSubmissionMissing(t *testing.T) { // REQ-130, REQ-175
	noLanguage := strings.Replace(compositionJSON, `"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},`, "", 1)
	if noLanguage == compositionJSON {
		t.Fatal("test setup: the composition fixture no longer carries the language member this test removes")
	}
	noSystemID := object(
		member("_type", `"AUDIT_DETAILS"`),
		member("committer", committerJSON),
		member("change_type", codedCreation),
		member("time_committed", "null"))
	body := bodyJSON(
		noSystemID,
		originalJSON(member("data", noLanguage)),
		importedJSON(compositionJSON),
		object(
			member("_type", `"ORIGINAL_VERSION"`),
			member("contribution", "null"),
			member("commit_audit", auditJSON("UPDATE_AUDIT", member("time_committed", "null"))),
			member("uid", "null"),
			member("lifecycle_state", codedComplete),
			member("data", compositionJSON)),
	)
	var got []string
	withOption := encodeSubmission(t, body, canjson.WithMissing(func(p canjson.Pointer) { got = append(got, p.String()) }))
	if want := []string{"/versions/0/data/language"}; !slices.Equal(got, want) {
		t.Errorf("WithMissing received %q, want %q", got, want)
	}
	if def := encodeSubmission(t, body); !bytes.Equal(withOption, def) {
		t.Errorf("decode with WithMissing differs from the default\nwith:    %s\ndefault: %s", withOption, def)
	}
	// Strict refuses nothing for a missing member.
	if _, err := contribution.UnmarshalSubmission(body, canjson.WithStrict()); err != nil {
		t.Errorf("WithStrict refused a body whose only fault is a missing member: %v", err)
	}
}

// TestUnmarshalSubmissionFailedDecodeReportsNothing checks the members only
// once the whole body has decoded and passed Validate: a later version that
// fails to decode, or a submission Validate refuses, reports nothing, and a
// decode error is returned as itself under WithStrict, ahead of an earlier
// unmatched member.
func TestUnmarshalSubmissionFailedDecodeReportsNothing(t *testing.T) { // REQ-130, REQ-170, REQ-175
	noLanguage := strings.Replace(compositionJSON, `"language":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"},`, "", 1)
	unmatched := strings.TrimSuffix(noLanguage, "}") + "," + member("x_note", `"note"`) + "}"
	undecodable := strings.TrimSuffix(compositionJSON, "}") + `,"content":[{"_type":"NO_SUCH_ENTRY"}]}`
	noCommitter := object(member("system_id", `"cdr.example"`), member("change_type", codedCreation))
	cases := []struct {
		name string
		body []byte
		// want is a sentinel the error wraps.
		want error
	}{
		{name: "a later version fails to decode", body: bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", unmatched)), originalJSON(member("data", undecodable))), want: typereg.ErrUnknownType},
		{name: "a later version fails its _type check", body: bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", unmatched)), originalJSON(member("data", `{"_type":"PERSON"}`))), want: typereg.ErrTypeMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reported []string
			report := func(p canjson.Pointer) { reported = append(reported, p.String()) }
			_, err := contribution.UnmarshalSubmission(tc.body, canjson.WithUnmatched(report), canjson.WithMissing(report), canjson.WithStrict())
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want one wrapping %v, ahead of the strict refusal", err, tc.want)
			}
			if errors.Is(err, canjson.ErrUnknownMember) {
				t.Errorf("error = %v: a strict refusal came back ahead of the decode error", err)
			}
			if len(reported) > 0 {
				t.Errorf("a decode that fails reported %q", reported)
			}
		})
	}
	t.Run("Validate refuses the submission", func(t *testing.T) {
		var reported []string
		report := func(p canjson.Pointer) { reported = append(reported, p.String()) }
		body := bodyJSON(noCommitter, originalJSON(member("data", unmatched)))
		_, err := contribution.UnmarshalSubmission(body, canjson.WithUnmatched(report), canjson.WithMissing(report))
		if err == nil || !strings.Contains(err.Error(), "committer is nil") {
			t.Errorf("error = %v, want Validate's", err)
		}
		if len(reported) > 0 {
			t.Errorf("a decode Validate refuses reported %q", reported)
		}
	})
}

// TestUnmarshalSubmissionRelaxedTypeDispatch accepts the dispatch option the
// other canjson decodes accept.
func TestUnmarshalSubmissionRelaxedTypeDispatch(t *testing.T) { // REQ-130
	body := bodyJSON(auditJSON("AUDIT_DETAILS"), originalJSON(member("data", compositionJSON)))
	if got, def := encodeSubmission(t, body, canjson.WithRelaxedTypeDispatch(true)), encodeSubmission(t, body); !bytes.Equal(got, def) {
		t.Errorf("decode with WithRelaxedTypeDispatch differs from the default\nwith:    %s\ndefault: %s", got, def)
	}
}
