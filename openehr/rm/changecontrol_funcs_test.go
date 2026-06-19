package rm_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestVersionedObjectContainerOpsFailLoud asserts the out-of-scope
// VERSIONED_OBJECT container operations remain fail-loud panic stubs
// (server-mediated, not in-memory — REQ-122), so they aren't mistaken
// for a silent zero-value.
func TestVersionedObjectContainerOpsFailLoud(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("VersionCount() should panic (out of scope, server-mediated)")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "not implemented: VERSIONED_OBJECT.version_count") {
			t.Errorf("unexpected panic value: %v", r)
		}
	}()
	vo := rm.VersionedObject[any]{}
	_ = vo.VersionCount()
}

// REQ-122 — version-control derived helper.

func TestVersionIsBranch(t *testing.T) {
	branch := rm.OriginalVersion[any]{UID: rm.ObjectVersionID{Value: "obj::sys::1.1.1"}}
	if !branch.IsBranch() {
		t.Error("branch OriginalVersion IsBranch = false, want true")
	}

	trunk := rm.OriginalVersion[any]{UID: rm.ObjectVersionID{Value: "obj::sys::1"}}
	if trunk.IsBranch() {
		t.Error("trunk OriginalVersion IsBranch = true, want false")
	}

	imported := rm.ImportedVersion[any]{
		Item: rm.OriginalVersion[any]{UID: rm.ObjectVersionID{Value: "obj::sys::2.3.1"}},
	}
	if !imported.IsBranch() {
		t.Error("branch ImportedVersion IsBranch = false, want true")
	}
}
