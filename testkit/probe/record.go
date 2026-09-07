package probe

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Recorder is a capture-time http.RoundTripper. It forwards every
// request to next, then appends a redacted HAR exchange. Credentials
// stay on the wire and are stripped from the recording (REQ-082).
type Recorder struct {
	next       http.RoundTripper
	provenance HARProvenance
	mu         sync.Mutex
	entries    []HAREntry
}

// NewRecorder returns a Recorder that writes exchanges through next.
// next is required — the recorder never allocates a transport
// (REQ-021). provenance is copied onto the finished HAR as
// log._req082; incomplete provenance is still written so ValidateHAR
// can refuse the file rather than the recorder inventing one.
func NewRecorder(next http.RoundTripper, provenance HARProvenance) *Recorder {
	return &Recorder{next: next, provenance: provenance}
}

// HAR returns a HAR 1.2 document of the exchanges captured so far,
// with capture-time redaction attested on log._req082.
func (r *Recorder) HAR() HAR {
	r.mu.Lock()
	defer r.mu.Unlock()
	return HAR{Log: HARLog{
		Version: "1.2",
		Creator: &HARCreator{Name: "openehr-sdk-go cassette recorder"},
		Req082: &HARReq082{
			Provenance: r.provenance,
			Redaction: HARRedaction{
				Ran:             true,
				HeadersStripped: redactedHeaderNames(),
			},
		},
		Entries: append([]HAREntry(nil), r.entries...),
	}}
}

// RoundTrip forwards req to next and records the redacted exchange.
func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("probe: nil request")
	}
	if r.next == nil {
		return nil, fmt.Errorf("probe: recorder has no next transport")
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	var reqBody []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("probe: read request body: %w", err)
		}
		reqBody = b
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	resp, err := r.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	var respBody []byte
	if resp.Body != nil {
		b, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("probe: read response body: %w", err)
		}
		respBody = b
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	}

	entry := HAREntry{
		StartedDateTime: time.Now().UTC().Format(time.RFC3339),
		Request: HARRequest{
			Method:   req.Method,
			URL:      redactURL(requestURL(req)),
			Headers:  redactHeaders(req.Header),
			BodySize: len(reqBody),
		},
		Response: HARResponse{
			Status:   resp.StatusCode,
			Headers:  redactHeaders(resp.Header),
			Content:  HARContent{Size: len(respBody), Text: string(respBody)},
			BodySize: len(respBody),
		},
	}
	if len(reqBody) > 0 {
		entry.Request.PostData = &HARPost{Text: string(reqBody)}
	}

	r.mu.Lock()
	r.entries = append(r.entries, entry)
	r.mu.Unlock()
	return resp, nil
}

func requestURL(req *http.Request) string {
	if req.URL == nil {
		return ""
	}
	return req.URL.String()
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery == "" {
		return raw
	}
	q := u.Query()
	for key := range q {
		if _, bad := credentialQueryKeys[strings.ToLower(key)]; bad {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func redactHeaders(h http.Header) []HARHeader {
	if h == nil {
		return nil
	}
	out := make([]HARHeader, 0, len(h))
	for name, values := range h {
		if _, bad := credentialHeaders[strings.ToLower(name)]; bad {
			continue
		}
		for _, v := range values {
			out = append(out, HARHeader{Name: name, Value: v})
		}
	}
	slices.SortFunc(out, func(a, b HARHeader) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

func redactedHeaderNames() []string {
	return slices.Sorted(maps.Keys(credentialHeaders))
}
