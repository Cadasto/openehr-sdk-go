package ehr

import (
	"strings"
	"testing"
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
		if _, err := FormatLifecycleStateHeader("532\r\nX-Inject: 1"); err == nil {
			t.Fatal("expected error for control characters, got nil")
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
