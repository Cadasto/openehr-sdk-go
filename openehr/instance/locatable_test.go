package instance

import (
	"regexp"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestApplyLocatableIdentityTypedNil pins the write-path typed-nil
// guard (PR #71 follow-up): a typed-nil pointer satisfies
// rm.MutableLocatable, so without the rm.IsTypedNil guard the first
// setter — or the set-only-if-unset GetUID read — would panic.
// Unreachable from the generator today (registry constructors return
// live pointers), guarded to match the read paths.
func TestApplyLocatableIdentityTypedNil(t *testing.T) {
	uidSource := func() *rm.HierObjectID {
		t.Fatal("uidSource must not be called for a typed-nil value")
		return nil
	}
	for _, v := range []any{
		(*rm.Composition)(nil), // stampsUID member
		(*rm.Section)(nil),     // non-uid LOCATABLE
		nil,                    // bare nil interface
		rm.Section{},           // value form: not MutableLocatable
	} {
		applyLocatableIdentity(v, "at0000", "name", nil, uidSource)
	}
}

// uuidV4Text matches the RFC 9562 §4 textual form of a version-4 UUID:
// 36 characters of lowercase hex and dashes, the version nibble 4 at
// index 14 and a variant nibble in [89ab] at index 19.
var uuidV4Text = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestNewHierObjectIDDefaultUID pins the default UID source — the one
// used when Options.UIDSource is nil: successive calls yield distinct
// values, and each is a version-4 UUID in the RFC 9562 textual form. A
// regression to a constant, a counter, a timestamp or an upper-case
// rendering fails here.
func TestNewHierObjectIDDefaultUID(t *testing.T) {
	first, second := newHierObjectID(), newHierObjectID()
	if first == nil || second == nil {
		t.Fatalf("newHierObjectID returned nil: first=%v second=%v", first, second)
	}
	if first.Value == second.Value {
		t.Errorf("two generated uids are identical: %q", first.Value)
	}
	for _, v := range []string{first.Value, second.Value} {
		if !uuidV4Text.MatchString(v) {
			t.Errorf("uid %q is not an RFC 9562 version-4 uuid (len %d)", v, len(v))
		}
	}
}
