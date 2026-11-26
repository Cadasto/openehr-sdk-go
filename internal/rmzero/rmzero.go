// Package rmzero holds the zero tests that the openehr/rm package registers
// when it is initialised: for an RM class held by value, whether a value of it
// is empty, every field its zero value. The canonical encoders of openehr/rm
// leave out a mandatory member whose value is empty, and the validation floor
// in internal/rmaccess reads a member held by value through the same tests
// wherever it reads one by its zero value, so the two cannot disagree there.
// The floor cannot call the tests itself, as they are unexported, and
// openehr/rm cannot import the floor back. The package uses only the standard
// library, so importing it costs openehr/rm nothing.
package rmzero

// tests holds the registered zero tests, each a func(T) bool for one type T.
// The generated init of openehr/rm writes it, and that init has run before
// any package that imports openehr/rm reads it.
var tests []any

// Register adds fns, each the zero test of one type T as a func(T) bool. The
// generated init of openehr/rm calls it once.
func Register(fns ...any) {
	tests = append(tests, fns...)
}

// Test returns the registered zero test of T, or nil when none is registered,
// as in a program that does not link openehr/rm.
func Test[T any]() func(T) bool {
	for _, fn := range tests {
		if test, ok := fn.(func(T) bool); ok {
			return test
		}
	}
	return nil
}
