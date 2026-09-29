//go:build ignore

// Package main is a tool program kept out of every build, and the only Go
// file here, so Scan and Imports must refuse the directory as vacuous.
package main

import _ "github.com/cadasto/openehr-sdk-go/transport"
