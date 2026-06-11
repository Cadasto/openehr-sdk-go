package template

import (
	"errors"
	"strings"
	"testing"
)

func TestParseOPT_inputTooLarge(t *testing.T) {
	orig := maxOPTBytes
	maxOPTBytes = 64
	t.Cleanup(func() { maxOPTBytes = orig })

	// A syntactically-started XML document that exceeds the 64-byte cap.
	// The padding is enough to push it well over the limit.
	xml := "<template>" + strings.Repeat("x", 100) + "</template>"
	_, err := ParseOPT(strings.NewReader(xml))
	if err == nil {
		t.Fatal("expected error when input exceeds maxOPTBytes")
	}
	if !errors.Is(err, ErrInvalidOPT) {
		t.Errorf("error should wrap ErrInvalidOPT, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}
