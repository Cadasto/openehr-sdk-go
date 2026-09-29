//go:build importguard_other

// This file does not count: no build compiles a file of package b into
// package a.
package b

import _ "github.com/cadasto/openehr-sdk-go/auth/basic"
