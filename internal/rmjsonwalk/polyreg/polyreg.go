// Package polyreg holds the table of polymorphic RM interfaces that the
// openehr/rm package registers when it is initialised: for each interface, the
// BMM class it stands for. The canonical-JSON walker in internal/rmjsonwalk
// reads a decode target's Go type through it, so neither the walker nor
// openehr/serialize/canjson imports openehr/rm. The package uses only the
// standard library, so importing it costs openehr/rm nothing.
package polyreg

import "sync/atomic"

// ClassFunc names the BMM class behind the polymorphic RM interface p points
// to, such as a pointer to an rm.DataValue: the abstract class the interface
// stands for, or for a narrow interface (<Parent>Like) its parent class. ok is
// false when p points to no polymorphic RM interface.
type ClassFunc func(p any) (class string, ok bool)

// table is the registered ClassFunc, or nil before openehr/rm registers one.
var table atomic.Pointer[ClassFunc]

// Register installs fn as the table. The generated code of openehr/rm calls it
// once, from its init; a later call replaces the table, and a nil fn is
// ignored.
func Register(fn ClassFunc) {
	if fn != nil {
		table.Store(&fn)
	}
}

// Class names the BMM class behind the polymorphic RM interface p points to,
// as the registered table does. ok is false when no table is registered, as in
// a program that does not link openehr/rm and so holds no RM interface, or
// when p points to no polymorphic RM interface.
func Class(p any) (class string, ok bool) {
	fn := table.Load()
	if fn == nil {
		return "", false
	}
	return (*fn)(p)
}
