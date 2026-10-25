package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// utf8BOM is the UTF-8 byte-order mark. EHRbase refuses an OPT that starts
// with one ("Content is not allowed in prolog"), so the run strips it before
// every upload, and before it reads or rewrites an OPT.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func stripBOM(b []byte) []byte { return bytes.TrimPrefix(b, utf8BOM) }

// templateRecord is one OPT file of the template directory, one line of
// templates.jsonl.
type templateRecord struct {
	TemplateID string `json:"template_id,omitempty"`
	File       string `json:"file"`
	Root       string `json:"root,omitempty"`
	Signature  string `json:"signature,omitempty"`
	// Kept is true for the file the run checks. A second file with the
	// same template id is not kept: Skipped says why, KeptFile names the
	// file kept instead.
	Kept     bool   `json:"kept"`
	Skipped  string `json:"skipped,omitempty"`
	KeptFile string `json:"kept_file,omitempty"`
	// RenamedFrom is set on an OPT the run uploaded under a new id because
	// a CDR holds other content under its own (see uploadStage), and
	// RenamedTo on the file it was made from.
	RenamedFrom string `json:"renamed_from,omitempty"`
	RenamedTo   string `json:"renamed_to,omitempty"`
}

// optInfo is what one scan of an OPT reads.
type optInfo struct {
	templateID string
	root       string
	signature  string
	// idStart and idEnd are the byte offsets, in the scanned bytes, of the
	// text of template/template_id/value.
	idStart, idEnd int64
}

// scanOPT reads an OPT's template id, its root RM type and its signature.
// The bytes must have no byte-order mark.
//
// The signature is a SHA-256 over the parsed document: each element's local
// name, its attributes (local name and value, the value of a namespaced
// type attribute after its last colon, so that an xsi:type prefix does not
// count, while a term code keeps its terminology) sorted, and its trimmed
// text, in document order. Namespace declarations are left out. A CDR that stores a
// template and serves it back re-serialised, with other prefixes, other
// indentation or another attribute order, yields the same signature; a
// different template does not.
func scanOPT(b []byte) (optInfo, error) {
	var info optInfo
	dec := xml.NewDecoder(bytes.NewReader(b))
	h := sha256.New()
	var path []string
	var text []*strings.Builder
	flush := func() {
		if n := len(text); n > 0 {
			_, _ = fmt.Fprintf(h, "%s|", strings.TrimSpace(text[n-1].String())) // a hash.Hash never fails a write
			text[n-1].Reset()
		}
	}
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return optInfo{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			flush()
			path = append(path, t.Name.Local)
			text = append(text, &strings.Builder{})
			var attrs []string
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					continue
				}
				v := a.Value
				if a.Name.Local == "type" && a.Name.Space != "" {
					if i := strings.LastIndex(v, ":"); i >= 0 {
						v = v[i+1:]
					}
				}
				attrs = append(attrs, a.Name.Local+"="+v)
			}
			slices.Sort(attrs)
			_, _ = fmt.Fprintf(h, "<%s%v>", t.Name.Local, attrs)
		case xml.EndElement:
			flush()
			path = path[:len(path)-1]
			text = text[:len(text)-1]
			h.Write([]byte("/"))
		case xml.CharData:
			if n := len(text); n > 0 {
				text[n-1].Write(t)
			}
			switch {
			case info.templateID == "" && pathIs(path, "template_id", "value"):
				info.templateID = strings.TrimSpace(string(t))
				info.idStart, info.idEnd = off, dec.InputOffset()
			case info.root == "" && pathIs(path, "definition", "rm_type_name"):
				info.root = strings.TrimSpace(string(t))
			}
		}
	}
	info.signature = hex.EncodeToString(h.Sum(nil))
	return info, nil
}

// pathIs reports whether path is the document element followed by want.
func pathIs(path []string, want ...string) bool {
	return len(path) == len(want)+1 && slices.Equal(path[1:], want)
}

// signatureOf returns an OPT's signature, or a hash of its bytes when they do
// not parse as XML: two unparseable copies then compare equal only when they
// are the same bytes.
func signatureOf(b []byte) string {
	b = stripBOM(b)
	if info, err := scanOPT(b); err == nil {
		return info.signature
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// collectTemplates reads every .opt file under dir. Files are taken in path
// order; of two files with one template id the first is kept, and the
// second is recorded as skipped. A file with no template id is recorded and
// skipped.
func collectTemplates(dir string) ([]templateRecord, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".opt") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read template directory: %w", err)
	}
	slices.Sort(files)
	var out []templateRecord
	kept := map[string]int{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read OPT: %w", err)
		}
		rec := templateRecord{File: f}
		info, err := scanOPT(stripBOM(raw))
		switch {
		case err != nil:
			rec.Skipped = "not well-formed XML: " + err.Error()
		case info.templateID == "":
			rec.Skipped = "no template_id"
		default:
			rec.TemplateID, rec.Root, rec.Signature = info.templateID, info.root, info.signature
			if i, dup := kept[info.templateID]; dup {
				rec.KeptFile = out[i].File
				if out[i].Signature == info.signature {
					rec.Skipped = "same template id, same content"
				} else {
					rec.Skipped = "same template id, other content"
				}
			} else {
				rec.Kept = true
				kept[info.templateID] = len(out)
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

// renamedOPT returns the OPT b with its template id replaced by newID: the
// text of template/template_id/value alone, which names the template on
// upload and in every document generated from it.
func renamedOPT(b []byte, newID string) ([]byte, error) {
	b = stripBOM(b)
	info, err := scanOPT(b)
	if err != nil {
		return nil, err
	}
	if info.templateID == "" {
		return nil, errors.New("no template_id to rename")
	}
	var esc bytes.Buffer
	if err := xml.EscapeText(&esc, []byte(newID)); err != nil {
		return nil, err
	}
	out := slices.Concat(b[:info.idStart], esc.Bytes(), b[info.idEnd:])
	return out, nil
}
