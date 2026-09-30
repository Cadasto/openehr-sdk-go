package probeclass

import "strings"

// Probe902LocalOnly reaches no backend package: it uses the strings package
// and a same-package helper, neither of which opens a connection.
func Probe902LocalOnly() string {
	return strings.ToUpper(greeting())
}

func greeting() string {
	return "hello"
}

// probe905Unexported is not an exported probe, so the classifier must not
// list it.
func probe905Unexported() string {
	return greeting()
}
