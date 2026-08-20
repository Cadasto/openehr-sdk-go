package ehr

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func writeTestClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeReq(prefer transport.Prefer) *transport.Request {
	return &transport.Request{
		Method: http.MethodPost,
		Path:   "/ehr/ehr-1/composition",
		Route:  "/ehr/{ehr_id}/composition",
		Body:   []byte("{}"),
		Prefer: prefer,
	}
}

func okComposition([]byte) (*rm.Composition, error) { return &rm.Composition{}, nil }

// REQ-094: a 2xx representation with an empty body is a NoRepresentationError
// carrying the commit metadata — not a silently-nil success.
func TestWriteResultRepresentationEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/ehr-1/composition/obj-1::sys::1")
		w.WriteHeader(http.StatusCreated) // representation asked, empty body
	}))
	defer srv.Close()

	out, meta, err := WriteResult(t.Context(), writeTestClient(t, srv), writeReq(transport.PreferRepresentation), "composition", okComposition)

	var nre *NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("empty body must wrap ErrInvalidShape")
	}
	if HasResource(out) {
		t.Error("HasResource true on absent representation")
	}
	if meta == nil || nre.Meta == nil {
		t.Error("commit metadata must survive the failed representation")
	}
	const wantUID VersionUID = "obj-1::sys::1"
	if meta.VersionUID != wantUID || nre.Meta.VersionUID != wantUID {
		t.Errorf("VersionUID meta=%q nre=%q, want %q", meta.VersionUID, nre.Meta.VersionUID, wantUID)
	}
}

// REQ-094: a 2xx representation whose body cannot be decoded is a
// NoRepresentationError wrapping the decoder's error, not ErrInvalidShape.
func TestWriteResultRepresentationDecodeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/ehr-1/composition/obj-1::sys::1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	decodeErr := errors.New("boom")
	_, meta, err := WriteResult(t.Context(), writeTestClient(t, srv), writeReq(transport.PreferRepresentation), "composition",
		func([]byte) (*rm.Composition, error) { return nil, decodeErr })

	var nre *NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, decodeErr) {
		t.Error("decode failure must be the wrapped Cause")
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a decode failure is not an empty-body ErrInvalidShape")
	}
	const wantUID VersionUID = "obj-1::sys::1"
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive a decode failure")
	}
	if meta.VersionUID != wantUID || nre.Meta.VersionUID != wantUID {
		t.Errorf("VersionUID meta=%q nre=%q, want %q", meta.VersionUID, nre.Meta.VersionUID, wantUID)
	}
}

// REQ-094: identifier mode is a nil-error success that returns no resource;
// HasResource reports the typed-nil zero as absent.
func TestWriteResultIdentifierSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/ehr-1/composition/obj-1::sys::1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uid":"obj-1::sys::1"}`))
	}))
	defer srv.Close()

	out, meta, err := WriteResult(t.Context(), writeTestClient(t, srv), writeReq(transport.PreferIdentifier), "composition", okComposition)
	if err != nil {
		t.Fatalf("identifier success err = %v", err)
	}
	if HasResource(out) {
		t.Error("identifier mode returns no resource")
	}
	if meta == nil {
		t.Error("metadata must be populated in identifier mode")
	}
}

// REQ-094: a wire failure stays a *transport.WireError and is never
// reclassified as a NoRepresentationError.
func TestWriteResultWireErrorNotNoRepresentation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	_, _, err := WriteResult(t.Context(), writeTestClient(t, srv), writeReq(transport.PreferRepresentation), "composition", okComposition)

	if _, ok := errors.AsType[*transport.WireError](err); !ok {
		t.Fatalf("409 err = %v, want *transport.WireError", err)
	}
	if _, ok := errors.AsType[*NoRepresentationError](err); ok {
		t.Fatal("a wire failure must not be a NoRepresentationError")
	}
}
