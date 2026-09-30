// Package a uses cgo in its only file, so Scan and Imports must read it
// whether cgo is on or off.
package a

import "C"

import _ "github.com/cadasto/openehr-sdk-go/transport"
