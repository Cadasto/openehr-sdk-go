package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// TestErrorEnvelopeShapes pins REQ-093 on the envelope shapes the openEHR contract
// itself shows: the overview's example has a numeric `code`, and the
// ITS-REST `Error` schema behind the 400 bodies has `validationErrors`
// beside `message` and no `code` at all.
func TestErrorEnvelopeShapes(t *testing.T) {
	const phi = "patient 1234 has no such composition"
	tests := []struct {
		name         string
		body         string
		wantEnvelope bool
		wantCode     string
		raw          bool
		wantMessage  string
		wantVE       []string
	}{
		{
			name:         "string code, default client",
			body:         `{"message":"` + phi + `","code":"NOT_FOUND"}`,
			wantEnvelope: true,
			wantCode:     "NOT_FOUND",
		},
		{
			name:         "numeric code keeps the envelope, default client",
			body:         `{"message":"` + phi + `","code":90000,"errors":[]}`,
			wantEnvelope: true,
			wantCode:     "90000",
		},
		{
			name:         "numeric code, raw bodies keep the message",
			body:         `{"message":"` + phi + `","code":90000}`,
			wantEnvelope: true,
			wantCode:     "90000",
			raw:          true,
			wantMessage:  phi,
		},
		{
			name:         "validationErrors only, default client drops the free text",
			body:         `{"message":"` + phi + `","validationErrors":["` + phi + `"]}`,
			wantEnvelope: true,
		},
		{
			name:         "validationErrors only, raw bodies keep them",
			body:         `{"message":"` + phi + `","validationErrors":["error1","error2"]}`,
			wantEnvelope: true,
			raw:          true,
			wantMessage:  phi,
			wantVE:       []string{"error1", "error2"},
		},
		{
			name:         "validationErrors without a message still counts",
			body:         `{"validationErrors":["error1"]}`,
			wantEnvelope: true,
			raw:          true,
			wantVE:       []string{"error1"},
		},
		{
			name:         "a code that is neither string nor number is ignored, not fatal",
			body:         `{"message":"` + phi + `","code":{"x":1}}`,
			wantEnvelope: true,
		},
		{name: "not an envelope", body: `{"unrelated":true}`},
		{name: "not JSON", body: `<html>bad gateway</html>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			opts := []Option{WithHTTPClient(srv.Client())}
			if tt.raw {
				opts = append(opts, WithRawErrorBodies(true))
			}
			c, err := New(newCatalog(t, srv), opts...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Do(t.Context(), &Request{Path: "/x"})
			we, ok := errors.AsType[*WireError](err)
			if !ok || we == nil {
				t.Fatalf("expected *WireError, got %T (%v)", err, err)
			}
			if !tt.wantEnvelope {
				if we.OpenEHR != nil {
					t.Fatalf("OpenEHR = %+v, want nil for a body that is no envelope", we.OpenEHR)
				}
				return
			}
			if we.OpenEHR == nil {
				t.Fatal("OpenEHR = nil, want the decoded envelope")
			}
			if we.OpenEHR.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", we.OpenEHR.Code, tt.wantCode)
			}
			if we.OpenEHR.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", we.OpenEHR.Message, tt.wantMessage)
			}
			if !slices.Equal(we.OpenEHR.ValidationErrors, tt.wantVE) {
				t.Errorf("ValidationErrors = %v, want %v", we.OpenEHR.ValidationErrors, tt.wantVE)
			}
			if !tt.raw && strings.Contains(we.Error(), "1234") {
				t.Errorf("Error() leaks the free text: %q", we.Error())
			}
		})
	}
}
