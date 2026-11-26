package rmzero

import "testing"

// point stands in for an RM class held by value.
type point struct{ x, y int }

// TestRegisterAndTest pins the seam: before a test is registered for a type
// there is none, and once one is, Test returns it for that type and for no
// other. This test binary does not link openehr/rm, so nothing is registered
// when it starts.
func TestRegisterAndTest(t *testing.T) {
	saved := tests
	t.Cleanup(func() { tests = saved })

	if Test[point]() != nil {
		t.Fatal("Test[point]() before Register is not nil")
	}
	Register(func(v point) bool { return v == point{} }, func(s string) bool { return s == "" })
	zero := Test[point]()
	if zero == nil {
		t.Fatal("Test[point]() after Register is nil")
	}
	if !zero(point{}) || zero(point{x: 1}) || zero(point{y: 1}) {
		t.Errorf("Test[point]() is not the registered test: zero(point{}) = %v, zero(point{x: 1}) = %v, zero(point{y: 1}) = %v", zero(point{}), zero(point{x: 1}), zero(point{y: 1}))
	}
	if Test[int]() != nil {
		t.Error("Test[int]() is not nil, but no test of int is registered")
	}
	if empty := Test[string](); empty == nil || !empty("") {
		t.Error("Test[string]() does not return the registered string test")
	}
}
