package definition

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

func TestUploadTemplate_inputTooLarge(t *testing.T) {
	orig := maxUploadBytes
	maxUploadBytes = 16
	t.Cleanup(func() { maxUploadBytes = orig })

	// Feed 100 bytes — over the 16-byte cap.
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 100))
	_, _, err := UploadTemplate(t.Context(), nil, FormatADL14, body)
	if err == nil {
		t.Fatal("expected error when body exceeds maxUploadBytes")
	}
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("error should wrap ErrInvalidConfig, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}
