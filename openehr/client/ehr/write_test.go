package ehr_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

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

	out, meta, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferRepresentation), "composition", okComposition)

	var nre *openehrclient.NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("empty body must wrap ErrInvalidShape")
	}
	if openehrclient.HasResource(out) {
		t.Error("HasResource true on absent representation")
	}
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive the failed representation")
	}
	const wantUID openehrclient.VersionUID = "obj-1::sys::1"
	if meta.VersionUID != wantUID || nre.Meta.VersionUID != wantUID {
		t.Errorf("VersionUID meta=%q nre=%q, want %q", meta.VersionUID, nre.Meta.VersionUID, wantUID)
	}
}

// REQ-094: a 2xx representation body of JSON null carries no resource and
// classifies as empty — json.Unmarshal(null, &struct) is a nil-error no-op,
// so without the guard a zero-value resource would report as full success.
func TestWriteResultRepresentationNullBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/ehr/ehr-1/composition/obj-1::sys::1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("null\n"))
	}))
	defer srv.Close()

	out, meta, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferRepresentation), "composition", okComposition)

	var nre *openehrclient.NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("null body: err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a null body classifies as empty and must wrap ErrInvalidShape")
	}
	if openehrclient.HasResource(out) {
		t.Error("no resource on a null representation body")
	}
	if meta == nil || nre.Meta == nil {
		t.Fatal("commit metadata must survive a null body")
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
	_, meta, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferRepresentation), "composition",
		func([]byte) (*rm.Composition, error) { return nil, decodeErr })

	var nre *openehrclient.NoRepresentationError
	if !errors.As(err, &nre) {
		t.Fatalf("err = %v, want *NoRepresentationError", err)
	}
	if !errors.Is(err, decodeErr) {
		t.Error("decode failure must be the wrapped Cause")
	}
	if errors.Is(err, transport.ErrInvalidShape) {
		t.Error("a decode failure is not an empty-body ErrInvalidShape")
	}
	const wantUID openehrclient.VersionUID = "obj-1::sys::1"
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

	out, meta, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferIdentifier), "composition", okComposition)
	if err != nil {
		t.Fatalf("identifier success err = %v", err)
	}
	if openehrclient.HasResource(out) {
		t.Error("identifier mode returns no resource")
	}
	if meta == nil {
		t.Error("metadata must be populated in identifier mode")
	}
}

// REQ-094: an identifier body that cannot be resolved is a plain labeled
// error on the identifier arm — never reclassified as a
// NoRepresentationError (that type is representation-mode only).
func TestWriteResultIdentifierResolveFailureNotNoRepresentation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not an identifier"))
	}))
	defer srv.Close()

	_, meta, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferIdentifier), "composition", okComposition)
	if !errors.Is(err, transport.ErrInvalidShape) {
		t.Fatalf("unresolvable identifier body: err = %v, want ErrInvalidShape", err)
	}
	if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Fatal("an identifier-arm failure must not be a NoRepresentationError")
	}
	if meta == nil {
		t.Error("metadata must survive an identifier-resolve failure")
	}
}

// REQ-094: a wire failure stays a *transport.WireError and is never
// reclassified as a NoRepresentationError.
func TestWriteResultWireErrorNotNoRepresentation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	_, _, err := openehrclient.WriteResult(t.Context(), newClient(t, srv), writeReq(transport.PreferRepresentation), "composition", okComposition)

	if _, ok := errors.AsType[*transport.WireError](err); !ok {
		t.Fatalf("409 err = %v, want *transport.WireError", err)
	}
	if _, ok := errors.AsType[*openehrclient.NoRepresentationError](err); ok {
		t.Fatal("a wire failure must not be a NoRepresentationError")
	}
}
