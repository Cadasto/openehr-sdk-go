package polyreg_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmjsonwalk/polyreg"
)

// iface stands in for a polymorphic RM interface.
type iface interface{ isIface() }

// TestRegisterAndClass pins the seam: before a table is registered nothing
// is a polymorphic RM interface, a nil table is ignored, and once one is
// registered Class answers from it. This test binary does not link
// openehr/rm, so the table starts empty.
func TestRegisterAndClass(t *testing.T) {
	if class, ok := polyreg.Class(new(iface)); ok {
		t.Fatalf("Class before Register = %q, true; want false: no table is registered", class)
	}
	polyreg.Register(nil)
	if class, ok := polyreg.Class(new(iface)); ok {
		t.Fatalf("Class after Register(nil) = %q, true; want false: a nil table is ignored", class)
	}
	polyreg.Register(func(p any) (string, bool) {
		if _, ok := p.(*iface); ok {
			return "IFACE", true
		}
		return "", false
	})
	if class, ok := polyreg.Class(new(iface)); !ok || class != "IFACE" {
		t.Errorf("Class(*iface) = %q, %v; want IFACE, true", class, ok)
	}
	if class, ok := polyreg.Class(new(int)); ok {
		t.Errorf("Class(*int) = %q, true; want false: the table does not name it", class)
	}
}
