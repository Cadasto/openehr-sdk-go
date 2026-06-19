package rm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

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
