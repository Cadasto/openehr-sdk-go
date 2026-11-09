package aom14

// REQ-111 (clinical-modeling.md § REQ-111, Reading the model, "What the
// reader tolerates"): the reader applies those readings to every document
// that needs one, so the facts that decide on a second reading never skip
// one that reads a document otherwise.

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// secondReading is what the token facts of one document say, and what the
// second reading makes of it.
type secondReading struct {
	decoded bool // the first decode read the document without a failure
	asked   bool // the token facts ask for a second reading
	// differs is true where the second reading reads the document otherwise
	// than the first decode: another model, or a refusal.
	differs bool
}

// secondReadingOf runs the first decode of data as ReadOperationalTemplate
// does, the token facts, and the second reading.
func secondReadingOf(data []byte, strict bool) secondReading {
	var facts tokenFacts
	opts := []canxml.DecoderOption{facts.observeOption()}
	if strict {
		opts = append(opts, canxml.WithStrict())
	}
	ot, err := readDocument(bufio.NewReader(bytes.NewReader(data)), opts)
	if err != nil {
		return secondReading{}
	}
	r := secondReading{decoded: true, asked: facts.needsTolerance(ot, strict)}
	read, changed, err := tolerate(data, strict, readMaxDepth)
	switch {
	case err != nil:
		r.differs = true
	case changed:
		var second []canxml.DecoderOption
		if strict {
			second = append(second, canxml.WithStrict())
		}
		ot2, err := readDocument(bufio.NewReader(bytes.NewReader(read)), second)
		r.differs = err != nil || !reflect.DeepEqual(ot, ot2)
	}
	return r
}

// templateRoots are the folders that hold the operational templates of the
// repository, relative to this package.
var templateRoots = []string{
	filepath.Join("testdata"),
	filepath.Join("..", "..", "..", "testkit", "corpus"),
	filepath.Join("..", "..", "..", "internal", "templatecompile", "testdata"),
	filepath.Join("..", "..", "serialize", "simplified", "testdata"),
}

// templateFiles returns the operational templates under templateRoots: the
// .opt files, and the .xml files whose text holds a <template> and a
// <definition>.
func templateFiles(t testing.TB) []string {
	t.Helper()
	var files []string
	for _, root := range templateRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			switch filepath.Ext(path) {
			case ".opt":
				files = append(files, path)
			case ".xml":
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if bytes.Contains(data, []byte("<template")) && bytes.Contains(data, []byte("<definition")) {
					files = append(files, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

// TestREQ111_TokenFactsMissNoSecondReading pins that, for every operational
// template of the repository, in both modes, where the token facts of the
// first decode ask for no second reading, that reading reads the document as
// the first decode did: the facts never skip a reading the reader needs.
//
// Can-fail control: drop the definition fact from needsTolerance, and
// testdata/reader/definition-type.opt, in both modes, and the parity
// exceptions definition_archetype_slot.opt and
// definition_primitive_constraint.opt read otherwise. The reader's behaviour
// tests hold the other facts.
func TestREQ111_TokenFactsMissNoSecondReading(t *testing.T) {
	t.Parallel()
	files := templateFiles(t)
	if len(files) < 100 {
		t.Fatalf("found %d operational templates under %q, want the repository's corpus", len(files), templateRoots)
	}
	for _, strict := range []bool{false, true} {
		var decoded, asked, needed int
		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			r := secondReadingOf(data, strict)
			if !r.decoded {
				continue
			}
			decoded++
			if r.asked {
				asked++
			}
			if r.differs {
				needed++
				if !r.asked {
					t.Errorf("strict=%v %s: the token facts ask for no second reading, which reads the document otherwise", strict, path)
				}
			}
		}
		if needed == 0 {
			t.Errorf("strict=%v: no template of the %d decoded needs a second reading, want the corpus to hold one", strict, decoded)
		}
		t.Logf("strict=%v: %d of %d templates decoded; a second reading asked for by %d, needed by %d", strict, decoded, len(files), asked, needed)
	}
}

// FuzzTokenFacts holds the token facts to the second reading on mutated
// documents: where the first decode reads a document and the facts ask for no
// second reading, that reading reads it as the first decode did. Its seeds are
// the documents of testdata/reader and the corpus templates under 40 KB.
func FuzzTokenFacts(f *testing.F) {
	for _, path := range templateFiles(f) {
		if !strings.Contains(path, filepath.Join("testdata", "reader")) {
			if fi, err := os.Stat(path); err != nil || fi.Size() > 40<<10 {
				continue
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data, false)
		f.Add(data, true)
	}
	f.Fuzz(func(t *testing.T, data []byte, strict bool) {
		r := secondReadingOf(data, strict)
		if r.decoded && r.differs && !r.asked {
			t.Errorf("strict=%v: the token facts ask for no second reading, which reads the document otherwise", strict)
		}
	})
}
