package probe

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ErrUnmatchedRecording is returned by [Replayer] when no remaining
// exchange matches the request. The replayer must not dial (REQ-082).
// Removing this sentinel must fail [TestReplayer_UnmatchedFailsClosed].
var ErrUnmatchedRecording = errors.New("probe: no recording matches the request")

// Replayer is a Cassette-mode http.RoundTripper. It serves recorded
// exchanges in capture order and refuses anything the recording does
// not contain. It never dials (REQ-082 fail-closed).
type Replayer struct {
	mu        sync.Mutex
	remaining []HAREntry
}

// NewReplayer returns a Replayer over har's entries.
func NewReplayer(har HAR) *Replayer {
	return &Replayer{
		remaining: append([]HAREntry(nil), har.Log.Entries...),
	}
}

// HTTPClient returns an *http.Client whose Transport is r.
func (r *Replayer) HTTPClient() *http.Client {
	return &http.Client{Transport: r}
}

// RoundTrip serves the first remaining recorded exchange whose
// normalised method+path matches req. An unmatched request returns
// [ErrUnmatchedRecording] and does not dial.
func (r *Replayer) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("probe: nil request")
	}
	if req.Body != nil {
		defer req.Body.Close()
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	key := replayKey(req.Method, requestPath(req))
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.remaining {
		if replayKey(e.Request.Method, recordedPath(e.Request.URL)) != key {
			continue
		}
		r.remaining = append(r.remaining[:i], r.remaining[i+1:]...)
		return entryResponse(e, req), nil
	}
	return nil, fmt.Errorf("%w: %s %s", ErrUnmatchedRecording, req.Method, key)
}

func requestPath(req *http.Request) string {
	if req.URL == nil {
		return ""
	}
	return req.URL.Path
}

func recordedPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

// replayKey is the normalised request key REQ-082 matches on: method
// plus the openEHR resource path, with the REST base prefix stripped
// so a capture against EHRbase and a replay against any catalog URL
// agree.
func replayKey(method, path string) string {
	return strings.ToUpper(method) + " " + stripRESTPrefix(path)
}

func stripRESTPrefix(p string) string {
	for _, prefix := range []string{
		"/openehr/v1",
		"/ehrbase/rest/openehr/v1",
		"/ferroehr/rest/openehr/v1",
		"/rest/openehr/v1",
	} {
		if rest, ok := strings.CutPrefix(p, prefix); ok {
			if rest == "" {
				return "/"
			}
			return rest
		}
	}
	return p
}

func entryResponse(e HAREntry, req *http.Request) *http.Response {
	h := make(http.Header)
	for _, hdr := range e.Response.Headers {
		h.Add(hdr.Name, hdr.Value)
	}
	body := e.Response.Content.Text
	return &http.Response{
		StatusCode:    e.Response.Status,
		Status:        http.StatusText(e.Response.Status),
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
	}
}
