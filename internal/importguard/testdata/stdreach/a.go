// Package a reaches net/http only through the standard library: expvar
// imports it.
package a

import _ "expvar"
