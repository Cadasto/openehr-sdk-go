package poly

import (
	"errors"
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

func TestDecodeErrorMessageIncludesPathAndType(t *testing.T) {
	de := &DecodeError{Path: "/content/0", Type: "OBSERVATION", Inner: errors.New("inner")}
	msg := de.Error()
	for _, want := range []string{"/content/0", "OBSERVATION", "inner"} {
		if !contains(msg, want) {
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

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
