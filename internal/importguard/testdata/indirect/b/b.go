// Package b imports a sub-package of a forbidden package.
package b

import _ "github.com/cadasto/openehr-sdk-go/auth/basic"
