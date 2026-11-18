package generic

// REQ-178: an embedded template that fails to read or compile panics with an
// error that names the template id and wraps the cause.

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

func TestREQ178FailedTemplatePanicsNamingTheID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		document string
		cause    error // a cause the panic's error wraps
	}{
		{
			name:     "read failure",
			document: "not an operational template",
			cause:    io.ErrUnexpectedEOF,
		},
		{
			name:     "compile failure",
			document: `<template xmlns="http://schemas.openehr.org/v1"></template>`,
			cause:    templatecompile.ErrInvalidInput,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const id = "example_template.v9"
			err := panicOf(func() { mustCompile(id, []byte(tc.document)) })
			if err == nil {
				t.Fatalf("mustCompile(%q, %q) did not panic with an error, want one naming %q", id, tc.document, id)
			}
			if !strings.Contains(err.Error(), id) {
				t.Errorf("mustCompile(%q, %q) panicked with %q, want text naming %q", id, tc.document, err, id)
			}
			if !errors.Is(err, tc.cause) {
				t.Errorf("mustCompile(%q, %q) panicked with %q, want it to wrap %v", id, tc.document, err, tc.cause)
			}
		})
	}
}

// panicOf runs f and returns the error it panics with, or nil when f returns
// or panics with a value that is not an error.
func panicOf(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err, _ = r.(error)
		}
	}()
	f()
	return nil
}
