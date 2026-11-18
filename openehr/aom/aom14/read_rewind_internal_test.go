package aom14

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRewindableTakesOnlyARegularFile pins, for REQ-176, that the reader
// reads an operational template again from a file only where the file is a
// regular one: a character device, such as a terminal or the null device,
// can report its offset and still give other bytes when read again, so the
// reader keeps a copy of it as it reads it instead.
func TestRewindableTakesOnlyARegularFile(t *testing.T) {
	regular := filepath.Join(t.TempDir(), "template.opt")
	if err := os.WriteFile(regular, []byte("<template/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
		want       bool
	}{
		{name: "regular file", path: regular, want: true},
		{name: "character device", path: os.DevNull, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.Open(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			if _, _, got := rewindable(f); got != tc.want {
				t.Errorf("rewindable(%s) = %t, want %t", tc.path, got, tc.want)
			}
		})
	}
}
