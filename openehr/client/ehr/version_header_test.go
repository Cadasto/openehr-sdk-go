package ehr

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestFormatLifecycleStateHeader(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := FormatLifecycleStateHeader("")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("code", func(t *testing.T) {
		got, err := FormatLifecycleStateHeader("532")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != `lifecycle_state.code_string="532"` {
			t.Errorf("got %q, want lifecycle_state.code_string=\"532\"", got)
		}
	})

	t.Run("rejects control chars", func(t *testing.T) {
		_, err := FormatLifecycleStateHeader("532\r\nX-Inject: 1")
		if err == nil {
			t.Fatal("expected error for control characters, got nil")
		}
		if !errors.Is(err, transport.ErrInvalidConfig) {
			t.Errorf("err = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("rejects unknown code", func(t *testing.T) {
		_, err := FormatLifecycleStateHeader("999")
		if !errors.Is(err, transport.ErrInvalidConfig) {
			t.Errorf("err = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("accepts all known codes", func(t *testing.T) {
		for _, s := range []LifecycleState{LifecycleStateComplete, LifecycleStateIncomplete, LifecycleStateDeleted} {
			if !s.IsValid() {
				t.Errorf("%q should be valid", s)
			}
			if _, err := FormatLifecycleStateHeader(s); err != nil {
				t.Errorf("FormatLifecycleStateHeader(%q): %v", s, err)
			}
		}
	})
}

// TestFormatLifecycleStateHeaderInjection guards the header-injection vector
// explicitly: a CRLF must never reach the header value.
func TestFormatLifecycleStateHeaderInjection(t *testing.T) {
	_, err := FormatLifecycleStateHeader("ok\nevil")
	if err == nil {
		t.Fatal("expected control-char rejection")
	}
	if !strings.Contains(err.Error(), "control characters") {
		t.Errorf("error = %v, want mention of control characters", err)
	}
}
