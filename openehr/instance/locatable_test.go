package instance

import (
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
