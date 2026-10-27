package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
)

// redacted replaces a credential wherever the run would otherwise write it.
const redacted = "[redacted]"

// redactor removes the CDR credentials from everything the run keeps or
// writes: the CDRs' answers (cdrClient.call), the output files, the progress
// lines and the error it ends with. The run never puts a credential there
// itself; a CDR can, by echoing the request's Authorization header or a URL
// with userinfo in its error text, and that text is kept and written.
//
// It removes the forms a credential takes on the wire: user:pass, the same
// pair percent-encoded as a URL's userinfo (op:p%40ss, its hex digits in
// upper or lower case), and the pair's base64 encoding, which is what a
// Basic Authorization header carries. Each
// is found spelt out, byte for byte, or JSON-escaped, in whole or in part, as
// a CDR's JSON encoder or the run's own may write it: \/, \", \\, the other
// short escapes and \uXXXX (a surrogate pair included) each read as the
// character they stand for, so YWI\/OmNkPn4\u003d matches as YWI/OmNkPn4=
// does. Both are tried at every byte, so a password that holds \t or \n is
// found spelt out although the escape reading takes it for a tab or a new
// line, and so is a credential right after a backslash. The password alone
// is not removed: it may equal an ordinary word of the output, such as the
// CDR's own name.
type redactor struct {
	secrets []secret // longest first, so one inside another is never left half-replaced
}

// percentHex matches a percent-encoded octet with upper-case hex digits, the
// form url.Userinfo writes; RFC 3986 §2.1 lets a server write them in lower
// case.
var percentHex = regexp.MustCompile(`%[0-9A-F]{2}`)

// secret is one credential form, as its bytes and as its characters.
type secret struct {
	raw   []byte
	runes []rune
}

func newRedactor(cdrs []cdrTarget) redactor {
	var r redactor
	for _, c := range cdrs {
		if c.user == "" {
			continue
		}
		pair := c.user + ":" + c.pass
		forms := []string{pair, base64.StdEncoding.EncodeToString([]byte(pair))}
		if ui := url.UserPassword(c.user, c.pass).String(); ui != pair {
			forms = append(forms, ui)
			if lower := percentHex.ReplaceAllStringFunc(ui, strings.ToLower); lower != ui {
				forms = append(forms, lower)
			}
		}
		for _, form := range forms {
			r.secrets = append(r.secrets, secret{raw: []byte(form), runes: []rune(form)})
		}
	}
	slices.SortFunc(r.secrets, func(a, b secret) int { return cmp.Compare(len(b.runes), len(a.runes)) })
	return r
}

func (r redactor) bytes(b []byte) []byte {
	if len(r.secrets) == 0 {
		return b
	}
	var out []byte
	last := 0
	for p := 0; p < len(b); {
		if n := r.match(b, p); n > 0 {
			out = append(append(out, b[last:p]...), redacted...)
			p += n
			last = p
			continue
		}
		p++
	}
	if out == nil {
		return b
	}
	return append(out, b[last:]...)
}

func (r redactor) string(s string) string { return string(r.bytes([]byte(s))) }

// match returns the length in bytes of the secret that starts at b[p], or 0:
// spelt out there byte for byte, or read there character by character, a
// JSON escape as the character it stands for (unit).
func (r redactor) match(b []byte, p int) int {
	first, w := unit(b, p)
	for _, s := range r.secrets {
		if bytes.HasPrefix(b[p:], s.raw) {
			return len(s.raw)
		}
		if s.runes[0] != first {
			continue
		}
		q, i := p+w, 1
		for i < len(s.runes) && q < len(b) {
			c, n := unit(b, q)
			if c != s.runes[i] {
				break
			}
			q, i = q+n, i+1
		}
		if i == len(s.runes) {
			return q - p
		}
	}
	return 0
}

// unit reads the character at b[p], a JSON escape as the character it
// stands for, and returns it with its length in bytes.
func unit(b []byte, p int) (rune, int) {
	if b[p] != '\\' || p+1 == len(b) {
		return utf8.DecodeRune(b[p:])
	}
	switch c := b[p+1]; c {
	case '"', '\\', '/':
		return rune(c), 2
	case 'b':
		return '\b', 2
	case 'f':
		return '\f', 2
	case 'n':
		return '\n', 2
	case 'r':
		return '\r', 2
	case 't':
		return '\t', 2
	case 'u':
		hi, ok := hex4(b, p+2)
		if !ok {
			break
		}
		if utf16.IsSurrogate(hi) && p+12 <= len(b) && b[p+6] == '\\' && b[p+7] == 'u' {
			if lo, ok := hex4(b, p+8); ok {
				if c := utf16.DecodeRune(hi, lo); c != utf8.RuneError {
					return c, 12
				}
			}
		}
		return hi, 6
	}
	return '\\', 1
}

// hex4 reads the four hex digits at b[p].
func hex4(b []byte, p int) (rune, bool) {
	if p+4 > len(b) {
		return 0, false
	}
	var v rune
	for _, c := range b[p : p+4] {
		var d byte
		switch {
		case '0' <= c && c <= '9':
			d = c - '0'
		case 'a' <= c && c <= 'f':
			d = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		v = v<<4 | rune(d)
	}
	return v, true
}

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

// writeJSONL writes one JSON line per record at rel, at once. The run uses
// it for templates.jsonl, whose records are final only once the upload
// stage has decided the renames; every other stage file is a stream.
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

// stream is one stage's JSON Lines file. Each record is written as one line
// the moment it is added, straight to the file with no buffer in between, so
// a run that stops early, interrupted or on an error, keeps every record made
// until then. The stream also keeps every record, whole, for the summary.
// Workers add to it concurrently; the lines are serialised.
type stream[T any] struct {
	name string
	red  redactor
	// written is the record as the file holds it (its CDR text cut); nil
	// writes the record as kept.
	written func(T) T
	// stop ends the run when a line cannot be written: the records lost
	// would include EHR ids, and the run would go on creating EHRs that
	// nothing records.
	stop context.CancelCauseFunc

	mu     sync.Mutex
	f      *os.File
	recs   []T
	failed bool
}

// openStream creates the stage file name in the output directory. The
// directory is the run's own (prepareOut), so the file is new.
func openStream[T any](s *sink, name string, stop context.CancelCauseFunc, written func(T) T) (*stream[T], error) {
	f, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	return &stream[T]{name: name, red: s.red, written: written, stop: stop, f: f}, nil
}

// add writes each record as a line and keeps it.
func (w *stream[T]) add(recs ...T) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rec := range recs {
		w.recs = append(w.recs, rec)
		if w.failed {
			continue
		}
		if err := w.writeLine(rec); err != nil {
			w.failed = true
			w.stop(fmt.Errorf("write %s: %w", w.name, err))
		}
	}
}

func (w *stream[T]) writeLine(rec T) error {
	if w.f == nil {
		return errors.New("the file is closed")
	}
	if w.written != nil {
		rec = w.written(rec)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return err
	}
	_, err := w.f.Write(w.red.bytes(b.Bytes()))
	return err
}

// records returns every record added so far. A nil stream, one the run did
// not open, has none.
func (w *stream[T]) records() []T {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.recs)
}

// close closes the file. A write that failed has stopped the run already,
// and its error is the run's.
func (w *stream[T]) close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	if err != nil {
		return fmt.Errorf("close %s: %w", w.name, err)
	}
	return nil
}

// unsafeName matches the runs of characters a file name built from a
// template id leaves out.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileName turns a template id into a file or directory name: the id with
// each run of other characters replaced by "_", then a hyphen and the first
// 8 hex digits of the id's SHA-256. Two ids the replacement makes alike (a
// doubled space and a single one) or that differ only in case (one file on a
// case-insensitive file system) still get names of their own, so concurrent
// workers never write to one file.
func fileName(id string) string {
	sum := sha256.Sum256([]byte(id))
	return unsafeName.ReplaceAllString(id, "_") + "-" + hex.EncodeToString(sum[:4])
}
