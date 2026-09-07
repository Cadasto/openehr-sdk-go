package ehr

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
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

	// The known codes are the pinned openEHR *version lifecycle state*
	// group's members, not a list typed here (REQ-034): every member is
	// valid, formats into a header, and reports the pin's own rubric.
	t.Run("accepts all known codes", func(t *testing.T) {
		for c := range terminology.VersionLifecycleState.All() {
			s := LifecycleState(c.Code)
			if !s.IsValid() {
				t.Errorf("%q should be valid", s)
			}
			if _, err := FormatLifecycleStateHeader(s); err != nil {
				t.Errorf("FormatLifecycleStateHeader(%q): %v", s, err)
			}
			rubric, ok := s.Rubric()
			if !ok {
				t.Errorf("LifecycleState(%q).Rubric() reported absence, want %q", s, c.Rubric)
				continue
			}
			if rubric != c.Rubric {
				t.Errorf("LifecycleState(%q).Rubric() = %q, want the pinned %q", s, rubric, c.Rubric)
			}
		}
	})
}

// TestLifecycleStateConstantsCoverTheGroup — REQ-034: the promoted constants
// MUST be exactly the group's members, so a member the pin carries is always
// nameable and no constant outlives its concept.
func TestLifecycleStateConstantsCoverTheGroup(t *testing.T) {
	want := map[LifecycleState]bool{
		LifecycleStateComplete:   true,
		LifecycleStateIncomplete: true,
		LifecycleStateDeleted:    true,
		LifecycleStateInactive:   true,
		LifecycleStateAbandoned:  true,
	}
	for c := range terminology.VersionLifecycleState.All() {
		if !want[LifecycleState(c.Code)] {
			t.Errorf("group member %s (%s) has no LifecycleState constant", c.Code, c.Rubric)
		}
	}
	if len(want) != terminology.VersionLifecycleState.Len() {
		t.Errorf("%d constants, group has %d members", len(want), terminology.VersionLifecycleState.Len())
	}
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
