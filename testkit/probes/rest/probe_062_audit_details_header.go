package restprobes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/contribution"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// auditDetailsAttributes are the AUDIT_DETAILS attributes the
// `openehr-audit-details` grammar documents (resources/its-rest/
// overview-validation.openapi.yaml, "openehr-version and
// openehr-audit-details"). A key outside this set is either a typo or an
// invented extension: either way a server is not obliged to understand it.
var auditDetailsAttributes = []string{
	"change_type.code_string",
	"description.value",
	"committer.name",
	"committer.external_ref.id",
	"committer.external_ref.namespace",
	"committer.external_ref.type",
	"system_id",
}

// Probe062AuditDetailsHeader implements PROBE-062: a write carrying audit
// details emits the `openehr-audit-details` request header in the openEHR
// dotted-attribute grammar (not JSON), and the CONTRIBUTION the write joined
// reflects the same fields on read-back (REQ-059).
//
// The probe holds two independent oracles, neither of which is the SDK's own
// encoder:
//
//   - Grammar. The header is re-parsed by [parseAuditDetailsHeader], written
//     here from the normative grammar in the ITS-REST contract and calling
//     nothing in openehr/client/ehr. Comparing the header against
//     MarshalAuditDetails instead would only prove the encoder agrees with
//     itself; re-parsing proves the bytes on the wire are a well-formed
//     assignment list whose attributes are documented ones carrying the values
//     the caller asked for.
//   - Read-back, bound to the write. The CONTRIBUTION is fetched and its audit
//     envelope compared with the committed one — and the version uid the write
//     returned (from its `Location`) must appear among the contribution's
//     `versions`. Without that binding the read-back could be satisfied by any
//     contribution that happened to carry the same audit fields.
//
// contributionUID is supplied by the caller because the single-resource write
// does not return one: `POST /ehr/{ehr_id}/composition` names the new version,
// not the contribution it was folded into. In Live mode a caller recovers the
// uid from the versioned object's revision history; in Sandbox and Cassette
// modes the scripted backend fixes it.
//
// captured returns the requests the backend received; the probe reads the write
// request to check the header grammar and route, then the read-back request to
// check its route.
func Probe062AuditDetailsHeader(ctx context.Context, c *transport.Client, captured func() []*http.Request, ehrID openehrclient.EHRID, contributionUID string, audit *rm.AuditDetails, comp *rm.Composition) (Result, error) {
	r := Result{Probe: "PROBE-062"}
	if c == nil || ehrID == "" || contributionUID == "" || audit == nil || comp == nil {
		return r, errors.New("PROBE-062: missing required inputs (client/ehr/contributionUID/audit/comp)")
	}
	if captured == nil {
		return r, errors.New("PROBE-062: nil captured-request recorder")
	}

	// Write arm — the audit details ride the openehr-audit-details header.
	before := len(captured())
	_, wmeta, err := composition.Save(ctx, c, ehrID, comp, composition.WithAuditDetails(audit))
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("audit-carrying write failed: %v", err)
		return r, nil
	}
	reqs := captured()
	if n := len(reqs) - before; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("write issued %d requests, want exactly 1", n)
		return r, nil
	}
	writeReq := reqs[len(reqs)-1]
	wantWritePath, err := servicePath(c, "/ehr/"+string(ehrID)+"/composition")
	if err != nil {
		return r, fmt.Errorf("PROBE-062: %w", err)
	}
	if writeReq.URL.Path != wantWritePath {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("write path %q, want the composition route %q", writeReq.URL.Path, wantWritePath)
		return r, nil
	}
	if fault := auditHeaderFault(writeReq.Header.Get("openehr-audit-details"), audit); fault != "" {
		r.Status = "fail"
		r.Detail = fault
		return r, nil
	}
	// The version uid the write returned is what binds the read-back below to
	// this write; without a Location there is nothing to bind to.
	if wmeta == nil || wmeta.VersionUID == "" {
		r.Status = "fail"
		r.Detail = "the write returned no usable Location, so no version uid can bind the Contribution read-back to it"
		return r, nil
	}
	versionUID := string(wmeta.VersionUID)

	// Read-back arm — the Contribution's audit reflects the committed fields.
	beforeRead := len(captured())
	contrib, _, err := contribution.Get(ctx, c, ehrID, contributionUID)
	if err != nil {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Contribution read-back failed: %v", err)
		return r, nil
	}
	if contrib == nil || contrib.Audit == nil {
		r.Status = "fail"
		r.Detail = "Contribution read-back carried no audit envelope"
		return r, nil
	}
	readReqs := captured()
	if n := len(readReqs) - beforeRead; n != 1 {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Contribution read-back issued %d requests, want exactly 1", n)
		return r, nil
	}
	wantReadPath, err := servicePath(c, "/ehr/"+string(ehrID)+"/contribution/"+contributionUID)
	if err != nil {
		return r, fmt.Errorf("PROBE-062: %w", err)
	}
	if got := readReqs[len(readReqs)-1].URL.Path; got != wantReadPath {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("Contribution read-back path %q, want the contribution route %q", got, wantReadPath)
		return r, nil
	}
	if code := contrib.Audit.GetChangeType().DefiningCode.CodeString; code != audit.ChangeType.DefiningCode.CodeString {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back change_type code = %q, want %q", code, audit.ChangeType.DefiningCode.CodeString)
		return r, nil
	}
	if sysID := contrib.Audit.GetSystemID(); sysID != audit.SystemID {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("read-back system_id = %q, want %q", sysID, audit.SystemID)
		return r, nil
	}
	if wantName, ok := committerName(audit.Committer); ok {
		gotName, gotOK := committerName(contrib.Audit.GetCommitter())
		if !gotOK {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("read-back committer (%T) names nobody, want %q", contrib.Audit.GetCommitter(), wantName)
			return r, nil
		}
		if gotName != wantName {
			r.Status = "fail"
			r.Detail = fmt.Sprintf("read-back committer name = %q, want %q", gotName, wantName)
			return r, nil
		}
	}
	// Bind the read-back to the write: this contribution must list the version
	// the write created.
	listed := contributionVersionIDs(contrib)
	if !slices.Contains(listed, versionUID) {
		r.Status = "fail"
		r.Detail = fmt.Sprintf("the Contribution does not list the version the write created (%q); its versions are %v", versionUID, listed)
		return r, nil
	}

	r.Status = "pass"
	r.Detail = fmt.Sprintf("openehr-audit-details parses as the dotted grammar; the Contribution lists version %q and its audit reflects the committed fields", versionUID)
	return r, nil
}

// auditHeaderFault checks the raw `openehr-audit-details` header value against
// the ITS-REST grammar and the audit the caller committed. It returns "" when
// the header is well-formed and faithful, and otherwise the probe detail
// describing the discrepancy.
func auditHeaderFault(got string, audit *rm.AuditDetails) string {
	if got == "" {
		return "write carried no openehr-audit-details header"
	}
	// Checked before parsing so the most likely wrong-shape failure — a JSON
	// object where the grammar belongs — is named for what it is.
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		return fmt.Sprintf("openehr-audit-details is JSON (%q); REQ-059 requires the dotted-attribute grammar", got)
	}
	attrs, err := parseAuditDetailsHeader(got)
	if err != nil {
		return fmt.Sprintf("openehr-audit-details %q does not parse as the dotted-attribute grammar: %v", got, err)
	}
	for _, key := range slices.Sorted(maps.Keys(attrs)) {
		if !slices.Contains(auditDetailsAttributes, key) {
			return fmt.Sprintf("openehr-audit-details carries the undocumented attribute %q; the grammar documents %v", key, auditDetailsAttributes)
		}
	}
	if want := audit.ChangeType.DefiningCode.CodeString; attrs["change_type.code_string"] != want {
		return fmt.Sprintf("openehr-audit-details header change_type.code_string = %q, want %q", attrs["change_type.code_string"], want)
	}
	if want, ok := committerName(audit.Committer); ok && attrs["committer.name"] != want {
		return fmt.Sprintf("openehr-audit-details header committer.name = %q, want %q", attrs["committer.name"], want)
	}
	if want := audit.SystemID; want != "" && attrs["system_id"] != want {
		return fmt.Sprintf("openehr-audit-details header system_id = %q, want %q", attrs["system_id"], want)
	}
	if audit.Description != nil {
		if want := audit.Description.GetValue(); want != "" && attrs["description.value"] != want {
			return fmt.Sprintf("openehr-audit-details header description.value = %q, want %q", attrs["description.value"], want)
		}
	}
	return ""
}

// contributionVersionIDs projects the id values out of a CONTRIBUTION's
// `versions` references. An entry whose id is of an unrecognised type is
// skipped — it names no version this probe can bind to, which surfaces as the
// binding check failing rather than as a silent match.
func contributionVersionIDs(contrib *rm.Contribution) []string {
	out := make([]string, 0, len(contrib.Versions))
	for _, v := range contrib.Versions {
		if v == nil {
			continue
		}
		if val, ok := rm.ObjectIDValue(v.GetID()); ok && val != "" {
			out = append(out, val)
		}
	}
	return out
}

// committerName extracts the human-readable name from a PARTY_PROXY committer.
// ok is false when the proxy names nobody: PARTY_SELF carries no name at all,
// a nil pointer carries nothing, and an unrecognised proxy type is one this
// probe cannot read — each leaves the caller to fail loudly rather than
// silently compare against an empty string.
func committerName(p rm.PartyProxy) (string, bool) {
	switch v := p.(type) {
	case rm.PartyIdentified:
		return nameOf(v.Name)
	case *rm.PartyIdentified:
		if v == nil {
			return "", false
		}
		return nameOf(v.Name)
	case rm.PartyRelated:
		return nameOf(v.Name)
	case *rm.PartyRelated:
		if v == nil {
			return "", false
		}
		return nameOf(v.Name)
	case rm.PartySelf, *rm.PartySelf:
		return "", false
	default:
		return "", false
	}
}

func nameOf(s *string) (string, bool) {
	if s == nil {
		return "", false
	}
	return *s, true
}

// parseAuditDetailsHeader parses an `openehr-audit-details` header value into
// its attribute assignments, strictly, per the grammar in the ITS-REST
// contract (resources/its-rest/overview-validation.openapi.yaml):
//
//	header     = assignment *( "," assignment )
//	assignment = key "=" DQUOTE value DQUOTE
//	key        = name *( "." name )
//	name       = ( ALPHA / "_" ) *( ALPHA / DIGIT / "_" )
//	value      = *( escaped / <any char except DQUOTE and "\"> )
//	escaped    = "\" ( DQUOTE / "\" )
//
// No whitespace is permitted around "=" or ","; a duplicate key and an empty
// header are both errors. Returns the unescaped values keyed by attribute.
//
// This is the probe's independent oracle for the header shape: it deliberately
// calls nothing in openehr/client/ehr, so a bug shared between the SDK's
// encoder and this parser cannot pass unnoticed.
func parseAuditDetailsHeader(s string) (map[string]string, error) {
	if s == "" {
		return nil, errors.New("the header value is empty")
	}
	out := make(map[string]string)
	for i := 0; ; {
		key, next, err := scanAuditKey(s, i)
		if err != nil {
			return nil, err
		}
		i = next
		if i >= len(s) || s[i] != '=' {
			return nil, fmt.Errorf(`attribute %q is not followed by "=" at offset %d`, key, i)
		}
		value, next, err := scanAuditValue(s, i+1)
		if err != nil {
			return nil, err
		}
		i = next
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("attribute %q is assigned more than once", key)
		}
		out[key] = value
		if i == len(s) {
			return out, nil
		}
		if s[i] != ',' {
			return nil, fmt.Errorf(`attribute %q is followed by %q at offset %d, want "," or the end of the value`, key, string(s[i]), i)
		}
		i++
	}
}

// scanAuditKey reads a dotted attribute name starting at i and returns it with
// the offset just past it.
func scanAuditKey(s string, i int) (key string, next int, err error) {
	start := i
	for {
		seg := i
		for i < len(s) && isAuditKeyByte(s[i], i == seg) {
			i++
		}
		if i == seg {
			return "", 0, fmt.Errorf("expected an attribute name at offset %d", seg)
		}
		if i < len(s) && s[i] == '.' {
			i++
			continue
		}
		return s[start:i], i, nil
	}
}

func isAuditKeyByte(b byte, first bool) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b == '_':
		return true
	case b >= '0' && b <= '9':
		return !first
	default:
		return false
	}
}

// scanAuditValue reads a double-quoted, backslash-escaped value starting at i
// (which must be the opening quote) and returns the unescaped text with the
// offset just past the closing quote.
func scanAuditValue(s string, i int) (value string, next int, err error) {
	if i >= len(s) || s[i] != '"' {
		return "", 0, fmt.Errorf("expected a double-quoted value at offset %d", i)
	}
	var b strings.Builder
	for i++; i < len(s); {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) {
				return "", 0, fmt.Errorf("the value ends with a dangling escape at offset %d", i)
			}
			esc := s[i+1]
			if esc != '"' && esc != '\\' {
				return "", 0, fmt.Errorf(`unsupported escape "\%s" at offset %d; only \" and \\ are defined`, string(esc), i)
			}
			b.WriteByte(esc)
			i += 2
		case '"':
			return b.String(), i + 1, nil
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return "", 0, errors.New("the value has no closing double quote")
}
