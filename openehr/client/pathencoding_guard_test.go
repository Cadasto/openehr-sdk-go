package client_test

// pathencoding_guard_test.go — REQ-095. The transport is the single canonical
// path encoder (Request.Path is a decoded url.URL.Path), so NO leaf client under
// openehr/client may pre-escape a path parameter with url.PathEscape — doing so
// double-encodes any id carrying a percent-encodable character (a space →
// %20 → %2520 → 404). This walks the whole client tree so the invariant holds
// uniformly across every REST resource, not only the packages fixed first.
// url.PathUnescape (decoding a server-supplied Location header) is allowed.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoPathEscapeInClientPathParams(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(self) // .../openehr/client
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "url.PathEscape(") {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s calls url.PathEscape into a request path; interpolate the raw id and let the transport encode once (REQ-095)", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
