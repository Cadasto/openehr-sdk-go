// Package a has only a test file, so Scan must refuse it even though the
// test file imports a forbidden package.
package a

import _ "github.com/cadasto/openehr-sdk-go/transport"
