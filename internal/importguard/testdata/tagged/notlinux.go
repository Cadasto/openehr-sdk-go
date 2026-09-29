//go:build !linux

// This file counts: a build for any system but Linux compiles it.
package a

import _ "github.com/cadasto/openehr-sdk-go/auth"
