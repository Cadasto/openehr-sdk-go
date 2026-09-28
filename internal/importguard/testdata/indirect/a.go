// Package a reaches a forbidden package only through b, by two routes.
package a

import (
	_ "github.com/cadasto/openehr-sdk-go/internal/importguard/testdata/indirect/b"
	_ "github.com/cadasto/openehr-sdk-go/internal/importguard/testdata/indirect/c"
)
