//go:build importguard_fixture

// This file counts: a build with the importguard_fixture tag compiles it.
package a

import _ "github.com/cadasto/openehr-sdk-go/transport"
