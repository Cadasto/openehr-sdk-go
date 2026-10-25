package main

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
)

// redacted replaces a credential wherever the run would otherwise write it.
const redacted = "[redacted]"

// redactor removes the CDR credentials from everything the run writes: the
// output files, the progress lines and the error it ends with. The run never
// puts a credential there itself; a CDR can, by echoing the request's
// Authorization header or a URL with userinfo in its error text, and that
// text is written to the output.
//
// It removes the forms a credential takes on the wire: user:pass and its
// base64 encoding, which is what a Basic Authorization header carries, each
// also in its JSON-escaped spelling. The password alone is not removed: it
// may equal an ordinary word of the output, such as the CDR's own name.
type redactor struct {
	secrets [][]byte
}

func newRedactor(cdrs []cdrTarget) redactor {
	var r redactor
	add := func(s string) {
		if s == "" {
			return
		}
		r.secrets = append(r.secrets, []byte(s))
		if esc := jsonEscape(s); esc != s {
			r.secrets = append(r.secrets, []byte(esc))
		}
	}
	for _, c := range cdrs {
		if c.user == "" {
			continue
		}
		pair := c.user + ":" + c.pass
		add(pair)
		add(base64.StdEncoding.EncodeToString([]byte(pair)))
	}
	// Longest first, so a secret inside another is never left half-replaced.
	slices.SortFunc(r.secrets, func(a, b []byte) int { return cmp.Compare(len(b), len(a)) })
	return r
}

// jsonEscape returns s as it appears inside a JSON string written by
// writeJSON.
func jsonEscape(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return s
	}
	out := bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	return string(out[1 : len(out)-1])
}

func (r redactor) bytes(b []byte) []byte {
	for _, s := range r.secrets {
		b = bytes.ReplaceAll(b, s, []byte(redacted))
	}
	return b
}

func (r redactor) string(s string) string { return string(r.bytes([]byte(s))) }

// sink owns the output directory. Every byte the run writes passes through
// its redactor.
type sink struct {
	dir string
	red redactor
	mu  sync.Mutex
	log io.Writer
}

// logf writes one progress line.
func (s *sink) logf(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.log.Write(s.red.bytes(fmt.Appendf(nil, format+"\n", args...)))
}

// writeFile writes b at rel below the output directory and returns rel.
func (s *sink) writeFile(rel string, b []byte) (string, error) {
	path := filepath.Join(s.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, s.red.bytes(b), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// writeJSON writes v as indented JSON at rel.
func (s *sink) writeJSON(rel string, v any) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", rel, err)
	}
	_, err := s.writeFile(rel, b.Bytes())
	return err
}

// writeJSONL writes one JSON line per record at rel.
func writeJSONL[T any](s *sink, rel string, recs []T) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encode %s: %w", rel, err)
		}
	}
	_, err := s.writeFile(rel, b.Bytes())
	return err
}

// unsafeName matches the runs of characters a file name built from a
// template id leaves out.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileName turns a template id into a file or directory name.
func fileName(id string) string { return unsafeName.ReplaceAllString(id, "_") }
