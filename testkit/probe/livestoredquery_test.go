package probe_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"uuid"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/testkit/probe"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// liveStoredQueryAQL is the per-run stored query. It selects one EHR by a
// bound parameter, so executing it scoped to the per-run EHR returns exactly
// that EHR — a self-scoped read (REQ-082) that depends on no other run's data.
// EHRbase does not restrict a top-level `FROM EHR e` by the openehr-ehr-id
// scope header, so the WHERE clause, not the header, is what pins the result to
// this run's EHR.
const liveStoredQueryAQL = "SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = $target_ehr"

// storedQuery carries the recovered query name from the store probe to the
// execute probe that runs it. probe.Run executes entries in order, so the
// closure hand-off is safe.
type storedQuery struct {
	name string
}

// storedQueryLiveEntries is the stored-AQL write/read path against a live
// deployment: register a per-run stored query, then execute it scoped to the
// per-run EHR. The qualified name embeds the run identifier, so the store is
// self-scoping (REQ-082 Live) — it collides with no other run and depends on no
// pre-seeded catalog query. It witnesses two catalog probes end to end:
//
//   - PROBE-079 (REQ-057): PutStoredQuery returns decoded {name, version}
//     metadata for the body-less 200 store reply.
//   - PROBE-066 (REQ-057): the stored execution returns a typed ResultSet with
//     Columns and Rows populated; the bound parameter round-trips this run's EHR.
func storedQueryLiveEntries(id openehrclient.EHRID, qualifiedName string, out *storedQuery) []probe.Entry {
	live := []probe.Mode{probe.ModeLive}
	return []probe.Entry{
		{
			ID:     "LIVE-STORED-QUERY-PUT",
			Effect: probe.EffectMutating,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				meta, _, err := definition.PutStoredQuery(ctx, c, qualifiedName, liveStoredQueryAQL)
				if err != nil {
					return liveFail(err), nil
				}
				if meta == nil {
					return liveFailf("store returned nil metadata"), nil
				}
				// PROBE-079: PutStoredQuery's recovery order — Location header
				// first, then the JSON body, then the caller's input — is REQ-057's
				// and is unit-covered in
				// openehr/client/definition/stored_query_test.go. What this entry
				// checks is the decoded StoredQueryMetadata: the name round-trips
				// and a version is present. An unversioned PUT sends version "", so
				// a non-empty version is evidence the server supplied one.
				if meta.Name != qualifiedName {
					return liveFailf("store returned metadata name %q, want %q", meta.Name, qualifiedName), nil
				}
				if meta.Version == "" {
					return liveFailf("store returned no version in its metadata"), nil
				}
				out.name = meta.Name
				return probe.Result{Status: probe.StatusPass, Detail: meta.Name + "/" + meta.Version}, nil
			},
		},
		{
			ID:     "LIVE-STORED-QUERY-RUN",
			Effect: probe.EffectReadOnly,
			Modes:  live,
			Run: func(ctx context.Context, c *transport.Client) (probe.Result, error) {
				if out.name == "" {
					return probe.Result{Status: probe.StatusSkip, Detail: "no stored query was registered to execute"}, nil
				}
				// A fetch bound is required: EHRbase rejects a stored execution
				// that sends offset without fetch (422). offset defaults to 0.
				rs, _, err := query.RunStored(ctx, c, out.name, map[string]any{"target_ehr": string(id)}, query.WithFetch(100))
				if err != nil {
					return liveFail(err), nil
				}
				if rs == nil {
					return liveFailf("execution returned a nil result set"), nil
				}
				if len(rs.Columns) == 0 {
					return liveFailf("result set carried no columns"), nil
				}
				if len(rs.Rows) == 0 || len(rs.Rows[0]) == 0 {
					return liveFailf("result set carried no rows for the per-run EHR %s", id), nil
				}
				// Self-scoped: the bound parameter filters to this run's EHR, so
				// exactly that id comes back — proof the stored query executed
				// against real data, not merely that a ResultSet decoded.
				got, ok := rs.Rows[0][0].(string)
				if !ok {
					return liveFailf("execution returned ehr_id cell of type %T, want a string carrying the per-run id %q", rs.Rows[0][0], id), nil
				}
				if got != string(id) {
					return liveFailf("execution returned ehr_id %q, want the per-run id %q", got, id), nil
				}
				return probe.Result{Status: probe.StatusPass, Detail: fmt.Sprintf("%d row(s), %d column(s)", len(rs.Rows), len(rs.Columns))}, nil
			},
		},
	}
}

// TestLiveStoredQuerySnapshot runs the stored-AQL write/read path against a live
// deployment (REQ-082 Live): create a per-run EHR, register a per-run stored
// query, then execute it scoped to that EHR and read the row back. It promotes
// PROBE-079 (stored-query metadata recovery) out of Deferred and implements
// PROBE-066 (stored execution → typed ResultSet), each witnessed against a real
// CDR rather than a hand-written fake. Opt-in and skipped in CI, sharing the
// runLiveSnapshot harness with TestLiveCoreSnapshot and
// TestLiveCompositionSnapshot.
//
// The qualified query name embeds the run identifier so the mutating store is
// self-scoping (REQ-082 Live): no collision with another run, no dependency on a
// pre-seeded catalog query.
func TestLiveStoredQuerySnapshot(t *testing.T) {
	id := openehrclient.EHRID(uuid.NewV4().String())
	// Per-run qualified query name: {namespace}::{query-name} with the run
	// identifier in the name part, so the store is self-scoping (REQ-082 Live).
	qualifiedName := "org.cadasto.sdk::snapshot_q_" + strings.ReplaceAll(string(id), "-", "")
	t.Logf("per-run EHR id %s, stored query %s", id, qualifiedName)

	out := &storedQuery{}
	entries := append([]probe.Entry{createEHRProbe(id)}, storedQueryLiveEntries(id, qualifiedName, out)...)
	runLiveSnapshot(t, "stored-query", entries)
}
