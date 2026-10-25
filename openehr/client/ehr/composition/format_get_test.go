package composition_test

// format_get_test.go — REQ-173 Reads at the composition leaf: the format,
// raw-body and format-query options of Get. The fixture, the recording
// server and the codec helpers are the ones format_write_test.go defines.

import (
	"bytes"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// getAt is the as-of time the version_at_time cases read at, and
// getAtParam the value Get sends for it.
var getAt = time.Date(2026, 5, 17, 8, 0, 0, 0, time.UTC)

const getAtParam = "2026-05-17T08:00:00Z"

// getRoute is the route template Get names its requests by.
const getRoute = "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}"

// readFormat is one of the four formats with the media type Get sends for
// it as Accept.
type readFormat struct {
	format    composition.Format
	mediaType string
}

var readFormats = []readFormat{
	{format: composition.FormatJSON, mediaType: mediaJSON},
	{format: composition.FormatXML, mediaType: mediaXML},
	{format: composition.FormatFlat, mediaType: mediaFlat},
	{format: composition.FormatStructured, mediaType: mediaStructured},
}

// templateFor returns wt for FLAT and STRUCTURED and nil for the canonical
// formats, which read no Web Template.
func templateFor(f composition.Format, wt *webtemplate.WebTemplate) *webtemplate.WebTemplate {
	if f == composition.FormatFlat || f == composition.FormatStructured {
		return wt
	}
	return nil
}

// sameQuery reports whether two query value sets hold the same keys and
// values.
func sameQuery(got, want url.Values) bool {
	return maps.EqualFunc(got, want, slices.Equal[[]string])
}

// wantVersionMeta fails the test unless meta carries the version uid the
// format server's ETag names.
func wantVersionMeta(t *testing.T, meta *openehrclient.VersionMetadata) {
	t.Helper()
	if meta == nil || meta.VersionUID != compositionVUID {
		t.Errorf("version metadata = %+v, want VersionUID %q from the ETag", meta, compositionVUID)
	}
}

// TestGetWithoutOptionsIsUnchanged pins REQ-173 Reads: with no option, or
// only nil options, Get sends Accept: application/json and no query
// parameter beyond the Ref's, and decodes canonical JSON, as it did before
// the formats existed.
func TestGetWithoutOptionsIsUnchanged(t *testing.T) {
	_, comp := loadFormatFixture(t)
	body := encodeAs(t, composition.FormatJSON, comp, nil)
	cases := []struct {
		name  string
		ref   openehrclient.Ref
		opts  []composition.GetOption
		query url.Values
	}{
		{name: "latest", ref: openehrclient.LatestOf(compositionVOID), query: url.Values{}},
		{
			name: "latest at time", ref: openehrclient.LatestAtTime(compositionVOID, getAt),
			query: url.Values{"version_at_time": {getAtParam}},
		},
		{name: "version", ref: openehrclient.VersionOf(compositionVUID), query: url.Values{}},
		{
			name: "nil option", ref: openehrclient.LatestOf(compositionVOID),
			opts: []composition.GetOption{nil}, query: url.Values{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, http.StatusOK, mediaJSON, body)
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture, tc.ref, tc.opts...)
			if err != nil {
				t.Fatalf("Get error = %v", err)
			}
			req := fs.only(t)
			if got := req.header.Get("Accept"); got != mediaJSON {
				t.Errorf("Accept = %q, want %q", got, mediaJSON)
			}
			if !sameQuery(req.query, tc.query) {
				t.Errorf("query = %v, want %v", req.query, tc.query)
			}
			sameComposition(t, "returned composition", out, comp)
			wantVersionMeta(t, meta)
		})
	}
}

// TestGetWithFormat pins REQ-173 WithGetFormat: Get sends the format's media
// type as Accept and no format parameter, and decodes a 200 body in that
// format into the returned composition: canonical JSON and XML with no Web
// Template, FLAT and STRUCTURED against the Web Template given.
func TestGetWithFormat(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	for _, f := range readFormats {
		t.Run(f.format.String(), func(t *testing.T) {
			fwt := templateFor(f.format, wt)
			body := encodeAs(t, f.format, comp, fwt)
			fs := newFormatServer(t, http.StatusOK, f.mediaType, body)
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), composition.WithGetFormat(f.format, fwt))
			if err != nil {
				t.Fatalf("Get(%v) error = %v", f.format, err)
			}
			req := fs.only(t)
			if got := req.header.Get("Accept"); got != f.mediaType {
				t.Errorf("Accept = %q, want %q", got, f.mediaType)
			}
			if len(req.query) != 0 {
				t.Errorf("query = %v, want none without WithGetFormatQuery", req.query)
			}
			if out == nil {
				t.Fatal("returned composition is nil, want the decoded body")
			}
			if got := encodeAs(t, f.format, out, fwt); !bytes.Equal(got, body) {
				t.Errorf("decoded composition re-encodes differently\n got: %s\nwant: %s", got, body)
			}
			wantVersionMeta(t, meta)
		})
	}
}

// TestGetSimplifiedWithoutWebTemplate pins REQ-173: with a nil Web Template a
// FLAT or STRUCTURED body is not decoded. Get returns a nil composition, the
// version metadata and a nil error, and WithRawBody holds the response's
// Content-Type and body. Each body served would decode in some format: the
// simplified body against the Web Template, and the canonical JSON body
// labelled application/json by the codec that Content-Type names. Only the
// format asked for and the missing Web Template keep it undecoded.
func TestGetSimplifiedWithoutWebTemplate(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	cases := []struct {
		name        string
		format      composition.Format
		contentType string
		body        []byte
	}{
		{
			name: "FLAT", format: composition.FormatFlat, contentType: mediaFlat,
			body: encodeAs(t, composition.FormatFlat, comp, wt),
		},
		{
			name: "STRUCTURED", format: composition.FormatStructured, contentType: mediaStructured,
			body: encodeAs(t, composition.FormatStructured, comp, wt),
		},
		{
			name: "FLAT answered with canonical JSON", format: composition.FormatFlat, contentType: mediaJSON,
			body: encodeAs(t, composition.FormatJSON, comp, nil),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, http.StatusOK, tc.contentType, tc.body)
			var raw composition.Raw
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID),
				composition.WithGetFormat(tc.format, nil), composition.WithRawBody(&raw))
			if err != nil {
				t.Fatalf("Get(%v, nil Web Template) error = %v, want nil", tc.format, err)
			}
			if out != nil {
				t.Errorf("returned composition = %+v, want nil with no Web Template", out)
			}
			wantVersionMeta(t, meta)
			if got := fs.only(t).header.Get("Accept"); got != tc.format.MediaType() {
				t.Errorf("Accept = %q, want %q", got, tc.format.MediaType())
			}
			if raw.ContentType != tc.contentType {
				t.Errorf("Raw.ContentType = %q, want %q", raw.ContentType, tc.contentType)
			}
			if !bytes.Equal(raw.Body, tc.body) {
				t.Errorf("Raw.Body holds %d bytes, want the %d bytes the server sent", len(raw.Body), len(tc.body))
			}
		})
	}
	t.Run("without WithRawBody", func(t *testing.T) {
		fs := newFormatServer(t, http.StatusOK, mediaFlat, encodeAs(t, composition.FormatFlat, comp, wt))
		out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
			openehrclient.LatestOf(compositionVOID), composition.WithGetFormat(composition.FormatFlat, nil))
		if err != nil || out != nil {
			t.Errorf("Get(FLAT, nil Web Template) = (%+v, %v), want (nil, nil)", out, err)
		}
		wantVersionMeta(t, meta)
	})
}

// TestGetRawBody pins REQ-173 WithRawBody on a 200 in every format, the
// default included: the destination holds the response's Content-Type value
// as received, parameters and all, and its body bytes, and the composition is
// decoded as without the option.
func TestGetRawBody(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	cases := []struct {
		name        string
		format      composition.Format
		opts        func(wt *webtemplate.WebTemplate) []composition.GetOption
		contentType string
	}{
		{
			name: "default", format: composition.FormatJSON, contentType: "application/json; charset=utf-8",
			opts: func(*webtemplate.WebTemplate) []composition.GetOption { return nil },
		},
		{
			name: "XML", format: composition.FormatXML, contentType: "application/xml; charset=UTF-8",
			opts: func(*webtemplate.WebTemplate) []composition.GetOption {
				return []composition.GetOption{composition.WithGetFormat(composition.FormatXML, nil)}
			},
		},
		{
			name: "FLAT", format: composition.FormatFlat, contentType: mediaFlat,
			opts: func(wt *webtemplate.WebTemplate) []composition.GetOption {
				return []composition.GetOption{composition.WithGetFormat(composition.FormatFlat, wt)}
			},
		},
		{
			name: "STRUCTURED", format: composition.FormatStructured, contentType: mediaStructured,
			opts: func(wt *webtemplate.WebTemplate) []composition.GetOption {
				return []composition.GetOption{composition.WithGetFormat(composition.FormatStructured, wt)}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fwt := templateFor(tc.format, wt)
			body := encodeAs(t, tc.format, comp, fwt)
			fs := newFormatServer(t, http.StatusOK, tc.contentType, body)
			var raw composition.Raw
			opts := append(tc.opts(fwt), composition.WithRawBody(&raw))
			out, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), opts...)
			if err != nil {
				t.Fatalf("Get(%s) error = %v", tc.name, err)
			}
			if raw.ContentType != tc.contentType {
				t.Errorf("Raw.ContentType = %q, want %q", raw.ContentType, tc.contentType)
			}
			if !bytes.Equal(raw.Body, body) {
				t.Errorf("Raw.Body holds %d bytes, want the %d bytes the server sent", len(raw.Body), len(body))
			}
			if out == nil {
				t.Fatal("returned composition is nil, want the decoded body")
			}
			if got := encodeAs(t, tc.format, out, fwt); !bytes.Equal(got, body) {
				t.Errorf("decoded composition re-encodes differently\n got: %s\nwant: %s", got, body)
			}
		})
	}
}

// TestGetDecodeFailureKeepsRawBody pins REQ-173 with REQ-151: a 200 body that
// does not decode in the format asked for returns the typed decode failure,
// a *transport.DecodeError behind the operation name, with the version
// metadata beside it, and WithRawBody still holds the Content-Type and bytes.
func TestGetDecodeFailureKeepsRawBody(t *testing.T) {
	wt, _ := loadFormatFixture(t)
	cases := []struct {
		format      composition.Format
		contentType string
		body        string
	}{
		{format: composition.FormatJSON, contentType: mediaJSON, body: `["not", "a", "composition"]`},
		{format: composition.FormatXML, contentType: mediaXML, body: "<composition"},
		{format: composition.FormatFlat, contentType: mediaFlat, body: `["not", "a", "flat", "object"]`},
		{format: composition.FormatStructured, contentType: mediaStructured, body: `["not", "structured"]`},
	}
	for _, tc := range cases {
		t.Run(tc.format.String(), func(t *testing.T) {
			fs := newFormatServer(t, http.StatusOK, tc.contentType, []byte(tc.body))
			var raw composition.Raw
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID),
				composition.WithGetFormat(tc.format, templateFor(tc.format, wt)), composition.WithRawBody(&raw))
			de, ok := errors.AsType[*transport.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("Get(%v) error = %v, want a *transport.DecodeError", tc.format, err)
			}
			if string(de.Body) != tc.body || de.Method != http.MethodGet || de.Route != getRoute {
				t.Errorf("DecodeError = {Method %q, Route %q, Body %q}, want {%q, %q, %q}",
					de.Method, de.Route, de.Body, http.MethodGet, getRoute, tc.body)
			}
			if op := "composition.Get:"; !strings.HasPrefix(err.Error(), op) {
				t.Errorf("error %q does not start with %q", err, op)
			}
			if errors.Is(err, transport.ErrInvalidShape) {
				t.Errorf("a body that does not decode surfaced as ErrInvalidShape: %v", err)
			}
			if out != nil {
				t.Errorf("returned composition = %+v, want nil", out)
			}
			wantVersionMeta(t, meta)
			if raw.ContentType != tc.contentType {
				t.Errorf("Raw.ContentType = %q, want %q", raw.ContentType, tc.contentType)
			}
			if string(raw.Body) != tc.body {
				t.Errorf("Raw.Body = %q, want the bytes the server sent, %q", raw.Body, tc.body)
			}
		})
	}
}

// TestGetDecoderFollowsAskedFormat pins REQ-173: the decoder is the one the
// format asked for names, never the one the response's Content-Type names.
// Each server labels its body with another format's media type, and the
// body still decodes in the format asked for.
func TestGetDecoderFollowsAskedFormat(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	cases := []struct {
		name   string
		format composition.Format
		label  string
	}{
		{name: "JSON labelled XML", format: composition.FormatJSON, label: mediaXML},
		{name: "XML labelled JSON", format: composition.FormatXML, label: mediaJSON},
		{name: "FLAT labelled JSON", format: composition.FormatFlat, label: mediaJSON},
		{name: "STRUCTURED labelled XML", format: composition.FormatStructured, label: mediaXML},
		{name: "STRUCTURED labelled FLAT", format: composition.FormatStructured, label: mediaFlat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fwt := templateFor(tc.format, wt)
			body := encodeAs(t, tc.format, comp, fwt)
			fs := newFormatServer(t, http.StatusOK, tc.label, body)
			out, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), composition.WithGetFormat(tc.format, fwt))
			if err != nil {
				t.Fatalf("Get(%v) of a body labelled %s error = %v", tc.format, tc.label, err)
			}
			if out == nil {
				t.Fatal("returned composition is nil, want the decoded body")
			}
			if got := encodeAs(t, tc.format, out, fwt); !bytes.Equal(got, body) {
				t.Errorf("decoded composition re-encodes differently\n got: %s\nwant: %s", got, body)
			}
		})
	}
}

// TestGetEmptyBodyAndDeletedInEveryFormat pins REQ-173: an empty or null 200
// body keeps transport.ErrInvalidShape, and a 204 keeps ErrDeletedAtTime,
// in every format, FLAT and STRUCTURED with a nil Web Template included.
// WithRawBody is filled on the empty 200, and left as it was on the 204.
func TestGetEmptyBodyAndDeletedInEveryFormat(t *testing.T) {
	wt, _ := loadFormatFixture(t)
	variants := []struct {
		name string
		opts []composition.GetOption
	}{
		{name: "default"},
		{name: "XML", opts: []composition.GetOption{composition.WithGetFormat(composition.FormatXML, nil)}},
		{name: "FLAT", opts: []composition.GetOption{composition.WithGetFormat(composition.FormatFlat, wt)}},
		{name: "FLAT without Web Template", opts: []composition.GetOption{composition.WithGetFormat(composition.FormatFlat, nil)}},
		{name: "STRUCTURED", opts: []composition.GetOption{composition.WithGetFormat(composition.FormatStructured, wt)}},
		{name: "STRUCTURED without Web Template", opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatStructured, nil),
		}},
		{name: "STRUCTURED by query parameter", opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatStructured, nil), composition.WithGetFormatQuery(),
		}},
	}
	for _, v := range variants {
		empties := []struct{ name, body string }{
			{name: "empty", body: ""},
			{name: "null", body: "null"},
			{name: "whitespace", body: " \n"},
		}
		for _, e := range empties {
			body := e.body
			t.Run(v.name+"/"+e.name, func(t *testing.T) {
				fs := newFormatServer(t, http.StatusOK, mediaJSON, []byte(body))
				var raw composition.Raw
				out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
					openehrclient.LatestOf(compositionVOID), append(v.opts, composition.WithRawBody(&raw))...)
				if !errors.Is(err, transport.ErrInvalidShape) {
					t.Errorf("Get(%s) of the 200 body %q error = %v, want ErrInvalidShape", v.name, body, err)
				}
				if _, ok := errors.AsType[*transport.DecodeError](err); ok {
					t.Errorf("an empty body surfaced as a *transport.DecodeError: %v", err)
				}
				if out != nil {
					t.Errorf("returned composition = %+v, want nil", out)
				}
				wantVersionMeta(t, meta)
				if raw.ContentType != mediaJSON || string(raw.Body) != body {
					t.Errorf("Raw = {%q, %q}, want {%q, %q} from the 200", raw.ContentType, raw.Body, mediaJSON, body)
				}
			})
		}
		t.Run(v.name+"/204", func(t *testing.T) {
			fs := newFormatServer(t, http.StatusNoContent, "", nil)
			raw := composition.Raw{ContentType: "seed", Body: []byte("seed")}
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestAtTime(compositionVOID, getAt), append(v.opts, composition.WithRawBody(&raw))...)
			if !errors.Is(err, composition.ErrDeletedAtTime) {
				t.Errorf("Get(%s) of a 204 error = %v, want ErrDeletedAtTime", v.name, err)
			}
			if errors.Is(err, transport.ErrInvalidShape) {
				t.Errorf("a 204 surfaced as ErrInvalidShape: %v", err)
			}
			if out != nil {
				t.Errorf("returned composition = %+v, want nil", out)
			}
			wantVersionMeta(t, meta)
			if raw.ContentType != "seed" || string(raw.Body) != "seed" {
				t.Errorf("Raw = {%q, %d bytes} after a 204, want it left as it was, {\"seed\", 4 bytes}",
					raw.ContentType, len(raw.Body))
			}
		})
	}
}

// TestGetRawBodyLeftAsItWas pins REQ-173 WithRawBody: on any outcome other
// than a 200 the destination is left as it was. Here a 404, a 204, and a
// 203 whose body Get still decodes.
func TestGetRawBodyLeftAsItWas(t *testing.T) {
	_, comp := loadFormatFixture(t)
	cases := []struct {
		name    string
		status  int
		body    []byte
		wantErr error
	}{
		{
			name: "404", status: http.StatusNotFound, wantErr: transport.ErrNotFound,
			body: []byte(`{"message":"not found","code":"NOT_FOUND"}`),
		},
		{name: "204", status: http.StatusNoContent, wantErr: composition.ErrDeletedAtTime},
		{
			name: "203", status: http.StatusNonAuthoritativeInfo,
			body: encodeAs(t, composition.FormatJSON, comp, nil),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, tc.status, mediaJSON, tc.body)
			raw := composition.Raw{ContentType: "seed", Body: []byte("seed")}
			_, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), composition.WithRawBody(&raw))
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Get error = %v, want %v", err, tc.wantErr)
			}
			if raw.ContentType != "seed" || string(raw.Body) != "seed" {
				t.Errorf("Raw = {%q, %d bytes} after a %d, want it left as it was, {\"seed\", 4 bytes}",
					raw.ContentType, len(raw.Body), tc.status)
			}
		})
	}
}

// TestGetFormatQuery pins REQ-173 WithGetFormatQuery. FLAT and STRUCTURED
// send Accept: application/json and the format parameter, beside the
// version_at_time parameter a Ref carries, and decode the body in the
// format asked for. Canonical JSON and XML carry no format parameter.
func TestGetFormatQuery(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	latest := openehrclient.LatestOf(compositionVOID)
	atTime := openehrclient.LatestAtTime(compositionVOID, getAt)
	cases := []struct {
		name       string
		format     composition.Format
		ref        openehrclient.Ref
		queryFirst bool // WithGetFormatQuery before WithGetFormat
		accept     string
		query      url.Values
	}{
		{
			name: "STRUCTURED at time", format: composition.FormatStructured, ref: atTime, accept: mediaJSON,
			query: url.Values{"format": {"STRUCTURED"}, "version_at_time": {getAtParam}},
		},
		{
			name: "FLAT latest", format: composition.FormatFlat, ref: latest, queryFirst: true, accept: mediaJSON,
			query: url.Values{"format": {"FLAT"}},
		},
		{
			name: "FLAT at time", format: composition.FormatFlat, ref: atTime, accept: mediaJSON,
			query: url.Values{"format": {"FLAT"}, "version_at_time": {getAtParam}},
		},
		{name: "JSON latest", format: composition.FormatJSON, ref: latest, accept: mediaJSON, query: url.Values{}},
		{
			name: "JSON at time", format: composition.FormatJSON, ref: atTime, queryFirst: true, accept: mediaJSON,
			query: url.Values{"version_at_time": {getAtParam}},
		},
		{name: "XML latest", format: composition.FormatXML, ref: latest, accept: mediaXML, query: url.Values{}},
		{
			name: "XML at time", format: composition.FormatXML, ref: atTime, accept: mediaXML,
			query: url.Values{"version_at_time": {getAtParam}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fwt := templateFor(tc.format, wt)
			body := encodeAs(t, tc.format, comp, fwt)
			fs := newFormatServer(t, http.StatusOK, tc.accept, body)
			opts := []composition.GetOption{composition.WithGetFormat(tc.format, fwt), composition.WithGetFormatQuery()}
			if tc.queryFirst {
				slices.Reverse(opts)
			}
			out, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture, tc.ref, opts...)
			if err != nil {
				t.Fatalf("Get(%v, WithGetFormatQuery) error = %v", tc.format, err)
			}
			req := fs.only(t)
			if got := req.header.Get("Accept"); got != tc.accept {
				t.Errorf("Accept = %q, want %q", got, tc.accept)
			}
			if !sameQuery(req.query, tc.query) {
				t.Errorf("query = %v, want %v", req.query, tc.query)
			}
			if out == nil {
				t.Fatal("returned composition is nil, want the decoded body")
			}
			if got := encodeAs(t, tc.format, out, fwt); !bytes.Equal(got, body) {
				t.Errorf("decoded composition re-encodes differently\n got: %s\nwant: %s", got, body)
			}
		})
	}
}

// TestGetOptionRefusals pins the REQ-173 refusals on Get: each fails with
// ErrInvalidConfig and issues no request. A Format outside the four, with or
// without the format query parameter, and WithRawBody(nil), wherever it
// comes in the options.
func TestGetOptionRefusals(t *testing.T) {
	wt, _ := loadFormatFixture(t)
	var raw composition.Raw
	cases := []struct {
		name string
		opts []composition.GetOption
	}{
		{name: "Format one past the four", opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatStructured+1, wt),
		}},
		{name: "negative Format", opts: []composition.GetOption{
			composition.WithGetFormat(composition.Format(-1), nil),
		}},
		{name: "Format outside the four by query parameter", opts: []composition.GetOption{
			composition.WithGetFormatQuery(), composition.WithGetFormat(composition.FormatStructured+1, wt),
		}},
		{name: "WithRawBody(nil)", opts: []composition.GetOption{composition.WithRawBody(nil)}},
		{name: "WithRawBody(nil) with XML", opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatXML, nil), composition.WithRawBody(nil),
		}},
		{name: "WithRawBody(nil) after a destination", opts: []composition.GetOption{
			composition.WithRawBody(&raw), composition.WithRawBody(nil),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, http.StatusOK, mediaJSON, nil)
			out, meta, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), tc.opts...)
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("Get error = %v, want ErrInvalidConfig", err)
			}
			if out != nil || meta != nil {
				t.Errorf("Get returned (%+v, %+v), want nil, nil", out, meta)
			}
			if n := fs.count(); n != 0 {
				t.Errorf("server saw %d requests, want 0", n)
			}
		})
	}
}

// TestWithGetFormatLastWins pins that the last WithGetFormat given decides
// the format of a read (REQ-173).
func TestWithGetFormatLastWins(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	cases := []struct {
		name   string
		format composition.Format
		opts   []composition.GetOption
	}{
		{name: "XML then FLAT", format: composition.FormatFlat, opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatXML, nil),
			composition.WithGetFormat(composition.FormatFlat, wt),
		}},
		{name: "FLAT then XML", format: composition.FormatXML, opts: []composition.GetOption{
			composition.WithGetFormat(composition.FormatFlat, wt),
			composition.WithGetFormat(composition.FormatXML, nil),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFormatServer(t, http.StatusOK, tc.format.MediaType(), encodeAs(t, tc.format, comp, wt))
			out, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
				openehrclient.LatestOf(compositionVOID), tc.opts...)
			if err != nil {
				t.Fatalf("Get error = %v", err)
			}
			if got := fs.only(t).header.Get("Accept"); got != tc.format.MediaType() {
				t.Errorf("Accept = %q, want %q", got, tc.format.MediaType())
			}
			if out == nil {
				t.Error("returned composition is nil, want the decoded body")
			}
		})
	}
}

// TestGetSendsNoSchemaMediaType pins REQ-173: no read the leaf sends carries
// a `.schema`-suffixed media type, in any format, with or without the format
// query parameter. The recording server also fails the test on one in any
// header or query value.
func TestGetSendsNoSchemaMediaType(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	for _, f := range readFormats {
		for _, byQuery := range []bool{false, true} {
			name := f.format.String()
			opts := []composition.GetOption{composition.WithGetFormat(f.format, templateFor(f.format, wt))}
			if byQuery {
				name += "/by query parameter"
				opts = append(opts, composition.WithGetFormatQuery())
			}
			t.Run(name, func(t *testing.T) {
				fs := newFormatServer(t, http.StatusOK, mediaJSON, encodeAs(t, f.format, comp, templateFor(f.format, wt)))
				if _, _, err := composition.Get(t.Context(), newClient(t, fs.srv), ehrIDFixture,
					openehrclient.LatestOf(compositionVOID), opts...); err != nil {
					t.Fatalf("Get error = %v", err)
				}
				if got := fs.only(t).header.Get("Accept"); got == "" || strings.Contains(got, ".schema") {
					t.Errorf("Accept = %q, want a media type without .schema", got)
				}
			})
		}
	}
}

// TestRepositoryGetTakesOptions pins REQ-173: the Repository's Get takes the
// same options as the package function and passes them on.
func TestRepositoryGetTakesOptions(t *testing.T) {
	wt, comp := loadFormatFixture(t)
	t.Run("XML with WithRawBody", func(t *testing.T) {
		body := encodeAs(t, composition.FormatXML, comp, nil)
		fs := newFormatServer(t, http.StatusOK, mediaXML, body)
		var raw composition.Raw
		out, meta, err := composition.NewRepository(newClient(t, fs.srv)).Get(t.Context(), ehrIDFixture,
			openehrclient.LatestOf(compositionVOID),
			composition.WithGetFormat(composition.FormatXML, nil), composition.WithRawBody(&raw))
		if err != nil {
			t.Fatalf("Repository.Get(XML) error = %v", err)
		}
		if got := fs.only(t).header.Get("Accept"); got != mediaXML {
			t.Errorf("Accept = %q, want %q", got, mediaXML)
		}
		sameComposition(t, "returned composition", out, comp)
		wantVersionMeta(t, meta)
		if raw.ContentType != mediaXML || !bytes.Equal(raw.Body, body) {
			t.Errorf("Raw = {%q, %d bytes}, want {%q, the %d bytes sent}", raw.ContentType, len(raw.Body), mediaXML, len(body))
		}
	})
	t.Run("STRUCTURED by query parameter", func(t *testing.T) {
		body := encodeAs(t, composition.FormatStructured, comp, wt)
		fs := newFormatServer(t, http.StatusOK, mediaJSON, body)
		out, _, err := composition.NewRepository(newClient(t, fs.srv)).Get(t.Context(), ehrIDFixture,
			openehrclient.LatestAtTime(compositionVOID, getAt),
			composition.WithGetFormat(composition.FormatStructured, wt), composition.WithGetFormatQuery())
		if err != nil {
			t.Fatalf("Repository.Get(STRUCTURED) error = %v", err)
		}
		req := fs.only(t)
		want := url.Values{"format": {"STRUCTURED"}, "version_at_time": {getAtParam}}
		if !sameQuery(req.query, want) {
			t.Errorf("query = %v, want %v", req.query, want)
		}
		if out == nil {
			t.Fatal("returned composition is nil, want the decoded body")
		}
		if got := encodeAs(t, composition.FormatStructured, out, wt); !bytes.Equal(got, body) {
			t.Errorf("decoded composition re-encodes differently\n got: %s\nwant: %s", got, body)
		}
	})
	t.Run("refusal", func(t *testing.T) {
		fs := newFormatServer(t, http.StatusOK, mediaJSON, nil)
		_, _, err := composition.NewRepository(newClient(t, fs.srv)).Get(t.Context(), ehrIDFixture,
			openehrclient.LatestOf(compositionVOID), composition.WithRawBody(nil))
		if !errors.Is(err, transport.ErrInvalidConfig) {
			t.Errorf("Repository.Get(WithRawBody(nil)) error = %v, want ErrInvalidConfig", err)
		}
		if n := fs.count(); n != 0 {
			t.Errorf("server saw %d requests, want 0", n)
		}
	})
}
