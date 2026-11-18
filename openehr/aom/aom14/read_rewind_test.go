package aom14_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"slices"
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

// TestReadOperationalTemplateRewindsOrCopies pins that a reader that can
// seek, one that cannot and one whose Seek fails give the same reading, the
// same error and the same reported items for every corpus template, in each
// mode, with and without a function the decode reports to; and that a
// seekable reader positioned past some prefix is read from that position.
func TestReadOperationalTemplateRewindsOrCopies(t *testing.T) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 50 {
		t.Fatalf("only %d corpus templates", len(refs))
	}
	type result struct {
		xml, err string
		reported []string
	}
	read := func(r io.Reader, opts []canxml.DecoderOption, report bool) result {
		var res result
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
	for _, ref := range refs {
		data, err := os.ReadFile(ref.Path)
		if err != nil {
			t.Fatal(err)
		}
		for mode, opts := range modes {
			for _, report := range []bool{false, true} {
				want := read(noSeek{bytes.NewReader(data)}, opts, report)
				prefixed := bytes.NewReader(append([]byte("ignored prefix"), data...))
				if _, err := prefixed.Seek(int64(len("ignored prefix")), io.SeekStart); err != nil {
					t.Fatal(err)
				}
				for name, r := range map[string]io.Reader{
					"seekable":      bytes.NewReader(data),
					"past a prefix": prefixed,
					"failing seek":  failingSeek{bytes.NewReader(data)},
				} {
					if got := read(r, opts, report); got.xml != want.xml || got.err != want.err || !slices.Equal(got.reported, want.reported) {
						t.Errorf("%s, %s mode, report %t: a %s reader read (err %q, %d reported) where a copying one read (err %q, %d reported)",
							ref.Name, mode, report, name, got.err, len(got.reported), want.err, len(want.reported))
					}
				}
			}
		}
	}
}

// BenchmarkReadOperationalTemplate reads every corpus template from a
// *bytes.Reader and from an *os.File, which the reader reads again only when
// it needs a second reading, and from a reader it copies as it reads.
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
