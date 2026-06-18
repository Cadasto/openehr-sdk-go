package auth_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// TestExchangeErrorTerminalClassification verifies that Terminal() returns true
// only for 4xx responses carrying invalid_grant or invalid_client — transient
// failures (5xx, network, context, unparsed) must return false (REQ-063).
func TestExchangeErrorTerminalClassification(t *testing.T) { // REQ-063
	t.Parallel()
	cases := []struct {
		name       string
		err        *auth.ExchangeError
		wantTermin bool
	}{
		{
			name: "400_invalid_grant",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 400,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_grant"},
			},
			wantTermin: true,
		},
		{
			name: "400_invalid_client",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 400,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_client"},
			},
			wantTermin: true,
		},
		{
			name: "401_invalid_grant",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 401,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_grant"},
			},
			wantTermin: true,
		},
		{
			name: "400_invalid_token",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 400,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_token"},
			},
			wantTermin: true,
		},
		{
			name: "400_other_code",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 400,
				OAuth2:     &auth.OAuth2Error{Code: "access_denied"},
			},
			wantTermin: false,
		},
		{
			name: "500_invalid_grant",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 500,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_grant"},
			},
			wantTermin: false,
		},
		{
			name: "503_invalid_grant",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 503,
				OAuth2:     &auth.OAuth2Error{Code: "invalid_grant"},
			},
			wantTermin: false,
		},
		{
			name: "zero_status_no_oauth2",
			err: &auth.ExchangeError{
				Sentinel: auth.ErrRefreshFailed,
			},
			wantTermin: false,
		},
		{
			name: "400_nil_oauth2",
			err: &auth.ExchangeError{
				Sentinel:   auth.ErrRefreshFailed,
				StatusCode: 400,
				OAuth2:     nil,
			},
			wantTermin: false,
		},
		{
			name:       "nil_receiver",
			err:        nil,
			wantTermin: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.err.Terminal()
			if got != tc.wantTermin {
				t.Errorf("Terminal() = %v, want %v (err = %+v)", got, tc.wantTermin, tc.err)
			}
		})
	}
}

// TestExchangeErrorSentinelUnwrap verifies errors.Is still works through the
// ExchangeError wrapper after adding Terminal (regression guard).
func TestExchangeErrorSentinelUnwrap(t *testing.T) {
	t.Parallel()
	ex := &auth.ExchangeError{
		Sentinel:   auth.ErrRefreshFailed,
		StatusCode: 400,
		OAuth2:     &auth.OAuth2Error{Code: "invalid_grant"},
	}
	if !errors.Is(ex, auth.ErrRefreshFailed) {
		t.Error("errors.Is(ex, ErrRefreshFailed) = false, want true")
	}
}
