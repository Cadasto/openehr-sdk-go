package aom14_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// noSeek hides every method of r but Read, so the reader keeps a copy of the
// input as it reads it.
type noSeek struct{ r io.Reader }

func (n noSeek) Read(p []byte) (int, error) { return n.r.Read(p) }

// failingSeek is a reader whose Seek always fails, as a pipe's does.
type failingSeek struct{ *bytes.Reader }

func (failingSeek) Seek(int64, int) (int64, error) { return 0, errors.New("illegal seek") }

// offsetOnly is a reader that reports its offset but cannot seek anywhere.
type offsetOnly struct{ *bytes.Reader }

func (o offsetOnly) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekCurrent {
		return o.Reader.Seek(offset, whence)
	}
	return 0, errors.New("cannot rewind")
}

// readResult is what one ReadOperationalTemplate call gave: the model written
// back as canonical XML, the error's text and the items it reported.
type readResult struct {
	xml, err string
	reported []string
}

// readWith reads r with opts, and with a function the decode reports to when
// report is set, and returns what the read gave.
func readWith(t *testing.T, r io.Reader, opts []canxml.DecoderOption, report bool) readResult {
	t.Helper()
	var res readResult
	if report {
		opts = append(slices.Clone(opts), canxml.WithUnmatched(func(p canxml.Path) { res.reported = append(res.reported, p.String()) }))
	}
	ot, err := aom14.ReadOperationalTemplate(r, opts...)
	if err != nil {
		res.err = err.Error()
		return res
	}
	data, err := canxml.Marshal(ot)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	res.xml = string(data)
	return res
}

// TestReadOperationalTemplateRewindsOrCopies pins that a reader that can
// seek, one that cannot, one whose Seek fails and one that reports its
// offset but cannot seek back give the same reading, the same error and the
// same reported items for every corpus template, in each mode, with and
// without a function the decode reports to; and that a seekable reader
// positioned past some prefix is read from that position (REQ-176). The
// reader copies every input as it reads it, so these hold by construction;
// the test keeps them should a reader ever read a seekable input twice.
func TestReadOperationalTemplateRewindsOrCopies(t *testing.T) {
	t.Parallel()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 50 {
		t.Fatalf("only %d corpus templates", len(refs))
	}
	for _, ref := range refs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(ref.Name, func(t *testing.T) {
			t.Parallel()
			for mode, opts := range modes {
				for _, report := range []bool{false, true} {
					want := readWith(t, noSeek{bytes.NewReader(data)}, opts, report)
					prefixed := bytes.NewReader(append([]byte("ignored prefix"), data...))
					if _, err := prefixed.Seek(int64(len("ignored prefix")), io.SeekStart); err != nil {
						t.Fatal(err)
					}
					file, err := os.Open(ref.Path)
					if err != nil {
						t.Fatal(err)
					}
					for name, r := range map[string]io.Reader{
						"seekable":       bytes.NewReader(data),
						"past a prefix":  prefixed,
						"section":        io.NewSectionReader(bytes.NewReader(data), 0, int64(len(data))),
						"file":           file,
						"failing seek":   failingSeek{bytes.NewReader(data)},
						"offset only":    offsetOnly{bytes.NewReader(data)},
						"strings.Reader": strings.NewReader(string(data)),
					} {
						if got := readWith(t, r, opts, report); got.xml != want.xml || got.err != want.err || !slices.Equal(got.reported, want.reported) {
							t.Errorf("%s mode, report %t: the %s reader read (err %q, %d reported) where a copying one read (err %q, %d reported)",
								mode, report, name, got.err, len(got.reported), want.err, len(want.reported))
						}
					}
					// The file is only read, so its Close has nothing to report.
					_ = file.Close()
				}
			}
		})
	}
}

// brokenAt reads, through ReadAt, as head followed by x up to size bytes, and
// fails with errSource each read that reaches past failAt, when failAt is
// above zero.
type brokenAt struct {
	head         string
	size, failAt int64
}

func (r brokenAt) ReadAt(p []byte, off int64) (int, error) {
	end := min(off+int64(len(p)), r.size)
	if r.failAt > 0 {
		end = min(end, r.failAt)
	}
	n := int(max(end-off, 0))
	for i := range n {
		p[i] = 'x'
	}
	if off < int64(len(r.head)) {
		copy(p[:n], r.head[off:])
	}
	switch {
	case n == len(p):
		return n, nil
	case r.failAt > 0 && off+int64(len(p)) > r.failAt:
		return n, errSource
	}
	return n, io.EOF
}

// TestREQ108_ReadOperationalTemplateReplaysWhatTheFirstDecodeRead pins that,
// where the first decode fails in a way no second reading mends, such as an
// element past the element limit, a reader that can seek back gives what a
// copying one gives with a function the decode reports to: the decode's
// refusal, never a failure of the reader or the size refusal from past the
// bytes the first decode read (REQ-108). A reader that read a seekable input
// again to its end for the replay would return that later failure instead.
func TestREQ108_ReadOperationalTemplateReplaysWhatTheFirstDecodeRead(t *testing.T) {
	t.Parallel()
	deep := `<template xmlns="http://schemas.openehr.org/v1">` + strings.Repeat("<a>", readElementLimit+1)
	inputs := map[string]brokenAt{
		"a reader failing past what the decode read": {head: deep, size: int64(len(deep)) + 20000, failAt: 8192},
		"an input past the size limit":               {head: deep, size: readSizeLimit + 1},
	}
	for name, in := range inputs {
		for mode, opts := range modes {
			for _, report := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/report %t", name, mode, report), func(t *testing.T) {
					t.Parallel()
					want := readWith(t, noSeek{io.NewSectionReader(in, 0, in.size)}, opts, report)
					if want.err == "" || strings.Contains(want.err, errSource.Error()) || want.err == readSizeRefusal {
						t.Fatalf("a copying reader read (err %q), want the decode's refusal", want.err)
					}
					got := readWith(t, io.NewSectionReader(in, 0, in.size), opts, report)
					if got.xml != want.xml || got.err != want.err || !slices.Equal(got.reported, want.reported) {
						t.Errorf("a seekable reader read (err %q, reported %q) where a copying one read (err %q, reported %q)",
							got.err, got.reported, want.err, want.reported)
					}
				})
			}
		}
	}
}

// BenchmarkReadOperationalTemplate reads every corpus template from a
// *bytes.Reader, from an *os.File and from a reader that can only be read,
// each of which the reader copies as it reads it.
func BenchmarkReadOperationalTemplate(b *testing.B) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		b.Fatal(err)
	}
	docs := make([][]byte, len(refs))
	files := make([]*os.File, len(refs))
	for i, ref := range refs {
		if docs[i], err = os.ReadFile(ref.Path); err != nil {
			b.Fatal(err)
		}
		f, err := os.Open(ref.Path)
		if err != nil {
			b.Fatal(err)
		}
		// The file is only read, so its Close has nothing to report.
		b.Cleanup(func() { _ = f.Close() })
		files[i] = f
	}
	readers := []struct {
		name string
		open func(b *testing.B, i int) io.Reader
	}{
		{name: "bytes.Reader", open: func(_ *testing.B, i int) io.Reader { return bytes.NewReader(docs[i]) }},
		{name: "os.File", open: func(b *testing.B, i int) io.Reader {
			if _, err := files[i].Seek(0, io.SeekStart); err != nil {
				b.Fatal(err)
			}
			return files[i]
		}},
		{name: "copying", open: func(_ *testing.B, i int) io.Reader { return noSeek{bytes.NewReader(docs[i])} }},
	}
	for _, r := range readers {
		b.Run(r.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for i := range docs {
					if _, err := aom14.ReadOperationalTemplate(r.open(b, i)); err != nil {
						b.Fatalf("%s: %v", refs[i].Name, err)
					}
				}
			}
		})
	}
}
