package introspect_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/introspect"
)

// REQ-062: RFC 7662 token introspection client — resource-server / opt-in.

func TestIntrospectActiveToken(t *testing.T) {
	// REQ-062
	futureExp := time.Now().Add(1 * time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected Content-Type application/x-www-form-urlencoded, got %q", ct)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer rs-bearer-token" {
			t.Errorf("expected Authorization: Bearer rs-bearer-token, got %q", auth)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if tok := r.FormValue("token"); tok != "access-token-under-test" {
			t.Errorf("expected token=access-token-under-test, got %q", tok)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"active":    true,
			"scope":     "patient/COMPOSITION.read openid",
			"client_id": "c1",
			"patient":   "p1",
			"exp":       futureExp,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := introspect.New(srv.URL+"/introspect", http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Introspect(context.Background(), "access-token-under-test", "rs-bearer-token")
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if !result.Active {
		t.Error("expected Active==true")
	}
	if result.Scope != "patient/COMPOSITION.read openid" {
		t.Errorf("Scope=%q", result.Scope)
	}
	if result.ClientID != "c1" {
		t.Errorf("ClientID=%q", result.ClientID)
	}
	if result.Patient != "p1" {
		t.Errorf("Patient=%q", result.Patient)
	}
	if result.Exp.IsZero() {
		t.Error("Exp should be set")
	}
	if result.Raw == nil {
		t.Error("Raw should be non-nil")
	}
	if _, ok := result.Raw["active"]; !ok {
		t.Error("Raw should carry 'active' key")
	}
	if _, ok := result.Raw["client_id"]; !ok {
		t.Error("Raw should carry 'client_id' key")
	}
}

func TestIntrospectInactiveToken(t *testing.T) {
	// REQ-062: active:false is a successful introspection, not an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":false}`))
	}))
	defer srv.Close()

	client, err := introspect.New(srv.URL, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Introspect(context.Background(), "expired-token", "bearer")
	if err != nil {
		t.Fatalf("Introspect returned error for inactive token (should be nil): %v", err)
	}
	if result.Active {
		t.Error("expected Active==false for inactive token")
	}
}

func TestIntrospectNonTwoXX(t *testing.T) {
	// REQ-062: non-2xx response is a wrapped error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"bad bearer"}`))
	}))
	defer srv.Close()

	client, err := introspect.New(srv.URL, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.Introspect(context.Background(), "some-token", "bad-bearer")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestIntrospectNilHTTPClient(t *testing.T) {
	// REQ-062 / REQ-021: nil http.Client → ErrInvalidConfig.
	_, err := introspect.New("https://example.com/introspect", nil)
	if err == nil {
		t.Fatal("expected ErrInvalidConfig for nil http.Client")
	}
	if !isInvalidConfig(err) {
		t.Errorf("expected errors.Is(err, auth.ErrInvalidConfig), got %v", err)
	}
}

func TestIntrospectBadEndpoint(t *testing.T) {
	// REQ-062: unparseable/relative endpoint → ErrInvalidConfig.
	_, err := introspect.New("not-a-url", http.DefaultClient)
	if err == nil {
		t.Fatal("expected ErrInvalidConfig for bad endpoint")
	}
	if !isInvalidConfig(err) {
		t.Errorf("expected errors.Is(err, auth.ErrInvalidConfig), got %v", err)
	}
}

func TestIntrospectAudArray(t *testing.T) {
	// REQ-062: aud can be a JSON array — parse both cases.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":true,"aud":["rs1","rs2"],"sub":"u1"}`))
	}))
	defer srv.Close()

	client, err := introspect.New(srv.URL, http.DefaultClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.Introspect(context.Background(), "token", "bearer")
	if err != nil {
		t.Fatalf("Introspect: %v", err)
	}
	if !result.Active {
		t.Error("expected Active==true")
	}
	if result.Aud != "rs1 rs2" {
		t.Errorf("Aud=%q, want 'rs1 rs2'", result.Aud)
	}
	if result.Sub != "u1" {
		t.Errorf("Sub=%q", result.Sub)
	}
}

func isInvalidConfig(err error) bool {
	return errors.Is(err, auth.ErrInvalidConfig)
}
