package sandbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// reply is one exchange served by a Backend, with the body already
// read and the response closed.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// roundTrip drives one request through b and returns the served reply.
func roundTrip(t *testing.T, b *sandbox.Backend, method, url string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext(%s, %q): %v", method, url, err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip(%s %s): %v", method, url, err)
	}
	if resp == nil {
		t.Fatalf("RoundTrip(%s %s) returned a nil response and a nil error", method, url)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("close response body for %s %s: %v", method, url, cerr)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body for %s %s: %v", method, url, err)
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: body}
}

// ehrIDOf reads the ehr_id.value out of an EHR response body.
func ehrIDOf(t *testing.T, body []byte) string {
	t.Helper()
	id, err := ehrID(body)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// ehrID is the error-returning half of ehrIDOf, so the concurrency
// test can report a decode failure with t.Error from a goroutine.
func ehrID(body []byte) (string, error) {
	var decoded struct {
		EHRID struct {
			Value string `json:"value"`
		} `json:"ehr_id"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("json.Unmarshal(%q): %w", body, err)
	}
	if decoded.EHRID.Value == "" {
		return "", fmt.Errorf("body %q carries no ehr_id.value", body)
	}
	return decoded.EHRID.Value, nil
}

// TestRoundTripHonoursContext pins that RoundTrip refuses a request
// whose context is already done. http.RoundTripper implementors must
// honour cancellation; without the guard a probe that cancels before
// dispatch saw a cheerful 201 from the sandbox and a cancellation
// error from a real CDR.
func TestRoundTripHonoursContext(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cancel  bool
		wantErr error
	}{
		{name: "cancelled before dispatch", cancel: true, wantErr: context.Canceled},
		{name: "live context still serves", cancel: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := sandbox.New()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := b.RoundTrip(req)
			if resp != nil {
				defer func() { _ = resp.Body.Close() }()
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("RoundTrip(POST /ehr, live context) error = %v, want nil", err)
				}
				if resp.StatusCode != http.StatusCreated {
					t.Fatalf("RoundTrip(POST /ehr, live context) status = %d, want %d", resp.StatusCode, http.StatusCreated)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("RoundTrip(POST /ehr, cancelled context) error = %v, want one satisfying errors.Is(err, %v)", err, tc.wantErr)
			}
			if resp != nil {
				t.Errorf("RoundTrip(POST /ehr, cancelled context) response = %d, want no response alongside the error", resp.StatusCode)
			}
		})
	}
}

// TestBasePathPrefixes pins that any deployment base reaches the
// built-in routes, not just the four prefixes the sandbox used to
// hard-code.
func TestBasePathPrefixes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		url  string
	}{
		{"canonical base", "https://sandbox.local/openehr/v1/ehr"},
		{"unknown api prefix", "https://sandbox.local/api/openehr/v1/ehr"},
		{"ehrbase base", "https://sandbox.local/ehrbase/rest/openehr/v1/ehr"},
		{"no base at all", "https://sandbox.local/ehr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := sandbox.New()
			got := roundTrip(t, b, http.MethodPost, tc.url)
			if got.status != http.StatusCreated {
				t.Errorf("RoundTrip(POST %s) status = %d, want %d", tc.url, got.status, http.StatusCreated)
			}
		})
	}
}

// TestBuiltinResponseShape pins the fields a built-in response carries:
// the status line net/http documents ("201 Created", not "Created")
// and a non-nil Request, so a consumer cannot tell a built-in route
// from a scripted one by inspecting the *http.Response.
func TestBuiltinResponseShape(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip(POST /ehr): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if want := "201 Created"; resp.Status != want {
		t.Errorf("RoundTrip(POST /ehr).Status = %q, want %q", resp.Status, want)
	}
	if resp.Request != req {
		t.Errorf("RoundTrip(POST /ehr).Request = %p, want the request that was served (%p)", resp.Request, req)
	}
}

// TestEHRResponseHeaders pins the sandbox's headers against ITS-REST
// (resources/its-rest/ehr-validation.openapi.yaml): 201_EHR defines
// ETag (the ehr_id in double quotes) and Location (format: url, so
// absolute); 200_EHR defines neither. Emitting ETag on the GET made
// transport.Metadata.ETag non-empty in Sandbox mode and empty against
// a conformant CDR.
func TestEHRResponseHeaders(t *testing.T) {
	t.Parallel()
	const base = "https://sandbox.local/openehr/v1"
	b := sandbox.New()

	created := roundTrip(t, b, http.MethodPost, base+"/ehr")
	if created.status != http.StatusCreated {
		t.Fatalf("RoundTrip(POST /ehr) status = %d, want %d", created.status, http.StatusCreated)
	}
	id := ehrIDOf(t, created.body)

	if got, want := created.header.Get("ETag"), `"`+id+`"`; got != want {
		t.Errorf("RoundTrip(POST /ehr) ETag = %q, want %q (ITS-REST ETag_EHR: the ehr_id in double quotes)", got, want)
	}
	loc := created.header.Get("Location")
	if !strings.HasPrefix(loc, "https://sandbox.local") {
		t.Errorf("RoundTrip(POST /ehr) Location = %q, want an absolute URL starting with %q (ITS-REST Location_EHR is format: url)", loc, "https://sandbox.local")
	}
	if !strings.HasSuffix(loc, "/ehr/"+id) {
		t.Errorf("RoundTrip(POST /ehr) Location = %q, want it to end with %q", loc, "/ehr/"+id)
	}

	fetched := roundTrip(t, b, http.MethodGet, base+"/ehr/"+id)
	if fetched.status != http.StatusOK {
		t.Fatalf("RoundTrip(GET /ehr/%s) status = %d, want %d", id, fetched.status, http.StatusOK)
	}
	for _, h := range []string{"ETag", "Location"} {
		if v := fetched.header.Get(h); v != "" {
			t.Errorf("RoundTrip(GET /ehr/%s) %s = %q, want it absent — ITS-REST 200_EHR defines only Content-Type", id, h, v)
		}
	}
}

// TestPutDuplicateEHRConflicts pins the 409 arm: PUT /ehr/{id} creates
// an EHR with a caller-chosen id, and a second PUT of the same id is a
// conflict, not a silent overwrite (ITS-REST 409_EHR_with_id).
func TestPutDuplicateEHRConflicts(t *testing.T) {
	t.Parallel()
	const url = "https://sandbox.local/openehr/v1/ehr/fixed-ehr-id"
	b := sandbox.New()

	first := roundTrip(t, b, http.MethodPut, url)
	if first.status != http.StatusCreated {
		t.Fatalf("RoundTrip(PUT /ehr/fixed-ehr-id) first call status = %d, want %d", first.status, http.StatusCreated)
	}
	second := roundTrip(t, b, http.MethodPut, url)
	if second.status != http.StatusConflict {
		t.Fatalf("RoundTrip(PUT /ehr/fixed-ehr-id) second call status = %d, want %d", second.status, http.StatusConflict)
	}
}

// TestDuplicateCreateDoesNotCorruptStoredBody pins that a rejected
// duplicate create leaves the stored EHR untouched. createEHR returns
// 409 before it writes the store, so the first body must survive
// byte-for-byte: a GET after the conflict returns exactly what the
// original create stored, not anything the duplicate attempt built
// (which carries fresh, random status and access ids).
func TestDuplicateCreateDoesNotCorruptStoredBody(t *testing.T) {
	t.Parallel()
	const url = "https://sandbox.local/openehr/v1/ehr/fixed-ehr-id"
	b := sandbox.New()

	created := roundTrip(t, b, http.MethodPut, url)
	if created.status != http.StatusCreated {
		t.Fatalf("RoundTrip(PUT /ehr/fixed-ehr-id) first call status = %d, want %d", created.status, http.StatusCreated)
	}
	original := created.body

	conflict := roundTrip(t, b, http.MethodPut, url)
	if conflict.status != http.StatusConflict {
		t.Fatalf("RoundTrip(PUT /ehr/fixed-ehr-id) duplicate call status = %d, want %d", conflict.status, http.StatusConflict)
	}

	fetched := roundTrip(t, b, http.MethodGet, url)
	if fetched.status != http.StatusOK {
		t.Fatalf("RoundTrip(GET /ehr/fixed-ehr-id) status = %d, want %d", fetched.status, http.StatusOK)
	}
	if string(fetched.body) != string(original) {
		t.Errorf("RoundTrip(GET /ehr/fixed-ehr-id) body = %q, want it byte-for-byte identical to the originally created body %q — the 409 must not overwrite the store", fetched.body, original)
	}
}

// TestUnroutedRequest404 pins the bare unmatched-route 404 — a
// resource the sandbox does not serve at all — as distinct from the
// looked-up-and-absent EHR 404. The two carry different bodies, so a
// probe can tell "this sandbox has no composition surface" from "that
// EHR does not exist".
func TestUnroutedRequest404(t *testing.T) {
	t.Parallel()
	b := sandbox.New()

	unrouted := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/composition/x")
	if unrouted.status != http.StatusNotFound {
		t.Fatalf("RoundTrip(GET /composition/x) status = %d, want %d", unrouted.status, http.StatusNotFound)
	}
	absent := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/ehr/does-not-exist")
	if absent.status != http.StatusNotFound {
		t.Fatalf("RoundTrip(GET /ehr/does-not-exist) status = %d, want %d", absent.status, http.StatusNotFound)
	}
	unroutedBody, absentBody := unrouted.body, absent.body

	if string(unroutedBody) == string(absentBody) {
		t.Errorf("RoundTrip(GET /composition/x) body = %q and RoundTrip(GET /ehr/does-not-exist) body = %q are identical, want the unrouted 404 distinguishable from the absent-EHR 404", unroutedBody, absentBody)
	}
	if want := "not found"; !strings.Contains(string(unroutedBody), want) {
		t.Errorf("RoundTrip(GET /composition/x) body = %q, want it to contain %q", unroutedBody, want)
	}
	if want := "ehr not found"; !strings.Contains(string(absentBody), want) {
		t.Errorf("RoundTrip(GET /ehr/does-not-exist) body = %q, want it to contain %q", absentBody, want)
	}
}

// TestSameInstanceEHRsAreDistinct pins the other half of REQ-082's
// isolation rule. TestIsolation covers two Backends; this covers one:
// two EHRs created on the same instance keep their own bodies, and an
// id nobody created is absent. Without it a Backend that served the
// last-written body for every id would still pass the cross-instance
// test.
func TestSameInstanceEHRsAreDistinct(t *testing.T) {
	t.Parallel()
	const base = "https://sandbox.local/openehr/v1"
	b := sandbox.New()

	firstID := ehrIDOf(t, roundTrip(t, b, http.MethodPost, base+"/ehr").body)
	secondID := ehrIDOf(t, roundTrip(t, b, http.MethodPost, base+"/ehr").body)
	if firstID == secondID {
		t.Fatalf("two POST /ehr calls on one Backend returned the same ehr_id %q, want distinct ids", firstID)
	}

	for _, id := range []string{firstID, secondID} {
		resp := roundTrip(t, b, http.MethodGet, base+"/ehr/"+id)
		if resp.status != http.StatusOK {
			t.Fatalf("RoundTrip(GET /ehr/%s) status = %d, want %d", id, resp.status, http.StatusOK)
		}
		if got := ehrIDOf(t, resp.body); got != id {
			t.Errorf("RoundTrip(GET /ehr/%s) body ehr_id = %q, want %q", id, got, id)
		}
	}

	absent := roundTrip(t, b, http.MethodGet, base+"/ehr/00000000-0000-4000-8000-000000000000")
	if absent.status != http.StatusNotFound {
		t.Errorf("RoundTrip(GET /ehr/00000000-0000-4000-8000-000000000000) status = %d, want %d — an id nobody created must be absent", absent.status, http.StatusNotFound)
	}
}

// TestConcurrentCreateEHR is the race pin for the "safe for concurrent
// use (REQ-026)" claim on [sandbox.Backend]: many goroutines POST /ehr
// against one shared instance, every call must be created, and every
// returned ehr_id must be distinct. CI runs this package under -race.
func TestConcurrentCreateEHR(t *testing.T) {
	t.Parallel()
	const goroutines = 32
	b := sandbox.New()

	var (
		mu  sync.Mutex
		ids = make(map[string]int, goroutines)
		wg  sync.WaitGroup
	)
	for i := range goroutines {
		wg.Go(func() {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
			if err != nil {
				t.Errorf("goroutine %d: http.NewRequestWithContext: %v", i, err)
				return
			}
			resp, err := b.RoundTrip(req)
			if err != nil {
				t.Errorf("goroutine %d: RoundTrip(POST /ehr): %v", i, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusCreated {
				t.Errorf("goroutine %d: RoundTrip(POST /ehr) status = %d, want %d", i, resp.StatusCode, http.StatusCreated)
				return
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("goroutine %d: read body: %v", i, err)
				return
			}
			id, err := ehrID(body)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			mu.Lock()
			ids[id]++
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(ids) != goroutines {
		t.Errorf("%d concurrent POST /ehr calls produced %d distinct ehr_ids, want %d (duplicates: %v)", goroutines, len(ids), goroutines, duplicates(ids))
	}
}

// duplicates lists the ids seen more than once, so a failure names the
// collision instead of only its count.
func duplicates(ids map[string]int) []string {
	var dup []string
	for id, n := range ids {
		if n > 1 {
			dup = append(dup, id)
		}
	}
	return dup
}

// TestConcurrentCreateSameEHRIDOneWins is the race pin for the
// check-and-insert inside createEHR (REQ-026): many goroutines PUT the
// very same EHR id at once, so every call but one must see an id
// already taken and answer 409, and exactly one must win the 201. A
// backend whose existence check and map write were not one atomic
// section under the mutex could let two calls both read "absent" and
// both answer 201, or lose the winner's body to a racing write.
func TestConcurrentCreateSameEHRIDOneWins(t *testing.T) {
	t.Parallel()
	const (
		goroutines = 32
		target     = "https://sandbox.local/openehr/v1/ehr/same-ehr-id"
	)
	b := sandbox.New()

	var (
		mu       sync.Mutex
		statuses []int
		wg       sync.WaitGroup
	)
	for i := range goroutines {
		wg.Go(func() {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, target, nil)
			if err != nil {
				t.Errorf("goroutine %d: http.NewRequestWithContext(PUT, %q): %v", i, target, err)
				return
			}
			resp, err := b.RoundTrip(req)
			if err != nil {
				t.Errorf("goroutine %d: RoundTrip(PUT %s): %v", i, target, err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			mu.Lock()
			statuses = append(statuses, resp.StatusCode)
			mu.Unlock()
		})
	}
	wg.Wait()

	var created, conflicted int
	var other []int
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			other = append(other, s)
		}
	}
	if len(other) > 0 {
		t.Fatalf("%d concurrent PUT %s calls produced unexpected statuses %v, want only %d and %d", goroutines, target, other, http.StatusCreated, http.StatusConflict)
	}
	if created != 1 {
		t.Fatalf("%d concurrent PUT %s calls produced %d %d responses, want exactly 1 (statuses: %v)", goroutines, target, created, http.StatusCreated, statuses)
	}
	if conflicted != goroutines-1 {
		t.Fatalf("%d concurrent PUT %s calls produced %d %d responses, want exactly %d", goroutines, target, conflicted, http.StatusConflict, goroutines-1)
	}

	resp := roundTrip(t, b, http.MethodGet, target)
	if resp.status != http.StatusOK {
		t.Fatalf("RoundTrip(GET %s) status = %d, want %d", target, resp.status, http.StatusOK)
	}
	if len(resp.body) == 0 {
		t.Fatal("RoundTrip(GET same-ehr-id) body is empty, want the winner's created EHR to have persisted")
	}
}
