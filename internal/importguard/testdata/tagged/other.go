//go:build importguard_other

// This file counts: like the go command, Scan and Imports do not read the
// package clause, so a file naming package b still counts in package a.
package b

import _ "github.com/cadasto/openehr-sdk-go/auth/basic"
