package probe_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// The error-envelopes scenario reads one fixed EHR id no deployment holds,
// then runs three fixed queries the server refuses, in this order.
const (
	cassetteUnknownEHRID        = "7d44b88c-4199-4bad-97dc-d78268e01398"
	cassetteMalformedAQL        = "SELECT e/ehr_id/value FROM"
	cassetteUnknownAttributeAQL = "SELECT c/no_such_attribute FROM EHR e CONTAINS COMPOSITION c"
	cassetteUnsupportedPathAQL  = "SELECT e/no_such_attribute FROM EHR e"
)

// rawReplayClientFor returns a client that replays har, reaches no network,
// and keeps error bodies: the envelope message and the raw bytes, which a
// default client withholds (REQ-093).
func rawReplayClientFor(t *testing.T, har probe.HAR) *transport.Client {
	t.Helper()
	const base = "https://sandbox.local/openehr/v1"
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: base,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: u, SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat,
		transport.WithHTTPClient(probe.NewReplayer(har).HTTPClient()),
		transport.WithRawErrorBodies(true))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// recordedEntries returns, in capture order, every exchange in har whose
// method is method and whose URL ends in suffix.
func recordedEntries(har *probe.HAR, method, suffix string) []*probe.HAREntry {
	var out []*probe.HAREntry
	for i := range har.Log.Entries {
		e := &har.Log.Entries[i]
		if e.Request.Method == method && strings.HasSuffix(e.Request.URL, suffix) {
			out = append(out, e)
		}
	}
	return out
}

// recordedQueries returns the recording's ad-hoc query exchanges in the
// order they were captured. It fails the test unless there are three, one
// per query the scenario sends.
func recordedQueries(t *testing.T, har *probe.HAR) []*probe.HAREntry {
	t.Helper()
	out := recordedEntries(har, "POST", "/query/aql")
	if len(out) != 3 {
		t.Fatalf("the recording holds %d ad-hoc query exchanges, want 3", len(out))
	}
	return out
}

// recordedMessage returns the message field of a recorded error body.
func recordedMessage(e *probe.HAREntry) string {
	var body struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(e.Response.Content.Text), &body)
	return body.Message
}

// replayErrorEnvelopes drives the error-envelopes recording through c, a
// client that keeps error bodies, and checks what the SDK makes of each
// recorded refusal (PROBE-068, PROBE-021, REQ-093, REQ-055): the read of the
// unknown EHR maps to transport.ErrNotFound; each refused query is a
// *query.AQLError over a *transport.WireError that keeps the status, the
// envelope message and the raw bytes; and a 400 is never a capability gap,
// which the SDK reads from a 501 alone. Whether a refusal is a path
// resolution failure is not checked: the SDK's classification does not
// recognise this deployment's unknown-attribute message (PROBE-021 is open
// on that).
func replayErrorEnvelopes(ctx context.Context, c *transport.Client, har probe.HAR) error {
	_, _, err := ehr.Get(ctx, c, ehr.EHRID(cassetteUnknownEHRID))
	if err == nil {
		return errors.New("unknown EHR: answered, want transport.ErrNotFound")
	}
	if !errors.Is(err, transport.ErrNotFound) {
		return fmt.Errorf("unknown EHR: want transport.ErrNotFound: %w", err)
	}
	var get *probe.HAREntry
	for i := range har.Log.Entries {
		if har.Log.Entries[i].Request.Method == "GET" {
			get = &har.Log.Entries[i]
			break
		}
	}
	we, ok := errors.AsType[*transport.WireError](err)
	if !ok || we == nil || we.OpenEHR == nil || get == nil || we.OpenEHR.Message != recordedMessage(get) {
		return fmt.Errorf("unknown EHR envelope message: want the recorded message: %w", err)
	}

	queries := recordedEntries(&har, "POST", "/query/aql")
	for i, q := range []string{cassetteMalformedAQL, cassetteUnknownAttributeAQL, cassetteUnsupportedPathAQL} {
		if i >= len(queries) {
			return fmt.Errorf("query %d: the recording holds no exchange for %q", i, q)
		}
		rec := queries[i]
		_, _, err := query.ExecuteString(ctx, c, q, nil)
		if err == nil {
			return fmt.Errorf("query %d: answered, want a *query.AQLError", i)
		}
		if _, ok := errors.AsType[*query.AQLError](err); !ok {
			return fmt.Errorf("query %d: want a *query.AQLError: %w", i, err)
		}
		if errors.Is(err, aql.ErrEngineCapability) {
			return fmt.Errorf("query %d: a %d refusal reads as a capability gap", i, rec.Response.Status)
		}
		we, ok := errors.AsType[*transport.WireError](err)
		if !ok || we == nil || we.StatusCode != 400 {
			return fmt.Errorf("query %d status: want a 400 wire error: %w", i, err)
		}
		if we.OpenEHR == nil || we.OpenEHR.Message == "" || we.OpenEHR.Message != recordedMessage(rec) {
			return fmt.Errorf("query %d envelope message: got %+v, want the recorded message", i, we.OpenEHR)
		}
		if !bytes.Equal(we.RawBody, []byte(rec.Response.Content.Text)) {
			return fmt.Errorf("query %d raw body: got %q, want the recorded body", i, we.RawBody)
		}
	}
	return nil
}

// errorEnvelopesRecordings are the error-envelopes captures, one per
// deployment.
var errorEnvelopesRecordings = []string{
	"error-envelopes.har",
	"ferroehr/error-envelopes.har",
}

// TestCassette_ReplaysErrorEnvelopes_PROBE068_PROBE021_REQ093_REQ055_REQ082
// replays each error-envelopes recording (PROBE-068, PROBE-021, REQ-082,
// REQ-093, REQ-055): the read of an unknown EHR, then three refused
// queries, checking what the SDK makes of each real refusal. An extra HEAD
// is unmatched.
func TestCassette_ReplaysErrorEnvelopes_PROBE068_PROBE021_REQ093_REQ055_REQ082(t *testing.T) {
	t.Parallel()
	for _, name := range errorEnvelopesRecordings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			har := loadRecording(t, name)
			c := rawReplayClientFor(t, har)
			if err := replayErrorEnvelopes(t.Context(), c, har); err != nil {
				t.Fatal(err)
			}
			_, err := ehr.Exists(t.Context(), c, ehr.EHRID(cassetteUnknownEHRID))
			wantUnmatched(t, err)
		})
	}
}

// TestCassette_ErrorEnvelopesWithholdBodiesByDefault_REQ093: a client built
// without raw error bodies keeps neither the envelope message nor the raw
// bytes of a real refusal, and its error text names neither, since an error
// body can carry patient data (REQ-093).
func TestCassette_ErrorEnvelopesWithholdBodiesByDefault_REQ093(t *testing.T) {
	t.Parallel()
	for _, name := range errorEnvelopesRecordings {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			har := loadRecording(t, name)
			c := replayClientFor(t, har)
			_, _, err := ehr.Get(t.Context(), c, ehr.EHRID(cassetteUnknownEHRID))
			if !errors.Is(err, transport.ErrNotFound) {
				t.Fatalf("ehr.Get(unknown) = %v, want transport.ErrNotFound", err)
			}
			queries := recordedQueries(t, &har)
			_, _, err = query.ExecuteString(t.Context(), c, cassetteMalformedAQL, nil)
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("query.ExecuteString(malformed) = %v, want a *transport.WireError inside", err)
			}
			if len(we.RawBody) != 0 {
				t.Errorf("default client kept the raw body %q, want none", we.RawBody)
			}
			if we.OpenEHR != nil && we.OpenEHR.Message != "" {
				t.Errorf("default client kept the envelope message %q, want none", we.OpenEHR.Message)
			}
			if msg := recordedMessage(queries[0]); msg == "" || strings.Contains(err.Error(), msg) {
				t.Errorf("error text %q names the recorded message %q, or the recording holds none", err, msg)
			}
		})
	}
}
