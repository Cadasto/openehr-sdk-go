package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// splitValidateRequestPath and splitSegmentCount are the strings.Split forms
// ValidateRequestPath and segmentCount had before they moved to
// strings.SplitSeq and strings.Count; the equivalence test holds the new
// forms to them.
func splitValidateRequestPath(path string) error {
	if path == "/" {
		return nil
	}
	n := 0
	for i, seg := range strings.Split(path, "/") {
		if i == 0 && seg == "" && strings.HasPrefix(path, "/") {
			continue
		}
		n++
		if fault := segmentFault(seg); fault != "" {
			return fmt.Errorf("%w: %w: segment %d: %s", ErrInvalidConfig, ErrInvalidPathSegment, n, fault)
		}
	}
	return nil
}

func splitSegmentCount(p string) int {
	if p == "/" {
		return 0
	}
	n := len(strings.Split(p, "/"))
	if strings.HasPrefix(p, "/") {
		n--
	}
	return n
}

// TestREQ150_PathChecksMatchTheirSplitForms pins that counting and ranging
// the segments without building a slice answers exactly as splitting did,
// over the edge cases of an absolute and a relative path (REQ-150).
func TestREQ150_PathChecksMatchTheirSplitForms(t *testing.T) {
	t.Parallel()
	for _, p := range []string{
		"", "/", "//", "///", "a", "/a", "a/", "/a/", "a/b", "/a/b", "/a//b", "a//b",
		"/ehr/{ehr_id}", "ehr/{ehr_id}/", "/./x", "/..", "..", "/a\\b", "/a\x01", "/ehr/Referral Request.v1",
	} {
		if got, want := segmentCount(p), splitSegmentCount(p); got != want {
			t.Errorf("segmentCount(%q) = %d, the Split form gives %d", p, got, want)
		}
		got, want := ValidateRequestPath(p), splitValidateRequestPath(p)
		if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
			t.Errorf("ValidateRequestPath(%q) = %v, the Split form gives %v", p, got, want)
		}
	}
}

// TestREQ093_BodySizeHint pins how much of a declared Content-Length is
// allocated before the body arrives: the declared length, never past the
// read limit, never past 4 MiB, and nothing for an unknown length or a HEAD,
// whose Content-Length describes a body it does not carry (REQ-093).
func TestREQ093_BodySizeHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		method        string
		contentLength int64
		limit         int64
		bounded       bool
		want          int64
	}{
		{name: "unknown length", method: http.MethodGet, contentLength: -1, limit: DefaultMaxResponseBody, bounded: true, want: -1},
		{name: "HEAD", method: http.MethodHead, contentLength: 3 << 20, limit: DefaultMaxResponseBody, bounded: true, want: -1},
		{name: "empty body", method: http.MethodGet, contentLength: 0, limit: DefaultMaxResponseBody, bounded: true, want: 0},
		{name: "declared length", method: http.MethodGet, contentLength: 2048, limit: DefaultMaxResponseBody, bounded: true, want: 2048},
		{name: "capped at the limit", method: http.MethodGet, contentLength: 2048, limit: 1024, bounded: true, want: 1024},
		{name: "capped at 4 MiB", method: http.MethodGet, contentLength: 1 << 40, limit: DefaultMaxResponseBody, bounded: true, want: maxBodyPrealloc},
		{name: "unbounded capped at 4 MiB", method: http.MethodGet, contentLength: 1 << 40, limit: -1, bounded: false, want: maxBodyPrealloc},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bodySizeHint(tc.method, tc.contentLength, tc.limit, tc.bounded); got != tc.want {
				t.Errorf("bodySizeHint(%s, %d, %d, %v) = %d, want %d", tc.method, tc.contentLength, tc.limit, tc.bounded, got, tc.want)
			}
		})
	}
}

// lyingRoundTripper answers with a body shorter or longer than the
// Content-Length it declares.
type lyingRoundTripper struct {
	contentLength int64
	body          string
}

func (l lyingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{},
		Body:          io.NopCloser(strings.NewReader(l.body)),
		ContentLength: l.contentLength,
		Request:       r,
	}, nil
}

// TestREQ093_DeclaredLengthDoesNotChangeTheRead pins that a body sized from
// its Content-Length reads exactly the bytes that arrive, whatever length
// was declared, and keeps the read ceiling: a body longer than the limit is
// refused even when its declared length is within it (REQ-093).
func TestREQ093_DeclaredLengthDoesNotChangeTheRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		contentLength int64
		body          string
		limit         int64
		wantBody      string
		wantTooLarge  bool
	}{
		{name: "declared far longer than sent", contentLength: 1 << 40, body: "hello", limit: 0, wantBody: "hello"},
		{name: "declared shorter than sent", contentLength: 2, body: "hello", limit: 0, wantBody: "hello"},
		{name: "exactly the limit", contentLength: 5, body: "hello", limit: 5, wantBody: "hello"},
		{name: "one byte over the limit, declared within it", contentLength: 5, body: "hello!", limit: 5, wantTooLarge: true},
		{name: "unbounded", contentLength: 5, body: "hello", limit: -1, wantBody: "hello"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := clientForBase(t, discovery.MustParseURL("https://cdr.example/openehr/v1"),
				&http.Client{Transport: lyingRoundTripper{contentLength: tc.contentLength, body: tc.body}},
				WithMaxResponseBody(tc.limit))
			resp, err := c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/x", Route: "/x"})
			if tc.wantTooLarge {
				if !errors.Is(err, ErrResponseTooLarge) {
					t.Fatalf("Do error = %v, want ErrResponseTooLarge", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Do error = %v, want nil", err)
			}
			if string(resp.Body) != tc.wantBody {
				t.Errorf("Body = %q, want %q", resp.Body, tc.wantBody)
			}
		})
	}
}

// TestREQ059_HeaderNamesOnTheWire reads the raw bytes the transport sends
// and answers with lower-case header names, so it pins the names on the
// wire in both directions: the request carries the canonical spelling
// net/http has always sent, and the response's openEHR headers are read
// whatever their case (REQ-059, REQ-051).
func TestREQ059_HeaderNamesOnTheWire(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	var (
		wg  sync.WaitGroup
		raw string
	)
	wg.Go(func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var head strings.Builder
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			head.WriteString(line)
			if err != nil || line == "\r\n" {
				break
			}
		}
		raw = head.String()
		// Read the two-byte body too: closing with unread bytes would reset
		// the connection before the client reads the response.
		_, _ = io.ReadFull(br, make([]byte, 2))
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\n"+
			"content-length: 2\r\n"+
			"connection: close\r\n"+
			"etag: \"e1\"\r\n"+
			"openehr-version: lifecycle_state.code_string=\"532\"\r\n"+
			"openehr-audit-details: committer.name=\"a\"\r\n"+
			"openehr-uri: ehr:/x\r\n"+
			"openehr-item-tag: k=v\r\n"+
			"openehr-version-item-tag: k2=v2\r\n"+
			"openehr-template-id: t.v1\r\n"+
			"cadasto-openehr-spec-version: 1.1.0\r\n"+
			"\r\n{}")
	})
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	c := clientForBase(t, discovery.MustParseURL("http://"+ln.Addr().String()+"/openehr/v1"), &http.Client{},
		WithSpecVersion("1.1.0"), WithCadastoSpecVersionHeader(true))
	resp, err := c.Do(t.Context(), &Request{
		Method:             http.MethodPut,
		Path:               "/x",
		Route:              "/x",
		Body:               []byte(`{}`),
		IfMatch:            "v1",
		Prefer:             PreferMinimal,
		AuditDetailsHeader: `committer.name="a"`,
		RMVersion:          `lifecycle_state.code_string="532"`,
		TemplateID:         "t.v1",
		URI:                "ehr:/x",
		ItemTag:            "k=v",
		VersionItemTag:     "k2=v2",
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	wg.Wait()

	for _, line := range []string{
		"Accept: application/json\r\n",
		"Content-Type: application/json\r\n",
		"If-Match: \"v1\"\r\n",
		"Prefer: return=minimal\r\n",
		"Openehr-Audit-Details: committer.name=\"a\"\r\n",
		"Openehr-Version: lifecycle_state.code_string=\"532\"\r\n",
		"Openehr-Template-Id: t.v1\r\n",
		"Openehr-Uri: ehr:/x\r\n",
		"Openehr-Item-Tag: k=v\r\n",
		"Openehr-Version-Item-Tag: k2=v2\r\n",
		"Cadasto-Openehr-Spec-Version: 1.1.0\r\n",
	} {
		if !strings.Contains(raw, line) {
			t.Errorf("request head = %q, want the line %q", raw, line)
		}
	}
	want := Metadata{
		ETag:               "e1",
		ETags:              []string{"e1"},
		RMVersion:          `lifecycle_state.code_string="532"`,
		AuditDetails:       `committer.name="a"`,
		URI:                "ehr:/x",
		ItemTag:            "k=v",
		VersionItemTag:     "k2=v2",
		TemplateID:         "t.v1",
		CadastoSpecVersion: "1.1.0",
	}
	got := *resp.Metadata
	if got.ETag != want.ETag || len(got.ETags) != 1 || got.ETags[0] != "e1" ||
		got.RMVersion != want.RMVersion || got.AuditDetails != want.AuditDetails || got.URI != want.URI ||
		got.ItemTag != want.ItemTag || got.VersionItemTag != want.VersionItemTag ||
		got.TemplateID != want.TemplateID || got.CadastoSpecVersion != want.CadastoSpecVersion {
		t.Errorf("Metadata = %+v, want %+v", got, want)
	}
}
