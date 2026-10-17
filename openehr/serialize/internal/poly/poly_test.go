package poly

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

func TestDecodeErrorUnwrapsTyperegSentinel(t *testing.T) {
	inner := typereg.ErrUnknownType
	de := &DecodeError{Path: "/content/0", Type: "WAT", Inner: inner}
	if !errors.Is(de, typereg.ErrUnknownType) {
		t.Errorf("errors.Is should reach typereg.ErrUnknownType through DecodeError; got %v", de)
	}
}

// The message names the path, the _type and the cause. The cause is an SDK
// sentinel, whose text is value-free; an arbitrary cause would be named by
// its class alone (REQ-052 § Value-free decode errors).
func TestDecodeErrorMessageIncludesPathAndType(t *testing.T) {
	de := &DecodeError{Path: "/content/0", Type: "OBSERVATION", Inner: typereg.ErrUnknownType}
	msg := de.Error()
	for _, want := range []string{"/content/0", "OBSERVATION", typereg.ErrUnknownType.Error()} {
		if !strings.Contains(msg, want) {
			t.Errorf("DecodeError.Error() = %q; missing substring %q", msg, want)
		}
	}
}

func TestResolveTypeMissing(t *testing.T) {
	_, err := ResolveType("__NEVER_REGISTERED__")
	if err == nil {
		t.Fatal("expected error for unregistered _type")
	}
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("err = %v; want errors.Is(_, typereg.ErrUnknownType)", err)
	}
}
