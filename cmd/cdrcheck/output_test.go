package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// escapeAll writes every character of s as a JSON \u escape, in upper-case
// hex, as some JSON encoders do for characters they consider unsafe.
func escapeAll(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04X`, r)
	}
	return b.String()
}

func TestRedactor_FindsTheCredentialInEveryForm_REQ082(t *testing.T) {
	t.Parallel()
	b64 := func(user, pass string) string { return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass)) }
	// "ab?:cd>~" encodes to YWI/OmNkPn4=, with a slash and padding, and
	// "op:pa55?~~" to b3A6cGE1NT9+fg==, with a plus.
	cases := []struct {
		name, user, pass, text, want string
	}{
		{name: "user:pass", user: "operator", pass: "s3cr3t", text: "url http://operator:s3cr3t@host/", want: "url http://" + redacted + "@host/"},
		{name: "base64", user: "ab?", pass: "cd>~", text: "Basic " + b64("ab?", "cd>~") + ".", want: "Basic " + redacted + "."},
		{name: `base64 with \/ escapes`, user: "ab?", pass: "cd>~", text: `"Basic YWI\/OmNkPn4="`, want: `"Basic ` + redacted + `"`},
		{name: `base64 padding as \u003d`, user: "ab?", pass: "cd>~", text: `"Basic YWI\/OmNkPn4\u003d"`, want: `"Basic ` + redacted + `"`},
		{name: `base64 plus and padding as \u002B and \u003D`, user: "op", pass: "pa55?~~", text: `"Basic b3A6cGE1NT9\u002Bfg\u003D\u003d"`, want: `"Basic ` + redacted + `"`},
		{name: `user:pass, every character a \u escape`, user: "operator", pass: "s3cr3t", text: `"` + escapeAll("operator:s3cr3t") + `"`, want: `"` + redacted + `"`},
		{name: `a quote and a backslash in the password`, user: "op", pass: `s"3\c`, text: `{"message":"op:s\"3\\c"}`, want: `{"message":"` + redacted + `"}`},
		{name: "a character outside the BMP as a surrogate pair", user: "op", pass: "key🔑", text: `"op:key\ud83d\udd11"`, want: `"` + redacted + `"`},
		{name: "the password alone is kept", user: "operator", pass: "ehrbase", text: "ehrbase answered 422", want: "ehrbase answered 422"},
		{name: "an escaped backslash is not an escape", user: "operator", pass: "s3cr3t", text: `operator:s3cr3\\u0074`, want: `operator:s3cr3\\u0074`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRedactor([]cdrTarget{{name: "x", user: tc.user, pass: tc.pass}})
			if got := r.string(tc.text); got != tc.want {
				t.Errorf("redact(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestFileName_DistinctForEveryTemplateID_REQ082(t *testing.T) {
	t.Parallel()
	safe := regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	pairs := [][2]string{
		{"x a.v1", "x  a.v1"}, // a real id carries a doubled space
		{"x/a.v1", "x?a.v1"},
		{"X.v1", "x.v1"}, // one file on a case-insensitive file system
	}
	for _, p := range pairs {
		a, b := fileName(p[0]), fileName(p[1])
		if strings.EqualFold(a, b) {
			t.Errorf("fileName(%q) = %q and fileName(%q) = %q, want names no file system confuses", p[0], a, p[1], b)
		}
		for _, n := range []string{a, b} {
			if !safe.MatchString(n) {
				t.Errorf("fileName = %q, want only [A-Za-z0-9._-]", n)
			}
		}
	}
	if got := fileName("x a.v1"); !strings.HasPrefix(got, "x_a.v1-") {
		t.Errorf(`fileName("x a.v1") = %q, want the id's readable form first`, got)
	}
}

func TestPrepareOut_ClaimsTheDirectoryOnce_REQ082(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(dir string) error
		wantErr string
	}{
		{name: "a new directory", setup: func(string) error { return nil }},
		{name: "a new directory under a new parent", setup: func(string) error { return nil }},
		{name: "an existing empty directory", setup: func(dir string) error { return os.Mkdir(dir, 0o755) }},
		{name: "a directory another run has claimed", wantErr: "in use", setup: func(dir string) error {
			if err := os.Mkdir(dir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, runFile), nil, 0o644)
		}},
		{name: "a directory holding other files", wantErr: "not empty", setup: func(dir string) error {
			if err := os.Mkdir(dir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, "summary.md"), []byte("an earlier run"), 0o644)
		}},
		{name: "a file", wantErr: "output directory", setup: func(dir string) error { return os.WriteFile(dir, nil, 0o644) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "out")
			if strings.Contains(tc.name, "new parent") {
				dir = filepath.Join(t.TempDir(), "a", "b", "out")
			}
			if err := tc.setup(dir); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadDir(dir)
			got, err := prepareOut(config{out: dir})
			if tc.wantErr == "" {
				if err != nil || got != dir {
					t.Fatalf("prepareOut(%s) = %q, %v, want the directory claimed", tc.name, got, err)
				}
				if _, err := os.Stat(filepath.Join(dir, runFile)); err != nil {
					t.Errorf("prepareOut(%s) left no %s claim: %v", tc.name, runFile, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("prepareOut(%s) = %q, %v, want a refusal naming %q", tc.name, got, err, tc.wantErr)
			}
			if after, _ := os.ReadDir(dir); len(after) != len(before) {
				t.Errorf("prepareOut(%s) left %d entries, want the %d it found", tc.name, len(after), len(before))
			}
		})
	}
}

func TestPrepareOut_ConcurrentRunsGetOneDirectoryEach_REQ082(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "out")
	var won atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := prepareOut(config{out: dir}); err == nil {
				won.Add(1)
			}
		})
	}
	wg.Wait()
	if n := won.Load(); n != 1 {
		t.Errorf("%d of 8 concurrent runs claimed %s, want exactly 1", n, dir)
	}
}

func TestPrepareOut_DefaultIsANewDirectoryPerRun_REQ082(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "openehr-cdrcheck")
	seen := map[string]bool{}
	for range 5 { // well within one second, the old name's resolution
		dir, err := prepareOut(config{outRoot: root})
		if err != nil {
			t.Fatalf("prepareOut(default) = %v, want a new directory", err)
		}
		if seen[dir] || filepath.Dir(dir) != root {
			t.Errorf("prepareOut(default) = %s, want a new directory under %s", dir, root)
		}
		seen[dir] = true
	}
}

func TestCall_MarksARequestTheRunStopped_REQ082(t *testing.T) {
	t.Parallel()
	c := &cdrClient{name: "fake"}
	live := c.call(t.Context(), func(context.Context) error { return errors.New("connection refused") })
	if live.OK || live.Stopped {
		t.Errorf("call(a failure while the run goes on) = %+v, want a failure not marked stopped", live)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stopped := c.call(ctx, func(ctx context.Context) error { return ctx.Err() })
	if stopped.OK || !stopped.Stopped {
		t.Errorf("call(the run stopped) = %+v, want it marked stopped: its verdict is unknown", stopped)
	}
}

func TestStream_WritesARedactedLineAndStopsTheRunOnAFailure_REQ082(t *testing.T) {
	t.Parallel()
	const user, pass = "operator", "s3cr3t"
	s := &sink{dir: t.TempDir(), red: newRedactor([]cdrTarget{{name: "x", user: user, pass: pass}})}
	var cause error
	w, err := openStream(s, "post.jsonl", func(err error) { cause = err }, postRecord.written)
	if err != nil {
		t.Fatal(err)
	}
	w.add(postRecord{CDR: "x", Label: "one", Error: "echoed " + user + ":" + pass})
	b, err := os.ReadFile(filepath.Join(s.dir, "post.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), pass) || !strings.Contains(string(b), redacted) {
		t.Errorf("post.jsonl = %s, want the line written at once with the credential redacted", b)
	}
	if err := w.f.Close(); err != nil { // the next write fails, as on a full disk
		t.Fatal(err)
	}
	w.add(postRecord{CDR: "x", Label: "two"})
	if cause == nil || !strings.Contains(cause.Error(), "post.jsonl") {
		t.Errorf("after a failed write the run's stop cause = %v, want one naming post.jsonl", cause)
	}
	if got := w.records(); len(got) != 2 {
		t.Errorf("records() = %d, want both records kept for the summary", len(got))
	}
}
